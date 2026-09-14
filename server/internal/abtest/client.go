package abtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jobshout/server/internal/wslproxymcp"
)

// Modes reported by Status.
const (
	ModeDemo        = "demo"        // no wslproxy configured: fixtures only
	ModeLive        = "live"        // wslproxy reachable and a write path works
	ModeReadOnly    = "read_only"   // wslproxy reachable, nothing can write
	ModeUnavailable = "unavailable" // wslproxy configured but not answering
)

// Write paths.
const (
	PathMCP  = "mcp"
	PathREST = "rest"
	PathDemo = "demo"
)

// Experiment describes one A/B or canary split managed via wslproxy.
type Experiment struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	RuleID      string    `json:"rule_id"`
	RuleName    string    `json:"rule_name,omitempty"`
	ProfileID   string    `json:"profile_id"`
	Mode        string    `json:"mode"` // demo | live
	Backends    []Backend `json:"backends"`
	ObservePath string    `json:"observe_path"`
	PublicURL   string    `json:"public_url"`
	Note        string    `json:"note,omitempty"`
	// SharedWith lists other wslproxy servers attached to the same rule.
	SharedWith []string `json:"shared_with,omitempty"`
	// Writable is true only when the rule is this host's own split and a
	// write path (MCP tools or the admin API) is available.
	Writable       bool   `json:"writable"`
	ReadOnlyReason string `json:"read_only_reason,omitempty"`
	WritePath      string `json:"write_path,omitempty"`
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

// Edge is the wslproxy admin REST surface. The agent reads rules through it,
// and writes through it when MCP tools are switched off. *waflab.Client
// satisfies it with the same credentials the WAF lab uses.
type Edge interface {
	Enabled() bool
	GetServer(ctx context.Context, id, profileID string) (map[string]any, error)
	GetRule(ctx context.Context, id, profileID string) (map[string]any, error)
	UpdateTrafficWeights(ctx context.Context, ruleID, profileID string, backends []wslproxymcp.BackendWeight) (map[string]any, error)
	PromoteTrafficBackend(ctx context.Context, ruleID, label, profileID string) (map[string]any, error)
	RollbackTrafficBackend(ctx context.Context, ruleID, profileID string) (map[string]any, error)
}

// NotFoundError means no experiment matches the id.
type NotFoundError struct{ ID string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("experiment %q not found", e.ID) }

// NotWritableError means a write was refused before reaching wslproxy.
type NotWritableError struct {
	Experiment string
	Reason     string
}

func (e *NotWritableError) Error() string {
	return fmt.Sprintf("experiment %q is read-only: %s", e.Experiment, e.Reason)
}

// InvalidRequestError means the caller asked for something the rule cannot do.
type InvalidRequestError struct{ Reason string }

func (e *InvalidRequestError) Error() string { return e.Reason }

// mcpProbeTTL bounds how often Status and writes re-list MCP tools.
const mcpProbeTTL = 30 * time.Second

// Client drives A/B experiments through wslproxy (MCP or admin API) and
// observes the public host.
type Client struct {
	mcp     *wslproxymcp.Client
	edge    Edge
	http    *http.Client
	host    string
	ruleID  string // WSLPROXY_AB_RULE_ID override
	profile string
	observe string

	mu       sync.Mutex
	probe    wslproxymcp.Probe
	probedAt time.Time
}

// NewClient wraps the wslproxy MCP client and admin API. With neither
// configured the agent runs on demo fixtures.
func NewClient(mcp *wslproxymcp.Client, edge Edge) *Client {
	return &Client{
		mcp:     mcp,
		edge:    edge,
		http:    &http.Client{Timeout: 10 * time.Second},
		host:    firstEnv("WSLPROXY_AB_DEMO_HOST", "abtesting.fictionally.org"),
		ruleID:  strings.TrimSpace(os.Getenv("WSLPROXY_AB_RULE_ID")),
		profile: firstEnv("WSLPROXY_PROFILE", "prod"),
		observe: firstEnv("WSLPROXY_AB_OBSERVE_PATH", "/version"),
	}
}

// Enabled always true — demo mode keeps the agent usable without wslproxy.
func (c *Client) Enabled() bool { return c != nil }

func (c *Client) mcpConfigured() bool  { return c != nil && c.mcp != nil && c.mcp.Enabled() }
func (c *Client) edgeConfigured() bool { return c != nil && c.edge != nil && c.edge.Enabled() }

