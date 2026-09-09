package waflab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	reBearer    = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9\-_\.=]+`)
	rePassword  = regexp.MustCompile(`(?i)"password"\s*:\s*"[^"]*"`)
	reAccess    = regexp.MustCompile(`(?i)"accessToken"\s*:\s*"[^"]*"`)
	reAPIToken  = regexp.MustCompile(`(?i)"api[_-]?token"\s*:\s*"[^"]*"`)
	reHTMLTitle = regexp.MustCompile(`(?is)<title>(.*?)</title>`)
)

// Config is loaded from WSLPROXY_* environment variables.
type Config struct {
	Enabled       bool
	BaseURL       string
	APIToken      string
	Username      string
	Password      string
	Platform      string
	Profile       string
	Timeout       time.Duration
	LabMaxRuntime time.Duration
	TargetAllow   []string // optional extra allow-list (hostnames)
}

// LoadConfig reads WSLPROXY_* settings.
func LoadConfig() Config {
	enabled := true
	if v := strings.TrimSpace(os.Getenv("WSLPROXY_ENABLED")); v != "" {
		enabled = parseBool(v, true)
	}
	timeout := 30 * time.Second
	if v := strings.TrimSpace(os.Getenv("WSLPROXY_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}
	maxRT := 10 * time.Minute
	if v := strings.TrimSpace(os.Getenv("WSLPROXY_LAB_MAX_RUNTIME")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			maxRT = d
		}
	}
	platform := strings.TrimSpace(os.Getenv("WSLPROXY_PLATFORM"))
	if platform == "" {
		platform = "openresty-admin-next"
	}
	profile := strings.TrimSpace(os.Getenv("WSLPROXY_PROFILE"))
	if profile == "" {
		profile = "prod"
	}
	var allow []string
	if v := strings.TrimSpace(os.Getenv("WSLPROXY_TARGET_ALLOWLIST")); v != "" {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(strings.ToLower(p))
			p = strings.TrimPrefix(p, "https://")
			p = strings.TrimPrefix(p, "http://")
			p = strings.TrimRight(p, "/")
			if p != "" {
				allow = append(allow, p)
			}
		}
	}
	return Config{
		Enabled:       enabled,
		BaseURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("WSLPROXY_BASE_URL")), "/"),
		APIToken:      strings.TrimSpace(os.Getenv("WSLPROXY_API_TOKEN")),
		Username:      strings.TrimSpace(os.Getenv("WSLPROXY_USERNAME")),
		Password:      strings.TrimSpace(os.Getenv("WSLPROXY_PASSWORD")),
		Platform:      platform,
		Profile:       profile,
		Timeout:       timeout,
		LabMaxRuntime: maxRT,
		TargetAllow:   allow,
	}
}

// DefaultOriginUpstream is the address the demo hosts proxy to.
//
// It must not be a loopback address: wslproxy commonly runs off-cluster, where
// 127.0.0.1 is the proxy host itself rather than the node holding the
// NodePort, which yields a 502 on every lab host. A resolvable name also
// survives the origin node being replaced, which a pinned node IP does not.
// wslproxy strips the scheme before proxy_pass (execution.lua), so including
// it here is safe.
func DefaultOriginUpstream() string {
	if v := strings.TrimSpace(os.Getenv("WSLPROXY_ORIGIN_UPSTREAM")); v != "" {
		return v
	}
	return "http://origin-uk-001.pop0.uk:30084"
}

func parseBool(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// Client talks to the wslproxy HTTP API.
type Client struct {
	cfg    Config
	http   *http.Client
	logger *zap.Logger

	mu          sync.Mutex
	accessToken string
}

// NewClient builds a wslproxy client.
func NewClient(cfg Config, logger *zap.Logger) *Client {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Platform == "" {
		cfg.Platform = "openresty-admin-next"
	}
	if cfg.Profile == "" {
		cfg.Profile = "prod"
	}
	c := &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: cfg.Timeout},
		logger: logger,
	}
	if cfg.APIToken != "" {
		c.accessToken = cfg.APIToken
	}
	return c
}

// Enabled reports whether the client has enough config to call wslproxy.
func (c *Client) Enabled() bool {
	if c == nil || !c.cfg.Enabled || c.cfg.BaseURL == "" {
		return false
	}
	return c.cfg.APIToken != "" || (c.cfg.Username != "" && c.cfg.Password != "")
}

