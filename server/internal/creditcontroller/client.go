package creditcontroller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// Client talks to the AIVC reference agents HTTP surface (aivc-agents on :8000).
//
// With no AIVC_BASE_URL the client serves demo fixtures for every read and
// refuses mutations with a clear message, like the Simpro agent's demo mode.
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	user       string
	roles      string
	scopes     string
	tenant     string
}

// Config is loaded from the environment.
type Config struct {
	BaseURL string
	User    string
	Roles   string
	Scopes  string
	Tenant  string
	Timeout time.Duration
}

// LoadConfig reads AIVC_* settings. Empty BaseURL keeps the agent in demo mode.
// There is deliberately no localhost default: a deployed API has no aivc-agents
// sidecar, and dialling 127.0.0.1 there only produces connection-refused 502s.
func LoadConfig() Config {
	timeout := 180 * time.Second
	if v := strings.TrimSpace(os.Getenv("AIVC_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}
	user := os.Getenv("AIVC_USER")
	if user == "" {
		user = "ap.clerk"
	}
	roles := os.Getenv("AIVC_ROLES")
	if roles == "" {
		roles = "finance"
	}
	scopes := os.Getenv("AIVC_SCOPES")
	if scopes == "" {
		scopes = "corpus:read,warehouse:read,ap:read"
	}
	tenant := os.Getenv("AIVC_TENANT")
	if tenant == "" {
		tenant = "northgate"
	}
	return Config{
		BaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("AIVC_BASE_URL")), "/"),
		User:    user,
		Roles:   roles,
		Scopes:  scopes,
		Tenant:  tenant,
		Timeout: timeout,
	}
}

// NewClient builds an AIVC HTTP client.
func NewClient(cfg Config, logger *zap.Logger) *Client {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
		user:   cfg.User,
		roles:  cfg.Roles,
		scopes: cfg.Scopes,
		tenant: cfg.Tenant,
	}
}

// Enabled is always true for a real client — demo fixtures keep the tab usable
// without the aivc-agents runtime.
func (c *Client) Enabled() bool { return c != nil }

// LiveConfigured reports whether AIVC_BASE_URL points at an aivc-agents runtime.
func (c *Client) LiveConfigured() bool {
	return c != nil && c.baseURL != ""
}

// Mode returns demo | live.
func (c *Client) Mode() string {
	if c.LiveConfigured() {
		return "live"
	}
	return "demo"
}

// BaseURL returns the configured aivc-agents URL ("" in demo mode).
func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

// Error is a failed Credit Controller call, worded for the UI.
//
// Status is the HTTP status the JobShout API should answer with: the upstream
// 4xx when aivc-agents rejected the request, 404/409 for demo-mode refusals,
// and 0 when the runtime was unreachable or broken (the handler maps that to 502).
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

// Status describes the runtime for the UI and the Ready check.
func (c *Client) Status(ctx context.Context) map[string]any {
	out := map[string]any{
		"mode":            c.Mode(),
		"live_configured": c.LiveConfigured(),
		"write_actions":   writeCapability(c.LiveConfigured()),
		"ok":              true,
	}
	if !c.LiveConfigured() {
		out["message"] = demoStatusMessage
		return out
	}
	out["base_url"] = c.baseURL
	health, err := c.Ping(ctx)
	if err != nil {
		out["ok"] = false
		out["message"] = err.Error()
		return out
	}
	out["aivc"] = health
	out["message"] = "Connected to aivc-agents at " + c.baseURL + "."
	return out
}

