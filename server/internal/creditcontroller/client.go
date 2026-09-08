package creditcontroller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Client talks to the AIVC reference agents HTTP surface (aivc-agents on :8000).
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

// LoadConfig reads AIVC_* settings. Empty BaseURL disables the client.
func LoadConfig() Config {
	timeout := 180 * time.Second
	if v := strings.TrimSpace(os.Getenv("AIVC_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("AIVC_BASE_URL")), "/")
	if base == "" {
		base = "http://127.0.0.1:8000"
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
		BaseURL: base,
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

// Enabled reports whether a base URL is configured.
func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != ""
}

// Ping checks /healthz.
func (c *Client) Ping(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/healthz", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListInvoices(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/v1/ap/invoices", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetInvoice(ctx context.Context, invoiceID string) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/v1/ap/invoices/"+invoiceID, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreditControllerSummary(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/v1/ap/credit-controller/summary", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GenerateInvoices(ctx context.Context, cadence string, count int) (map[string]any, error) {
	if cadence == "" {
		cadence = "monthly"
	}
	if count < 1 {
		count = 10
	}
	body := map[string]any{"cadence": cadence, "count": count}
	var out map[string]any
	if err := c.post(ctx, "/v1/ap/invoices/generate", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Triage(ctx context.Context, invoiceID string) (map[string]any, error) {
	body := map[string]any{"invoice_id": invoiceID}
	var out map[string]any
	if err := c.post(ctx, "/v1/ap/triage", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) TriageBatch(ctx context.Context, invoiceIDs []string) (map[string]any, error) {
	body := map[string]any{"invoice_ids": invoiceIDs}
	var out map[string]any
	if err := c.post(ctx, "/v1/ap/triage/batch", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Queue(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/v1/ap/queue", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Approve(ctx context.Context, runID string, approved bool, approver, note string) (map[string]any, error) {
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
	// Approve as controller identity for SoD demos.
	prevUser, prevRoles := c.user, c.roles
	c.user, c.roles = "s.oyelaran", "finance"
	err := c.post(ctx, "/v1/ap/approve", body, &out)
	c.user, c.roles = prevUser, prevRoles
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	c.auth(req)
	return c.do(req, dest)
}

func (c *Client) post(ctx context.Context, path string, body any, dest any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)
	return c.do(req, dest)
}

func (c *Client) auth(req *http.Request) {
	req.Header.Set("X-User", c.user)
	req.Header.Set("X-Roles", c.roles)
	req.Header.Set("X-Scopes", c.scopes)
	req.Header.Set("X-Tenant", c.tenant)
}

func (c *Client) do(req *http.Request, dest any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("aivc %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("aivc %s %s: HTTP %d: %s", req.Method, req.URL.Path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if dest == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("aivc decode: %w", err)
	}
	return nil
}