// LiveConfigured reports whether any wslproxy connection is configured. It
// says nothing about whether writes work; Status does.
func (c *Client) LiveConfigured() bool {
	return c.mcpConfigured() || c.edgeConfigured()
}

// mcpProbe returns a recent tools/list probe.
func (c *Client) mcpProbe(ctx context.Context) wslproxymcp.Probe {
	if !c.mcpConfigured() {
		return wslproxymcp.Probe{Err: errors.New("not configured")}
	}
	c.mu.Lock()
	if !c.probedAt.IsZero() && time.Since(c.probedAt) < mcpProbeTTL {
		p := c.probe
		c.mu.Unlock()
		return p
	}
	c.mu.Unlock()
	p := c.mcp.ProbeTools(ctx)
	c.mu.Lock()
	c.probe, c.probedAt = p, time.Now()
	c.mu.Unlock()
	return p
}

func (c *Client) forgetProbe() {
	c.mu.Lock()
	c.probedAt = time.Time{}
	c.mu.Unlock()
}

// writePath picks how a write for tool would be sent, or explains why none can.
func (c *Client) writePath(ctx context.Context, tool string) (string, string) {
	if c.mcpConfigured() {
		if p := c.mcpProbe(ctx); p.Has(tool) {
			return PathMCP, ""
		}
	}
	if c.edgeConfigured() {
		return PathREST, ""
	}
	if c.mcpConfigured() {
		return "", c.mcpProbe(ctx).Message() + ", and no wslproxy admin credentials are set for the REST fallback (WSLPROXY_USERNAME + WSLPROXY_PASSWORD or WSLPROXY_API_TOKEN)"
	}
	return "", "wslproxy is not configured"
}

// Status reports what the agent can do right now. Mode is live only when a
// write path works.
func (c *Client) Status(ctx context.Context) map[string]any {
	out := map[string]any{
		"enabled":        true,
		"demo_host":      c.host,
		"public_url":     "https://" + c.host,
		"profile_id":     c.profile,
		"mcp_configured": c.mcpConfigured(),
		"api_configured": c.edgeConfigured(),
	}
	if !c.LiveConfigured() {
		out["mode"] = ModeDemo
		out["ok"] = true
		out["writable"] = true
		out["write_path"] = PathDemo
		out["message"] = "Demo fixtures — nothing is sent to wslproxy. Set WSLPROXY_BASE_URL with admin credentials (or an MCP API key) for live traffic splits."
		return out
	}

	var notes []string
	mcpTools := false
	if c.mcpConfigured() {
		p := c.mcpProbe(ctx)
		mcpTools = p.Has("update_traffic_split")
		out["mcp"] = map[string]any{
			"reachable":     p.Reachable,
			"tools_enabled": len(p.Tools) > 0,
			"tool_count":    len(p.Tools),
			"message":       p.Message(),
		}
		if !mcpTools {
			notes = append(notes, p.Message())
		}
	}
	apiOK := false
	if c.edgeConfigured() {
		_, err := c.edge.GetServer(ctx, "host:"+c.host, c.profile)
		apiOK = err == nil
		api := map[string]any{"reachable": apiOK}
		if err != nil {
			api["message"] = err.Error()
			notes = append(notes, "wslproxy admin API: "+err.Error())
		}
		out["api"] = api
	}

	switch {
	case mcpTools:
		out["mode"], out["write_path"] = ModeLive, PathMCP
		out["message"] = "Live — writes go through wslproxy MCP."
	case apiOK:
		out["mode"], out["write_path"] = ModeLive, PathREST
		out["message"] = "Live — writes go through the wslproxy admin API."
	case c.mcpConfigured() && c.mcpProbe(ctx).Reachable:
		out["mode"], out["write_path"] = ModeReadOnly, ""
		out["message"] = "Read-only — nothing can change traffic weights."
	default:
		out["mode"], out["write_path"] = ModeUnavailable, ""
		out["message"] = "wslproxy is not answering."
	}
	out["writable"] = out["mode"] == ModeLive
	out["ok"] = out["mode"] == ModeLive
	if len(notes) > 0 {
		out["message"] = fmt.Sprint(out["message"], " ", strings.Join(notes, "; "))
	}
	return out
}

