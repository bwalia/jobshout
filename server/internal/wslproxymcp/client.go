package wslproxymcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/jobshout/server/internal/mcp"
)

// Client wraps JobShout's generic MCP client against wslproxy's /mcp/jsonrpc.
type Client struct {
	cfg Config
	raw *mcp.Client

	mu     sync.Mutex
	inited bool
}

// NewClient builds a wslproxy MCP client. Returns a disabled client when
// BaseURL is empty so callers can check Enabled() without nil guards.
func NewClient(cfg Config) *Client {
	c := &Client{cfg: cfg}
	if !cfg.Enabled || cfg.BaseURL == "" {
		return c
	}
	url := cfg.BaseURL + cfg.JSONRPCPath
	headers := map[string]string{}
	if cfg.APIKey != "" {
		headers[cfg.APIKeyHeader] = cfg.APIKey
	}
	raw := mcp.NewClientWithHeaders(url, headers).WithTimeout(cfg.Timeout)
	// Never follow redirects: a missing /mcp nginx location on the Next.js
	// admin port 307s to /login, and re-POSTing login surfaces as HTTP 405.
	raw.DisallowRedirects()
	c.raw = raw
	return c
}

// Enabled reports whether MCP is configured enough to dial wslproxy.
func (c *Client) Enabled() bool {
	return c != nil && c.cfg.Enabled && c.cfg.BaseURL != "" && c.raw != nil
}

// BaseURL returns the configured admin base URL.
func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.cfg.BaseURL
}

func (c *Client) ensure(ctx context.Context) error {
	if !c.Enabled() {
		return fmt.Errorf("wslproxy mcp: not configured (set WSLPROXY_BASE_URL and optionally WSLPROXY_MCP_API_KEY)")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inited {
		return nil
	}
	if err := c.raw.Initialize(ctx); err != nil {
		return fmt.Errorf("wslproxy mcp initialize: %w", err)
	}
	c.inited = true
	return nil
}

// ListTools returns tools advertised by the wslproxy MCP server.
func (c *Client) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	if err := c.ensure(ctx); err != nil {
		return nil, err
	}
	return c.raw.ListTools(ctx)
}

// CallTool invokes a named MCP tool and returns the text content.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	if err := c.ensure(ctx); err != nil {
		return "", err
	}
	return c.raw.CallTool(ctx, name, args)
}

// CallJSON invokes a tool and unmarshals its text content as JSON into dest.
func (c *Client) CallJSON(ctx context.Context, name string, args map[string]any, dest any) error {
	text, err := c.CallTool(ctx, name, args)
	if err != nil {
		return err
	}
	text = strings.TrimSpace(text)
	if dest == nil || text == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(text), dest); err != nil {
		// Some tools wrap JSON in a prose envelope — try to extract the object.
		if i := strings.Index(text, "{"); i >= 0 {
			if j := strings.LastIndex(text, "}"); j > i {
				if err2 := json.Unmarshal([]byte(text[i:j+1]), dest); err2 == nil {
					return nil
				}
			}
		}
		return fmt.Errorf("wslproxy mcp %s: decode result: %w (body=%q)", name, err, truncate(text, 200))
	}
	return nil
}

// BackendWeight is one entry for update_traffic_split.
type BackendWeight struct {
	Label  string  `json:"label"`
	Weight float64 `json:"weight"`
}

// UpdateTrafficSplit calls MCP tool update_traffic_split.
func (c *Client) UpdateTrafficSplit(ctx context.Context, ruleID, profileID string, backends []BackendWeight) (string, error) {
	if profileID == "" {
		profileID = "prod"
	}
	args := map[string]any{
		"rule_id":    ruleID,
		"profile_id": profileID,
		"backends":   backends,
	}
	return c.CallTool(ctx, "update_traffic_split", args)
}