// Ping checks /healthz.
func (c *Client) Ping(ctx context.Context) (map[string]any, error) {
	if !c.LiveConfigured() {
		return map[string]any{"status": "demo"}, nil
	}
	var out map[string]any
	if err := c.get(ctx, "/healthz", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListInvoices(ctx context.Context) (map[string]any, error) {
	if !c.LiveConfigured() {
		return demoInvoiceList(), nil
	}
	var out map[string]any
	if err := c.get(ctx, "/v1/ap/invoices", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetInvoice(ctx context.Context, invoiceID string) (map[string]any, error) {
	if !c.LiveConfigured() {
		inv, ok := demoInvoice(invoiceID)
		if !ok {
			return nil, &Error{Status: http.StatusNotFound, Msg: fmt.Sprintf("invoice %s not found in the demo mailbox", invoiceID)}
		}
		return inv, nil
	}
	var out map[string]any
	if err := c.get(ctx, "/v1/ap/invoices/"+url.PathEscape(invoiceID), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreditControllerSummary(ctx context.Context) (map[string]any, error) {
	if !c.LiveConfigured() {
		return demoSummary(), nil
	}
	var out map[string]any
	if err := c.get(ctx, "/v1/ap/credit-controller/summary", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GenerateInvoices(ctx context.Context, cadence string, count int) (map[string]any, error) {
	if !c.LiveConfigured() {
		return nil, demoWriteError("AI invoice generation")
	}
	if cadence == "" {
		cadence = "monthly"
	}
	if count < 1 {
		count = 10
	}
	body := map[string]any{"cadence": cadence, "count": count}
	var out map[string]any
	if err := c.post(ctx, "/v1/ap/invoices/generate", body, &out, c.user, c.roles); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Triage(ctx context.Context, invoiceID string) (map[string]any, error) {
	if !c.LiveConfigured() {
		return nil, demoWriteError("Triage")
	}
	body := map[string]any{"invoice_id": invoiceID}
	var out map[string]any
	if err := c.post(ctx, "/v1/ap/triage", body, &out, c.user, c.roles); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) TriageBatch(ctx context.Context, invoiceIDs []string) (map[string]any, error) {
	if !c.LiveConfigured() {
		return nil, demoWriteError("Batch triage")
	}
	body := map[string]any{"invoice_ids": invoiceIDs}
	var out map[string]any
	if err := c.post(ctx, "/v1/ap/triage/batch", body, &out, c.user, c.roles); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Queue(ctx context.Context) (map[string]any, error) {
	if !c.LiveConfigured() {
		return demoQueue(), nil
	}
	var out map[string]any
	if err := c.get(ctx, "/v1/ap/queue", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Approve(ctx context.Context, runID string, approved bool, approver, note string) (map[string]any, error) {
	if !c.LiveConfigured() {
		return nil, demoWriteError("Approval")
	}
	if approver == "" {
		approver = "s.oyelaran"
	}
	body := map[string]any{
		"run_id":   runID,
		"approved": approved,
		"approver": approver,
		"note":     note,
	}
	var out map[string]any
	// Approve as the controller identity so segregation-of-duties holds: the
	// clerk who triaged cannot approve. Passed per request, not by mutating c.
	if err := c.post(ctx, "/v1/ap/approve", body, &out, "s.oyelaran", "finance"); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	c.auth(req, c.user, c.roles)
	return c.do(req, dest)
}

func (c *Client) post(ctx context.Context, path string, body any, dest any, user, roles string) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req, user, roles)
	return c.do(req, dest)
}

func (c *Client) auth(req *http.Request, user, roles string) {
	req.Header.Set("X-User", user)
	req.Header.Set("X-Roles", roles)
	req.Header.Set("X-Scopes", c.scopes)
	req.Header.Set("X-Tenant", c.tenant)
}

func (c *Client) do(req *http.Request, dest any) error {
	op := req.Method + " " + req.URL.Path
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Warn("aivc request failed", zap.String("op", op), zap.Error(err))
		return &Error{Msg: fmt.Sprintf(
			"Credit Controller runtime (aivc-agents) is unreachable at %s: %s. Check AIVC_BASE_URL and that aivc-agents is running.",
			c.baseURL, transportReason(err))}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return &Error{Msg: fmt.Sprintf("aivc-agents %s: reading response: %s", op, transportReason(err))}
	}
	if resp.StatusCode >= 300 {
		e := &Error{Msg: fmt.Sprintf("aivc-agents %s returned HTTP %d: %s", op, resp.StatusCode, upstreamDetail(data))}
		// Pass through request-shaped rejections. Never forward 401/403: the web
		// client treats those as an expired JobShout session and logs the user out.
		switch resp.StatusCode {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity:
			e.Status = resp.StatusCode
		}
		return e
	}
	if dest == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return &Error{Msg: fmt.Sprintf("aivc-agents %s returned a response JobShout could not read: %v", op, err)}
	}
	return nil
}

// transportReason turns a Go transport error into a short human phrase instead
// of `Get "http://…": dial tcp …: connect: connection refused`.
func transportReason(err error) string {
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.As(err, &dnsErr):
		return "host " + dnsErr.Name + " not found"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return "request timed out"
	case errors.Is(err, context.Canceled):
		return "request cancelled"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err.Error()
	}
	return err.Error()
}

// upstreamDetail extracts FastAPI-style {"detail": …} / {"error": …} messages.
func upstreamDetail(data []byte) string {
	var body map[string]any
	if json.Unmarshal(data, &body) == nil {
		for _, k := range []string{"detail", "error", "message"} {
			if v, ok := body[k]; ok && v != nil {
				if s, ok := v.(string); ok {
					return truncate(s, 300)
				}
				b, _ := json.Marshal(v)
				return truncate(string(b), 300)
			}
		}
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "empty response"
	}
	return truncate(s, 300)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