// ListExperiments returns the configured experiment, resolved from wslproxy.
func (c *Client) ListExperiments(ctx context.Context) ([]Experiment, string, error) {
	if !c.LiveConfigured() {
		return demoExperiments(c.host), ModeDemo, nil
	}
	ex := c.resolve(ctx)
	return []Experiment{ex}, "live", nil
}

// GetExperiment returns one experiment by id or host.
func (c *Client) GetExperiment(ctx context.Context, id string) (*Experiment, error) {
	list, _, err := c.ListExperiments(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id || strings.EqualFold(list[i].Host, id) {
			return &list[i], nil
		}
	}
	return nil, &NotFoundError{ID: id}
}

// resolve reads the host's server and rule from wslproxy. It never fabricates
// backends: anything it cannot confirm leaves the experiment read-only with
// the reason.
func (c *Client) resolve(ctx context.Context) Experiment {
	ex := Experiment{
		ID:          "abtesting",
		Name:        c.host,
		Host:        c.host,
		RuleID:      c.ruleID,
		ProfileID:   c.profile,
		Mode:        "live",
		Backends:    []Backend{},
		ObservePath: c.observe,
		PublicURL:   "https://" + c.host,
	}
	readOnly := func(reason string) Experiment {
		ex.Writable = false
		ex.ReadOnlyReason = reason
		return ex
	}

	if !c.edgeConfigured() {
		return readOnly("the agent reads the host's rule over the wslproxy admin API, which has no credentials here — set WSLPROXY_USERNAME + WSLPROXY_PASSWORD (or WSLPROXY_API_TOKEN)")
	}
	server, err := c.edge.GetServer(ctx, "host:"+c.host, c.profile)
	if err != nil {
		return readOnly("could not read wslproxy server host:" + c.host + ": " + err.Error())
	}
	if server == nil {
		return readOnly(fmt.Sprintf("wslproxy has no server %q in profile %q", "host:"+c.host, c.profile))
	}
	attached := stringList(server["rules"])

	var rule map[string]any
	switch {
	case c.ruleID != "":
		rule, err = c.edge.GetRule(ctx, c.ruleID, c.profile)
		if err != nil {
			return readOnly("could not read wslproxy rule " + c.ruleID + ": " + err.Error())
		}
		if rule == nil {
			return readOnly(fmt.Sprintf("WSLPROXY_AB_RULE_ID %q does not exist on wslproxy (profile %q)", c.ruleID, c.profile))
		}
		if !containsFold(attached, c.ruleID) {
			fillRule(&ex, rule)
			return readOnly(fmt.Sprintf("WSLPROXY_AB_RULE_ID %q is not attached to %s, so changing it would not affect this host", c.ruleID, c.host))
		}
	case len(attached) == 0:
		return readOnly(c.host + " has no wslproxy rules attached")
	default:
		// Prefer an attached rule that actually splits traffic.
		for _, id := range attached {
			r, gerr := c.edge.GetRule(ctx, id, c.profile)
			if gerr != nil {
				err = gerr
				continue
			}
			if r == nil {
				continue
			}
			if rule == nil || (len(ruleBackends(r)) >= 2 && len(ruleBackends(rule)) < 2) {
				rule = r
			}
		}
		if rule == nil {
			if err != nil {
				return readOnly("could not read the rules attached to " + c.host + ": " + err.Error())
			}
			return readOnly(fmt.Sprintf("the rules attached to %s (%s) do not exist on wslproxy", c.host, strings.Join(attached, ", ")))
		}
	}
	fillRule(&ex, rule)

	ruleLabel := ex.RuleID
	if ex.RuleName != "" {
		ruleLabel = ex.RuleName + " (" + ex.RuleID + ")"
	}
	if len(ex.SharedWith) > 0 {
		return readOnly(fmt.Sprintf(
			"%s is served by wslproxy rule %s, which is shared with %d other host(s) (%s). Changing its weights would move their traffic too. Give %s its own rule with one backend per variant to manage the split here.",
			c.host, ruleLabel, len(ex.SharedWith), strings.Join(ex.SharedWith, ", "), c.host))
	}
	if len(ex.Backends) < 2 {
		return readOnly(fmt.Sprintf(
			"wslproxy rule %s has %d backend(s), so there is no split to manage there. If Observe still shows several variants, the split happens behind wslproxy (for example in the cluster ingress), which this agent does not control.",
			ruleLabel, len(ex.Backends)))
	}
	path, why := c.writePath(ctx, "update_traffic_split")
	if path == "" {
		return readOnly(why)
	}
	ex.Writable = true
	ex.WritePath = path
	return ex
}