// LabMaxRuntime returns the run backstop duration.
func (c *Client) LabMaxRuntime() time.Duration { return c.cfg.LabMaxRuntime }

// Profile returns the configured env profile.
func (c *Client) Profile() string { return c.cfg.Profile }

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string { return c.cfg.BaseURL }

// Login obtains (or refreshes) a bearer token via POST /api/user/login.
func (c *Client) Login(ctx context.Context) error {
	if c.cfg.APIToken != "" {
		c.mu.Lock()
		c.accessToken = c.cfg.APIToken
		c.mu.Unlock()
		return nil
	}
	if c.cfg.Username == "" || c.cfg.Password == "" {
		return fmt.Errorf("wslproxy: no API token or username/password configured")
	}
	body := map[string]string{
		"email":    c.cfg.Username,
		"password": c.cfg.Password,
	}
	var resp struct {
		Data struct {
			AccessToken string `json:"accessToken"`
		} `json:"data"`
		AccessToken string `json:"accessToken"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/user/login", body, &resp, false); err != nil {
		return err
	}
	tok := resp.Data.AccessToken
	if tok == "" {
		tok = resp.AccessToken
	}
	if tok == "" {
		return fmt.Errorf("wslproxy login: empty accessToken")
	}
	c.mu.Lock()
	c.accessToken = tok
	c.mu.Unlock()
	return nil
}

func (c *Client) token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accessToken
}

func (c *Client) setToken(tok string) {
	c.mu.Lock()
	c.accessToken = tok
	c.mu.Unlock()
}

// SeedWAFRules POSTs /api/waf_rules/seed.
func (c *Client) SeedWAFRules(ctx context.Context, profileID string) error {
	if profileID == "" {
		profileID = c.cfg.Profile
	}
	return c.doJSON(ctx, http.MethodPost, "/api/waf_rules/seed", map[string]any{
		"profile_id": profileID,
	}, nil, true)
}

// ListWAFRules GETs /api/waf_rules.
func (c *Client) ListWAFRules(ctx context.Context) ([]map[string]any, error) {
	return c.listMaps(ctx, "/api/waf_rules")
}

// UpsertWAFRules imports rules via POST /api/projects/import.
func (c *Client) UpsertWAFRules(ctx context.Context, rules []map[string]any) error {
	return c.ImportData(ctx, "waf_rules", rules)
}

// ListWAFPolicies GETs /api/waf_policies.
func (c *Client) ListWAFPolicies(ctx context.Context) ([]map[string]any, error) {
	return c.listMaps(ctx, "/api/waf_policies")
}

// CreateWAFPolicy POSTs /api/waf_policies (or upserts via import).
func (c *Client) CreateWAFPolicy(ctx context.Context, policy map[string]any) error {
	return c.doJSON(ctx, http.MethodPost, "/api/waf_policies", policy, nil, true)
}

// UpsertWAFPolicies imports policies via projects/import.
func (c *Client) UpsertWAFPolicies(ctx context.Context, policies []map[string]any) error {
	return c.ImportData(ctx, "waf_policies", policies)
}

// ListServers GETs /api/servers.
func (c *Client) ListServers(ctx context.Context) ([]map[string]any, error) {
	return c.listMaps(ctx, "/api/servers")
}

// CreateServer POSTs /api/servers.
func (c *Client) CreateServer(ctx context.Context, server map[string]any) error {
	return c.doJSON(ctx, http.MethodPost, "/api/servers", server, nil, true)
}

// UpsertServers imports servers via projects/import.
func (c *Client) UpsertServers(ctx context.Context, servers []map[string]any) error {
	return c.ImportData(ctx, "servers", servers)
}

// ListRules GETs /api/rules.
func (c *Client) ListRules(ctx context.Context) ([]map[string]any, error) {
	return c.listMaps(ctx, "/api/rules")
}

// CreateRule POSTs /api/rules.
func (c *Client) CreateRule(ctx context.Context, rule map[string]any) error {
	return c.doJSON(ctx, http.MethodPost, "/api/rules", rule, nil, true)
}

// UpsertRules imports routing rules via projects/import.
func (c *Client) UpsertRules(ctx context.Context, rules []map[string]any) error {
	return c.ImportData(ctx, "rules", rules)
}

// DNSProvision POSTs /api/dns/provision.
func (c *Client) DNSProvision(ctx context.Context, serverID, profileID string) (map[string]any, error) {
	var out map[string]any
	err := c.doJSON(ctx, http.MethodPost, "/api/dns/provision", map[string]any{
		"server_id":  serverID,
		"profile_id": profileID,
	}, &out, true)
	return out, err
}

// DNSLookup GETs /api/dns/lookup.
func (c *Client) DNSLookup(ctx context.Context, domain string) (map[string]any, error) {
	var out map[string]any
	path := "/api/dns/lookup?domain=" + url.QueryEscape(domain)
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out, true)
	return out, err
}

// WAFTestRequest is the body for POST /api/waf/test.
type WAFTestRequest struct {
	Target      string            `json:"target"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	TimeoutMS   int               `json:"timeout_ms,omitempty"`
}

// WAFTestResponse is the relay result.
type WAFTestResponse struct {
	OK           bool     `json:"ok"`
	Error        string   `json:"error,omitempty"`
	AllowList    []string `json:"allow_list,omitempty"`
	Status       int      `json:"status"`
	Blocked      bool     `json:"blocked"`
	WAFBlock     bool     `json:"waf_block"`
	WAFRule      string   `json:"waf_rule"`
	WAFViolation string   `json:"waf_violation"`
	SupportID    string   `json:"support_id"`
	LatencyMS    int      `json:"latency_ms"`
	BodySnippet  string   `json:"body_snippet"`
	Target       string   `json:"target"`
	Path         string   `json:"path"`
	Method       string   `json:"method"`
}

// WAFTestTargets returns the SSRF allow-list from GET /api/waf/test/targets.
func (c *Client) WAFTestTargets(ctx context.Context) ([]string, error) {
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, "/api/waf/test/targets", nil, &raw, true); err != nil {
		return nil, err
	}
	return parseTargetList(raw)
}

