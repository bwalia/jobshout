package marketingmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// marketingPrefix is the OpsAPI Marketing Cloud plugin's api_prefix.
const marketingPrefix = "/api/v2/opsapi-marketing"

const (
	defaultTimeout   = 30 * time.Second
	maxResponseBytes = 512 << 10
	userAgent        = "JobShout-MarketingManager/1"
)

// Conn is a resolved OpsAPI workspace: where it is, the key, the namespace.
// The key never leaves this package — not in errors, results or logs.
type Conn struct {
	Website   string // integration name, or "ring" for the ring's own OpsAPI
	BaseURL   string
	APIKey    string
	Namespace string
	// TestRecipients are the only addresses a test send may go to (lower case).
	TestRecipients []string
	// Brand is the website's look for emails (colours, logo, font, tone),
	// from the integration's config.marketing_brand.
	Brand map[string]any
}

// Client calls one OpsAPI workspace's Marketing Cloud and CRM endpoints.
type Client struct {
	conn Conn
	http *http.Client
}

func newClient(conn Conn, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	} else {
		cp := *hc
		hc = &cp
	}
	// A redirect could carry the bearer key off the configured host.
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	conn.BaseURL = strings.TrimRight(conn.BaseURL, "/")
	return &Client{conn: conn, http: hc}
}

// Website names the workspace the client talks to, for results.
func (c *Client) Website() string { return c.conn.Website }

// Namespace is the OpsAPI namespace slug.
func (c *Client) Namespace() string { return c.conn.Namespace }

// APIError is a non-2xx answer from OpsAPI, already in OpsAPI's own words.
type APIError struct {
	Status  int
	Message string
	Code    string // details.code when OpsAPI gives one (approval_required, …)
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if e.Code == codePluginDisabled {
		return "The OpsAPI Marketing Cloud plugin is turned off for this workspace: an admin enables it in OpsAPI → Workspace → Plugins. " + msg
	}
	switch e.Status {
	case http.StatusUnauthorized:
		return "OpsAPI refused the workspace key (401): it is invalid, expired or revoked. " + msg
	case http.StatusForbidden:
		return "OpsAPI refused this for the workspace key (403): " + msg +
			". The key needs the opsapi_marketing_* (and crm) scopes — see docs/marketing-manager.md."
	case http.StatusNotFound:
		return "Not found in OpsAPI (404): " + msg
	}
	if e.Code != "" {
		return fmt.Sprintf("OpsAPI (%d, %s): %s", e.Status, e.Code, msg)
	}
	return fmt.Sprintf("OpsAPI (%d): %s", e.Status, msg)
}

// codePluginDisabled is OpsAPI's details.code when a plugin is off for the
// namespace (lapis/middleware/namespace.lua).
const codePluginDisabled = "PLUGIN_DISABLED"

// IsCode reports whether err is an OpsAPI error with details.code == code.
func IsCode(err error, code string) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Code == code
}

// envelope is OpsAPI's house shape: {success, data, meta} / {success, error, details}.
type envelope struct {
	Success *bool           `json:"success"`
	Data    json.RawMessage `json:"data"`
	Meta    json.RawMessage `json:"meta"`
	Error   json.RawMessage `json:"error"`
	Details json.RawMessage `json:"details"`
}

// Response is the decoded data (and meta) of a successful call.
type Response struct {
	Data json.RawMessage
	Meta json.RawMessage
}

// retryDelays are the waits before a second and third attempt. Only calls
// that cannot do anything twice are retried: GETs, and creates carrying an
// Idempotency-Key (OpsAPI returns the first result). A send never is.
var retryDelays = []time.Duration{500 * time.Millisecond, 2 * time.Second}

// Do calls path (absolute, starting /api/v2/) with an optional JSON body.
// idemKey, when set, is sent as Idempotency-Key so a repeated create returns
// the first result instead of making a second record.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any, idemKey string) (*Response, error) {
	u, err := c.buildURL(path, query)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if body != nil {
		if raw, err = json.Marshal(body); err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
	}
	retryable := method == http.MethodGet || idemKey != ""
	for attempt := 0; ; attempt++ {
		resp, err := c.doOnce(ctx, method, u, path, raw, body != nil, idemKey)
		if err == nil || !retryable || attempt >= len(retryDelays) || !transient(err) {
			return resp, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(retryDelays[attempt]):
		}
	}
}

// transient is a failure worth one more try: no answer, rate limiting, or a
// gateway/overload status.
func transient(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		switch ae.Status {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	return errors.Is(err, errUnreachable)
}

var errUnreachable = errors.New("OpsAPI unreachable")

func (c *Client) doOnce(ctx context.Context, method, u, path string, payload []byte, hasBody bool, idemKey string) (*Response, error) {
	var rdr io.Reader
	if hasBody {
		rdr = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.conn.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Namespace-Slug", c.conn.Namespace)
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUnreachable, scrub(err, c.conn.APIKey))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("OpsAPI answer for %s %s is over %d KB; narrow the request (per_page, search)", method, path, maxResponseBytes>>10)
	}

	var env envelope
	jsonErr := json.Unmarshal(raw, &env)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		ae := &APIError{Status: resp.StatusCode}
		if jsonErr == nil {
			ae.Message = errorText(env.Error)
			var d struct {
				Code string `json:"code"`
			}
			if len(env.Details) > 0 && json.Unmarshal(env.Details, &d) == nil {
				ae.Code = d.Code
			}
			if ae.Code == "" {
				// Validation failures put per-field errors in details.
				if s := strings.TrimSpace(string(env.Details)); s != "" && s != "null" && len(s) < 600 {
					ae.Message += " " + s
				}
			}
		}
		if ae.Message == "" {
			ae.Message = strings.TrimSpace(firstN(string(raw), 300))
		}
		if c.conn.APIKey != "" {
			ae.Message = strings.ReplaceAll(ae.Message, c.conn.APIKey, "[redacted]")
		}
		return nil, ae
	}
	if jsonErr != nil {
		return nil, fmt.Errorf("OpsAPI answered %s %s with something that is not JSON", method, path)
	}
	if env.Success != nil && !*env.Success {
		return nil, &APIError{Status: resp.StatusCode, Message: errorText(env.Error)}
	}
	return &Response{Data: env.Data, Meta: env.Meta}, nil
}

func (c *Client) buildURL(path string, query url.Values) (string, error) {
	if !strings.HasPrefix(path, "/api/v2/") || strings.Contains(path, "..") || strings.Contains(path, "//") {
		return "", fmt.Errorf("refusing OpsAPI path %q", path)
	}
	base, err := url.Parse(c.conn.BaseURL)
	if err != nil || base.Host == "" {
		return "", errors.New("the OpsAPI base URL is not valid")
	}
	u := *base
	u.Path = strings.TrimRight(base.Path, "/") + path
	u.RawQuery = query.Encode()
	final, err := url.Parse(u.String())
	if err != nil || final.Host != base.Host || final.Scheme != base.Scheme {
		return "", errors.New("refusing a request off the configured OpsAPI host")
	}
	return final.String(), nil
}

// errorText reads OpsAPI's error field, which is usually a string but is an
// object on a few legacy routes.
func errorText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		if v, ok := m["message"].(string); ok {
			return v
		}
	}
	return firstN(string(raw), 300)
}

// scrub removes the key from an error before it can reach a user or a log.
func scrub(err error, key string) error {
	if err == nil || key == "" || !strings.Contains(err.Error(), key) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), key, "[redacted]"))
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