func fillRule(ex *Experiment, rule map[string]any) {
	if id, _ := rule["id"].(string); id != "" {
		ex.RuleID = id
	}
	ex.RuleName, _ = rule["name"].(string)
	if p, _ := rule["profile_id"].(string); p != "" {
		ex.ProfileID = p
	}
	ex.Backends = ruleBackends(rule)
	self := "host:" + strings.ToLower(ex.Host)
	ex.SharedWith = nil
	for _, s := range stringList(rule["servers"]) {
		if strings.EqualFold(s, self) || strings.EqualFold(s, ex.Host) {
			continue
		}
		ex.SharedWith = append(ex.SharedWith, strings.TrimPrefix(s, "host:"))
	}
	if ex.RuleName != "" {
		ex.Note = "wslproxy rule " + ex.RuleName
	}
}

func ruleBackends(rule map[string]any) []Backend {
	match, _ := rule["match"].(map[string]any)
	resp, _ := match["response"].(map[string]any)
	raw, _ := resp["backends"].([]any)
	out := make([]Backend, 0, len(raw))
	for i, item := range raw {
		b, _ := item.(map[string]any)
		if b == nil {
			continue
		}
		addr, _ := b["address"].(string)
		label, _ := b["label"].(string)
		if label == "" {
			label = addr // traffic_mgmt matches on label, falling back to address
		}
		w := 1.0 // traffic_mgmt's default weight
		switch v := b["weight"].(type) {
		case float64:
			w = v
		case string:
			fmt.Sscanf(v, "%g", &w)
		}
		role := "variant"
		if i == 0 {
			role = "stable"
		} else if i == 1 && len(raw) == 2 {
			role = "canary"
		}
		out = append(out, Backend{Label: label, Weight: w, Address: addr, Role: role})
	}
	return out
}