// PromoteBackend calls MCP tool promote_backend.
func (c *Client) PromoteBackend(ctx context.Context, ruleID, promoteLabel, profileID string) (string, error) {
	if profileID == "" {
		profileID = "prod"
	}
	return c.CallTool(ctx, "promote_backend", map[string]any{
		"rule_id":       ruleID,
		"promote_label": promoteLabel,
		"profile_id":    profileID,
	})
}

// RollbackBackend calls MCP tool rollback_backend.
func (c *Client) RollbackBackend(ctx context.Context, ruleID, profileID string) (string, error) {
	if profileID == "" {
		profileID = "prod"
	}
	return c.CallTool(ctx, "rollback_backend", map[string]any{
		"rule_id":    ruleID,
		"profile_id": profileID,
	})
}

// BindWAFPolicy calls MCP tool bind_waf_policy.
func (c *Client) BindWAFPolicy(ctx context.Context, serverID, policyID, profileID string) (string, error) {
	if profileID == "" {
		profileID = "prod"
	}
	return c.CallTool(ctx, "bind_waf_policy", map[string]any{
		"server_id":     serverID,
		"waf_policy_id": policyID,
		"profile_id":    profileID,
	})
}

// CodeToolsDisabled is the JSON-RPC error wslproxy returns from tools/call when
// mcp.tools_enabled is false or mcp.mode is not read-write.
const CodeToolsDisabled = -32002

// ToolsDisabledHint is what an operator has to change on wslproxy.
const ToolsDisabledHint = "set mcp.tools_enabled=true and mcp.mode=\"read-write\" in wslproxy settings.json"

// IsToolsDisabled reports whether err is wslproxy refusing a tool call because
// its MCP tools are switched off.
func IsToolsDisabled(err error) bool {
	code, ok := mcp.ErrorCode(err)
	return ok && code == CodeToolsDisabled
}

// Probe is what the MCP endpoint can actually do right now.
type Probe struct {
	Reachable bool
	Tools     []string
	Err       error
}

// Has reports whether every named tool is advertised.
func (p Probe) Has(names ...string) bool {
	for _, n := range names {
		found := false
		for _, t := range p.Tools {
			if t == n {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return len(names) > 0
}

// Message is a one-line, secret-free description of the probe.
func (p Probe) Message() string {
	switch {
	case p.Err != nil:
		return "wslproxy MCP unreachable: " + p.Err.Error()
	case len(p.Tools) == 0:
		return "wslproxy MCP is reachable but advertises no tools — its tools are disabled (" + ToolsDisabledHint + ")"
	default:
		return fmt.Sprintf("wslproxy MCP: %d tools available", len(p.Tools))
	}
}

// ProbeTools initializes the session and lists tools. A server that answers
// with an empty list is reachable but cannot mutate anything.
func (c *Client) ProbeTools(ctx context.Context) Probe {
	if !c.Enabled() {
		return Probe{Err: fmt.Errorf("not configured")}
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		return Probe{Err: err}
	}
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	return Probe{Reachable: true, Tools: names}
}

// Status returns a safe status map for Ready / HTTP status endpoints.
func (c *Client) Status(ctx context.Context) map[string]any {
	out := map[string]any{
		"enabled":            c.Enabled(),
		"base_url":           c.BaseURL(),
		"api_key_configured": c != nil && c.cfg.APIKey != "",
	}
	if c != nil {
		out["jsonrpc_path"] = c.cfg.JSONRPCPath
	}
	if !c.Enabled() {
		out["ok"] = false
		out["reachable"] = false
		out["tools_enabled"] = false
		out["message"] = "wslproxy MCP not configured"
		return out
	}
	p := c.ProbeTools(ctx)
	out["reachable"] = p.Reachable
	out["tools_enabled"] = len(p.Tools) > 0
	out["tool_count"] = len(p.Tools)
	out["tools"] = p.Tools
	out["ok"] = p.Reachable && len(p.Tools) > 0
	out["message"] = p.Message()
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