func parseTargetList(raw json.RawMessage) ([]string, error) {
	var asSlice []string
	if err := json.Unmarshal(raw, &asSlice); err == nil {
		return asSlice, nil
	}
	var wrap struct {
		Data      []string `json:"data"`
		Targets   []string `json:"targets"`
		AllowList []string `json:"allow_list"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("wslproxy waf test targets: decode: %w", err)
	}
	switch {
	case len(wrap.Data) > 0:
		return wrap.Data, nil
	case len(wrap.Targets) > 0:
		return wrap.Targets, nil
	case len(wrap.AllowList) > 0:
		return wrap.AllowList, nil
	}
	return nil, nil
}

// WAFTest fires one request through the server-side relay after allow-list checks.
func (c *Client) WAFTest(ctx context.Context, req WAFTestRequest) (*WAFTestResponse, error) {
	if err := c.ensureTargetAllowed(ctx, req.Target); err != nil {
		return nil, err
	}
	var out WAFTestResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/waf/test", req, &out, true); err != nil {
		return nil, err
	}
	if !out.OK && out.Error != "" {
		return &out, fmt.Errorf("wslproxy waf test: %s", redactSecrets(out.Error))
	}
	return &out, nil
}

// ListWAFEvents GETs /api/waf_events.
func (c *Client) ListWAFEvents(ctx context.Context) ([]map[string]any, error) {
	return c.listMaps(ctx, "/api/waf_events")
}

// ImportData POSTs /api/projects/import with a percent-encoded JSON body so
// literal & and = inside rule patterns survive ngx.req.get_post_args form parsing.
func (c *Client) ImportData(ctx context.Context, dataType string, data []map[string]any) error {
	payload := map[string]any{
		"dataType":   dataType,
		"data":       data,
		"envProfile": c.cfg.Profile,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	encoded := []byte(url.QueryEscape(string(raw)))
	return c.doRaw(ctx, http.MethodPost, "/api/projects/import", encoded, "application/x-www-form-urlencoded", nil, true)
}

func (c *Client) ensureTargetAllowed(ctx context.Context, target string) error {
	host := hostFromTarget(target)
	if host == "" {
		return fmt.Errorf("wslproxy waf test: invalid target %q", target)
	}
	if len(c.cfg.TargetAllow) > 0 {
		ok := false
		for _, a := range c.cfg.TargetAllow {
			if host == a || strings.EqualFold(host, a) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("wslproxy waf test: target host %q is not on WSLPROXY_TARGET_ALLOWLIST %v", host, c.cfg.TargetAllow)
		}
	}
	list, err := c.WAFTestTargets(ctx)
	if err != nil {
		return fmt.Errorf("wslproxy waf test: could not load allow-list: %w", err)
	}
	if len(list) == 0 {
		return fmt.Errorf("wslproxy waf test: empty allow-list from /api/waf/test/targets")
	}
	for _, entry := range list {
		if hostFromTarget(entry) == host {
			return nil
		}
	}
	return fmt.Errorf("wslproxy waf test: target host %q is not on the WAF test allow-list %v", host, list)
}

func hostFromTarget(target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return ""
	}
	if !strings.Contains(t, "://") {
		t = "https://" + t
	}
	u, err := url.Parse(t)
	if err != nil {
		return strings.ToLower(strings.TrimRight(target, "/"))
	}
	return strings.ToLower(u.Hostname())
}

func (c *Client) listMaps(ctx context.Context, path string) ([]map[string]any, error) {
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &raw, true); err != nil {
		return nil, err
	}
	var asSlice []map[string]any
	if err := json.Unmarshal(raw, &asSlice); err == nil {
		return asSlice, nil
	}
	var wrap struct {
		Data  []map[string]any `json:"data"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("wslproxy decode %s: %w", path, err)
	}
	if wrap.Data != nil {
		return wrap.Data, nil
	}
	return wrap.Items, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, dest any, auth bool) error {
	var payload []byte
	var ctype string
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
		ctype = "application/json"
	}
	return c.doRaw(ctx, method, path, payload, ctype, dest, auth)
}