// writable resolves the experiment and refuses anything that must not be written.
func (c *Client) writable(ctx context.Context, experimentID string) (*Experiment, error) {
	ex, err := c.GetExperiment(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	if ex.Mode != ModeDemo && !ex.Writable {
		return nil, &NotWritableError{Experiment: ex.ID, Reason: ex.ReadOnlyReason}
	}
	return ex, nil
}

// SetWeights updates traffic weights (or records demo intent).
func (c *Client) SetWeights(ctx context.Context, experimentID string, backends []wslproxymcp.BackendWeight) (map[string]any, error) {
	ex, err := c.writable(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	if err := checkWeights(ex, backends); err != nil {
		return nil, err
	}
	if ex.Mode == ModeDemo {
		return map[string]any{
			"mode":       ModeDemo,
			"experiment": ex.ID,
			"backends":   backends,
			"message":    "Demo only — weights not pushed to wslproxy.",
			"write_live": false,
		}, nil
	}
	result, path, err := c.write(ctx, ex, "update_traffic_split",
		func() (any, error) { return c.mcp.UpdateTrafficSplit(ctx, ex.RuleID, ex.ProfileID, backends) },
		func() (any, error) { return c.edge.UpdateTrafficWeights(ctx, ex.RuleID, ex.ProfileID, backends) },
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode":       ModeLive,
		"experiment": ex.ID,
		"rule_id":    ex.RuleID,
		"backends":   backends,
		"result":     result,
		"write_live": true,
		"write_path": path,
		"message":    "Traffic weights updated on wslproxy via " + pathName(path) + ".",
	}, nil
}

// Promote shifts 100% of traffic to label. An empty label promotes the
// second backend (the canary in a two-way split).
func (c *Client) Promote(ctx context.Context, experimentID, label string) (map[string]any, error) {
	ex, err := c.writable(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	label = strings.TrimSpace(label)
	if label == "" && len(ex.Backends) >= 2 {
		label = ex.Backends[1].Label
	}
	if !hasLabel(ex, label) {
		return nil, &InvalidRequestError{Reason: fmt.Sprintf("backend %q is not on rule %s (backends: %s)", label, ex.RuleID, labels(ex))}
	}
	if ex.Mode == ModeDemo {
		return map[string]any{
			"mode": ModeDemo, "experiment": ex.ID, "promote_label": label,
			"message": "Demo only — promote not pushed.", "write_live": false,
		}, nil
	}
	result, path, err := c.write(ctx, ex, "promote_backend",
		func() (any, error) { return c.mcp.PromoteBackend(ctx, ex.RuleID, label, ex.ProfileID) },
		func() (any, error) { return c.edge.PromoteTrafficBackend(ctx, ex.RuleID, label, ex.ProfileID) },
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode": ModeLive, "experiment": ex.ID, "promote_label": label,
		"result": result, "write_live": true, "write_path": path,
		"message": fmt.Sprintf("Promoted %s to 100%% via %s.", label, pathName(path)),
	}, nil
}

// Rollback removes the backend split and restores single-backend routing.
func (c *Client) Rollback(ctx context.Context, experimentID string) (map[string]any, error) {
	ex, err := c.writable(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	if ex.Mode == ModeDemo {
		return map[string]any{
			"mode": ModeDemo, "experiment": ex.ID,
			"message": "Demo only — rollback not pushed.", "write_live": false,
		}, nil
	}
	result, path, err := c.write(ctx, ex, "rollback_backend",
		func() (any, error) { return c.mcp.RollbackBackend(ctx, ex.RuleID, ex.ProfileID) },
		func() (any, error) { return c.edge.RollbackTrafficBackend(ctx, ex.RuleID, ex.ProfileID) },
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode": ModeLive, "experiment": ex.ID, "result": result,
		"write_live": true, "write_path": path,
		"message": "Rolled back to single-backend routing via " + pathName(path) + ".",
	}, nil
}

// write sends through MCP when its tool is advertised, otherwise (or when
// wslproxy turns out to have tools disabled) through the admin API.
func (c *Client) write(ctx context.Context, ex *Experiment, tool string, viaMCP, viaREST func() (any, error)) (any, string, error) {
	path, why := c.writePath(ctx, tool)
	switch path {
	case PathMCP:
		out, err := viaMCP()
		if err == nil {
			return out, PathMCP, nil
		}
		if !wslproxymcp.IsToolsDisabled(err) || !c.edgeConfigured() {
			return nil, "", fmt.Errorf("wslproxy MCP %s: %w", tool, err)
		}
		c.forgetProbe()
		fallthrough
	case PathREST:
		out, err := viaREST()
		if err != nil {
			return nil, "", err
		}
		return out, PathREST, nil
	default:
		return nil, "", &NotWritableError{Experiment: ex.ID, Reason: why}
	}
}

func pathName(path string) string {
	if path == PathMCP {
		return "wslproxy MCP"
	}
	return "the wslproxy admin API"
}

func checkWeights(ex *Experiment, backends []wslproxymcp.BackendWeight) error {
	if len(backends) == 0 {
		return &InvalidRequestError{Reason: "backends required"}
	}
	for _, b := range backends {
		if b.Weight < 0 || b.Weight > 100 {
			return &InvalidRequestError{Reason: fmt.Sprintf("weight for %q must be between 0 and 100", b.Label)}
		}
		if !hasLabel(ex, b.Label) {
			return &InvalidRequestError{Reason: fmt.Sprintf("backend %q is not on rule %s (backends: %s)", b.Label, ex.RuleID, labels(ex))}
		}
	}
	return nil
}

func hasLabel(ex *Experiment, label string) bool {
	for _, b := range ex.Backends {
		if b.Label == label {
			return true
		}
	}
	return false
}

func labels(ex *Experiment) string {
	names := make([]string, 0, len(ex.Backends))
	for _, b := range ex.Backends {
		names = append(names, fmt.Sprintf("%q", b.Label))
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// Observe hits the public observe path n times and returns samples. It only
// needs the public host, so it works even when the rule is read-only.
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
	if ex.Mode == ModeDemo {
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
	variants := make([]string, 0, len(counts))
	for v := range counts {
		variants = append(variants, v)
	}
	sort.Strings(variants)
	return map[string]any{
		"mode":       "live",
		"experiment": ex.ID,
		"url":        url,
		"n":          n,
		"counts":     counts,
		"variants":   variants,
		"expected":   expected,
		"samples":    samples,
		"message":    fmt.Sprintf("Observed %d live requests through %s.", n, ex.Host),
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

// stringList reads a JSON value that wslproxy stores as either a list or a
// single string.
func stringList(v any) []string {
	switch t := v.(type) {
	case string:
		if s := strings.TrimSpace(t); s != "" {
			return []string{s}
		}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	}
	return nil
}

func containsFold(list []string, s string) bool {
	for _, item := range list {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}

func firstEnv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
