package abtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jobshout/server/internal/wslproxymcp"
)

// Experiment describes one A/B or canary split managed via wslproxy.
type Experiment struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Host        string           `json:"host"`
	RuleID      string           `json:"rule_id"`
	ProfileID   string           `json:"profile_id"`
	Mode        string           `json:"mode"` // demo | live
	Backends    []Backend        `json:"backends"`
	ObservePath string           `json:"observe_path"`
	PublicURL   string           `json:"public_url"`
	Note        string           `json:"note,omitempty"`
}

// Backend is one weighted origin behind a rule.
type Backend struct {
	Label   string  `json:"label"`
	Weight  float64 `json:"weight"`
	Address string  `json:"address,omitempty"`
	Role    string  `json:"role,omitempty"` // stable | canary | variant
}

// ObserveSample is one hit to the public /version-style endpoint.
type ObserveSample struct {
	Variant   string `json:"variant"`
	Status    int    `json:"status"`
	LatencyMS int    `json:"latency_ms"`
	Body      string `json:"body,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Client drives A/B experiments through wslproxy MCP (+ optional public observe).
type Client struct {
	mcp    *wslproxymcp.Client
	http   *http.Client
	demo   bool
	host   string // default public demo host
}

// NewClient wraps an MCP client. When MCP is disabled, demo fixtures are used.
func NewClient(mcp *wslproxymcp.Client) *Client {
	host := strings.TrimSpace(os.Getenv("WSLPROXY_AB_DEMO_HOST"))
	if host == "" {
		host = "abtesting.fictionally.org"
	}
	return &Client{
		mcp:  mcp,
		http: &http.Client{Timeout: 10 * time.Second},
		demo: mcp == nil || !mcp.Enabled(),
		host: host,
	}
}

// Enabled always true — demo mode keeps the agent usable without MCP.
func (c *Client) Enabled() bool { return c != nil }

// LiveConfigured reports whether MCP is ready for live mutations.
func (c *Client) LiveConfigured() bool {
	return c != nil && c.mcp != nil && c.mcp.Enabled()
}

// Status for Ready / HTTP.
func (c *Client) Status(ctx context.Context) map[string]any {
	out := map[string]any{
		"enabled":          true,
		"mode":             "demo",
		"demo_host":        c.host,
		"mcp_configured":   c.LiveConfigured(),
		"public_url":       "https://" + c.host,
	}
	if c.LiveConfigured() {
		out["mode"] = "live"
		st := c.mcp.Status(ctx)
		out["mcp"] = st
		out["ok"] = st["ok"]
		out["message"] = st["message"]
	} else {
		out["ok"] = true
		out["message"] = "Demo experiment fixtures active. Set WSLPROXY_BASE_URL + WSLPROXY_MCP_API_KEY for live traffic splits."
	}
	return out
}

// ListExperiments returns configured or demo experiments.
func (c *Client) ListExperiments(ctx context.Context) ([]Experiment, string, error) {
	if !c.LiveConfigured() {
		return demoExperiments(c.host), "demo", nil
	}
	// Live: expose the fictionally AB host as the managed experiment; weights
	// come from env override or defaults until we read rule JSON via MCP resource.
	ex := liveExperiment(c.host)
	if rule := strings.TrimSpace(os.Getenv("WSLPROXY_AB_RULE_ID")); rule != "" {
		ex.RuleID = rule
	}
	_ = ctx
	return []Experiment{ex}, "live", nil
}

// GetExperiment returns one experiment by id.
func (c *Client) GetExperiment(ctx context.Context, id string) (*Experiment, error) {
	list, _, err := c.ListExperiments(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id || list[i].Host == id {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("experiment %q not found", id)
}

// SetWeights updates traffic weights via MCP (or records demo intent).
func (c *Client) SetWeights(ctx context.Context, experimentID string, backends []wslproxymcp.BackendWeight) (map[string]any, error) {
	ex, err := c.GetExperiment(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	if !c.LiveConfigured() {
		return map[string]any{
			"mode":        "demo",
			"experiment":  ex.ID,
			"backends":    backends,
			"message":     "Demo only — weights not pushed to wslproxy. Configure MCP to mutate live splits.",
			"write_live":  false,
		}, nil
	}
	text, err := c.mcp.UpdateTrafficSplit(ctx, ex.RuleID, ex.ProfileID, backends)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode":       "live",
		"experiment": ex.ID,
		"rule_id":    ex.RuleID,
		"backends":   backends,
		"result":     text,
		"write_live": true,
		"message":    "Traffic weights updated via wslproxy MCP update_traffic_split.",
	}, nil
}

// Promote shifts 100% traffic to label via MCP.
func (c *Client) Promote(ctx context.Context, experimentID, label string) (map[string]any, error) {
	ex, err := c.GetExperiment(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	if !c.LiveConfigured() {
		return map[string]any{
			"mode": "demo", "experiment": ex.ID, "promote_label": label,
			"message": "Demo only — promote not pushed.", "write_live": false,
		}, nil
	}
	text, err := c.mcp.PromoteBackend(ctx, ex.RuleID, label, ex.ProfileID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode": "live", "experiment": ex.ID, "promote_label": label,
		"result": text, "write_live": true,
		"message": "Promoted via wslproxy MCP promote_backend.",
	}, nil
}

// Rollback restores single-backend routing via MCP.
func (c *Client) Rollback(ctx context.Context, experimentID string) (map[string]any, error) {
	ex, err := c.GetExperiment(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	if !c.LiveConfigured() {
		return map[string]any{
			"mode": "demo", "experiment": ex.ID,
			"message": "Demo only — rollback not pushed.", "write_live": false,
		}, nil
	}
	text, err := c.mcp.RollbackBackend(ctx, ex.RuleID, ex.ProfileID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode": "live", "experiment": ex.ID, "result": text, "write_live": true,
		"message": "Rolled back via wslproxy MCP rollback_backend.",
	}, nil
}

// Observe hits the public observe path n times and returns samples.
func (c *Client) Observe(ctx context.Context, experimentID string, n int) (map[string]any, error) {
	if n <= 0 {
		n = 20
	}
	if n > 100 {
		n = 100
	}
	ex, err := c.GetExperiment(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	if !c.LiveConfigured() {
		return demoObserve(ex, n), nil
	}
	url := strings.TrimRight(ex.PublicURL, "/") + ex.ObservePath
	samples := make([]ObserveSample, 0, n)
	counts := map[string]int{}
	for i := 0; i < n; i++ {
		s := c.hit(ctx, url)
		samples = append(samples, s)
		if s.Variant != "" {
			counts[s.Variant]++
		} else if s.Error != "" {
			counts["error"]++
		} else {
			counts["unknown"]++
		}
	}
	expected := map[string]float64{}
	for _, b := range ex.Backends {
		expected[b.Label] = b.Weight
	}
	return map[string]any{
		"mode":       "live",
		"experiment": ex.ID,
		"url":        url,
		"n":          n,
		"counts":     counts,
		"expected":   expected,
		"samples":    samples,
		"message":    "Observed live requests through the edge host.",
	}, nil
}

func (c *Client) hit(ctx context.Context, url string) ObserveSample {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ObserveSample{Error: err.Error()}
	}
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := c.http.Do(req)
	if err != nil {
		return ObserveSample{Error: err.Error(), LatencyMS: int(time.Since(start).Milliseconds())}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	variant := parseVariant(string(body), resp.Header)
	return ObserveSample{
		Variant:   variant,
		Status:    resp.StatusCode,
		LatencyMS: int(time.Since(start).Milliseconds()),
		Body:      strings.TrimSpace(string(body)),
	}
}

func parseVariant(body string, hdr http.Header) string {
	if v := strings.TrimSpace(hdr.Get("X-AB-Variant")); v != "" {
		return strings.ToLower(v)
	}
	if v := strings.TrimSpace(hdr.Get("X-Served-By")); v != "" {
		return strings.ToLower(v)
	}
	body = strings.TrimSpace(body)
	var probe map[string]any
	if json.Unmarshal([]byte(body), &probe) == nil {
		for _, k := range []string{"variant", "version", "served_by", "branch"} {
			if v, ok := probe[k].(string); ok && v != "" {
				return strings.ToLower(v)
			}
		}
	}
	lower := strings.ToLower(body)
	switch {
	case strings.Contains(lower, `"v2"`) || strings.HasPrefix(lower, "v2") || strings.Contains(lower, "canary"):
		return "v2"
	case strings.Contains(lower, `"v1"`) || strings.HasPrefix(lower, "v1") || strings.Contains(lower, "stable"):
		return "v1"
	}
	return ""
}

func liveExperiment(host string) Experiment {
	return Experiment{
		ID:        "abtesting",
		Name:      "AB Testing demo",
		Host:      host,
		RuleID:    strings.TrimSpace(firstEnv("WSLPROXY_AB_RULE_ID", "abtesting-default")),
		ProfileID: firstEnv("WSLPROXY_PROFILE", "prod"),
		Mode:      "live",
		Backends: []Backend{
			{Label: "v1", Weight: 80, Role: "stable"},
			{Label: "v2", Weight: 20, Role: "canary"},
		},
		ObservePath: firstEnv("WSLPROXY_AB_OBSERVE_PATH", "/version"),
		PublicURL:   "https://" + host,
		Note:        "Managed through wslproxy MCP traffic tools. Replaces the standalone abtesting.fictionally.org control UI.",
	}
}

func firstEnv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