func (c *Client) doRaw(ctx context.Context, method, path string, payload []byte, contentType string, dest any, auth bool) error {
	if auth && c.token() == "" {
		if err := c.Login(ctx); err != nil {
			return err
		}
	}
	if err := c.doOnce(ctx, method, path, payload, contentType, dest, auth); err != nil {
		if auth && isUnauthorized(err) && c.cfg.APIToken == "" {
			if lerr := c.Login(ctx); lerr != nil {
				return lerr
			}
			return c.doOnce(ctx, method, path, payload, contentType, dest, auth)
		}
		return err
	}
	return nil
}

func isUnauthorized(err error) bool {
	return err != nil && strings.Contains(err.Error(), "HTTP 401")
}

func (c *Client) doOnce(ctx context.Context, method, path string, payload []byte, contentType string, dest any, auth bool) error {
	u := c.cfg.BaseURL + path
	var rdr io.Reader
	if payload != nil {
		rdr = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	// Mutating requests need x-platform for the instance-lock gate.
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set("x-platform", c.cfg.Platform)
		if contentType == "" {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	if auth {
		if tok := c.token(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wslproxy %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		msg := redactSecrets(summarizeErrorBody(string(data)))
		return fmt.Errorf("wslproxy %s %s: HTTP %d: %s", method, path, resp.StatusCode, msg)
	}
	if dest == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("wslproxy decode %s: %w", path, err)
	}
	return nil
}

// summarizeErrorBody reduces a failed response body to a line a person can
// read in the run history. wslproxy answers 5xx with a full HTML error page —
// several KB of inline CSS — and putting that in the error buried the actual
// failure behind a wall of stylesheet in the task log.
func summarizeErrorBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return "(empty response body)"
	}
	// wslproxy's structured errors: {"error":{"message":..,"code":..}}.
	var probe struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &probe) == nil {
		if m := strings.TrimSpace(probe.Error.Message); m != "" {
			if probe.Error.Code != "" {
				return m + " (" + probe.Error.Code + ")"
			}
			return m
		}
		if m := strings.TrimSpace(probe.Message); m != "" {
			return m
		}
	}
	if lower := strings.ToLower(body); strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html") {
		if m := reHTMLTitle.FindStringSubmatch(body); len(m) == 2 {
			if t := strings.TrimSpace(m[1]); t != "" {
				return "HTML error page from wslproxy (" + t + ") — check the wslproxy error log for the real cause"
			}
		}
		return "HTML error page from wslproxy — check the wslproxy error log for the real cause"
	}
	if len(body) > 400 {
		return body[:400] + "… (truncated)"
	}
	return body
}

// redactSecrets strips bearer tokens and password-like values from error text.
func redactSecrets(s string) string {
	out := reBearer.ReplaceAllString(s, "Bearer [REDACTED]")
	out = rePassword.ReplaceAllString(out, `"password":"[REDACTED]"`)
	out = reAccess.ReplaceAllString(out, `"accessToken":"[REDACTED]"`)
	out = reAPIToken.ReplaceAllString(out, `"api_token":"[REDACTED]"`)
	return out
}
