package abtest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/wslproxymcp"
)

func TestDemoListAndObserve(t *testing.T) {
	c := NewClient(nil, nil)
	if !c.Enabled() {
		t.Fatal("demo client should be enabled")
	}
	if c.LiveConfigured() {
		t.Fatal("nil mcp should not be live")
	}
	list, mode, err := c.ListExperiments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mode != "demo" || len(list) != 1 {
		t.Fatalf("mode=%s len=%d", mode, len(list))
	}
	if !list[0].Writable {
		t.Fatal("demo experiment should accept demo writes")
	}
	out, err := c.Observe(context.Background(), "abtesting", 10)
	if err != nil {
		t.Fatal(err)
	}
	if out["mode"] != "demo" {
		t.Fatalf("observe mode=%v", out["mode"])
	}
	counts, _ := out["counts"].(map[string]int)
	if counts["v1"]+counts["v2"] != 10 {
		t.Fatalf("counts=%v", counts)
	}
	res, err := c.SetWeights(context.Background(), "abtesting", []wslproxymcp.BackendWeight{{Label: "v1", Weight: 50}, {Label: "v2", Weight: 50}})
	if err != nil || res["write_live"] != false {
		t.Fatalf("demo set weights = %v, %v", res, err)
	}
	if st := c.Status(context.Background()); st["mode"] != ModeDemo {
		t.Fatalf("status mode = %v", st["mode"])
	}
}

// fakeEdge is an in-memory wslproxy admin API.
type fakeEdge struct {
	servers map[string]map[string]any
	rules   map[string]map[string]any
	writes  []string
}

func (f *fakeEdge) Enabled() bool { return true }
func (f *fakeEdge) GetServer(_ context.Context, id, _ string) (map[string]any, error) {
	return f.servers[id], nil
}
func (f *fakeEdge) GetRule(_ context.Context, id, _ string) (map[string]any, error) {
	return f.rules[id], nil
}
func (f *fakeEdge) UpdateTrafficWeights(_ context.Context, ruleID, _ string, _ []wslproxymcp.BackendWeight) (map[string]any, error) {
	f.writes = append(f.writes, "weights:"+ruleID)
	return map[string]any{"rule_id": ruleID}, nil
}
func (f *fakeEdge) PromoteTrafficBackend(_ context.Context, ruleID, label, _ string) (map[string]any, error) {
	f.writes = append(f.writes, "promote:"+ruleID+":"+label)
	return map[string]any{}, nil
}
func (f *fakeEdge) RollbackTrafficBackend(_ context.Context, ruleID, _ string) (map[string]any, error) {
	f.writes = append(f.writes, "rollback:"+ruleID)
	return map[string]any{}, nil
}

func rule(id string, servers []any, backends ...map[string]any) map[string]any {
	bs := make([]any, 0, len(backends))
	for _, b := range backends {
		bs = append(bs, b)
	}
	return map[string]any{
		"id": id, "name": id + "-name", "profile_id": "prod", "servers": servers,
		"match": map[string]any{"response": map[string]any{"backends": bs}},
	}
}

func newLiveClient(t *testing.T, edge Edge, mcpURL string) *Client {
	t.Helper()
	t.Setenv("WSLPROXY_AB_DEMO_HOST", "ab.example.org")
	t.Setenv("WSLPROXY_AB_RULE_ID", "")
	var m *wslproxymcp.Client
	if mcpURL != "" {
		m = wslproxymcp.NewClient(wslproxymcp.Config{Enabled: true, BaseURL: mcpURL, JSONRPCPath: "/mcp/jsonrpc"})
	}
	return NewClient(m, edge)
}

// mcpServer answers like wslproxy: tools/list returns {} when tools are off,
// and tools/call returns -32002.
func mcpServer(t *testing.T, toolsOn bool, calls *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int64          `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "initialize":
			resp["result"] = map[string]any{"protocolVersion": "2025-03-26"}
		case "tools/list":
			if toolsOn {
				resp["result"] = map[string]any{"tools": []any{map[string]any{"name": "update_traffic_split"}, map[string]any{"name": "promote_backend"}, map[string]any{"name": "rollback_backend"}}}
			} else {
				resp["result"] = json.RawMessage(`{"tools":{}}`)
			}
		case "tools/call":
			name, _ := req.Params["name"].(string)
			*calls = append(*calls, name)
			if toolsOn {
				resp["result"] = map[string]any{"content": []any{map[string]any{"type": "text", "text": `{"ok":true}`}}}
			} else {
				resp["error"] = map[string]any{"code": -32002, "message": "MCP tools are disabled. Set mcp.tools_enabled=true in settings.json"}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestSharedSingleBackendRuleIsReadOnly(t *testing.T) {
	edge := &fakeEdge{
		servers: map[string]map[string]any{"host:ab.example.org": {"id": "host:ab.example.org", "rules": []any{"r1"}}},
		rules: map[string]map[string]any{"r1": rule("r1",
			[]any{"host:jenkins.example.org", "host:ab.example.org", "host:shop.example.org"},
			map[string]any{"address": "http://10.0.0.1:8888", "label": "stable, ingress", "weight": 100.0})},
	}
	var calls []string
	srv := mcpServer(t, false, &calls)
	defer srv.Close()
	c := newLiveClient(t, edge, srv.URL)
	ctx := context.Background()

	ex, err := c.GetExperiment(ctx, "abtesting")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Writable {
		t.Fatal("shared rule must not be writable")
	}
	if !strings.Contains(ex.ReadOnlyReason, "shared with 2 other host(s)") || !strings.Contains(ex.ReadOnlyReason, "jenkins.example.org") {
		t.Fatalf("reason = %q", ex.ReadOnlyReason)
	}
	if len(ex.Backends) != 1 || ex.Backends[0].Weight != 100 || ex.RuleID != "r1" {
		t.Fatalf("experiment = %+v", ex)
	}

	_, err = c.SetWeights(ctx, "abtesting", []wslproxymcp.BackendWeight{{Label: "stable, ingress", Weight: 50}})
	var nw *NotWritableError
	if !errors.As(err, &nw) {
		t.Fatalf("want NotWritableError, got %v", err)
	}
	if _, err := c.Promote(ctx, "abtesting", ""); !errors.As(err, &nw) {
		t.Fatalf("promote: want NotWritableError, got %v", err)
	}
	if _, err := c.Rollback(ctx, "abtesting"); !errors.As(err, &nw) {
		t.Fatalf("rollback: want NotWritableError, got %v", err)
	}
	if len(edge.writes) != 0 || len(calls) != 0 {
		t.Fatalf("nothing may be written: rest=%v mcp=%v", edge.writes, calls)
	}

	st := c.Status(ctx)
	if st["mode"] != ModeLive || st["write_path"] != PathREST {
		t.Fatalf("status = %v", st)
	}
	if msg, _ := st["message"].(string); !strings.Contains(msg, "tools are disabled") {
		t.Fatalf("status message should say MCP tools are disabled: %q", msg)
	}
}

func TestOwnSplitRuleWritesOverRESTWhenMCPToolsOff(t *testing.T) {
	edge := &fakeEdge{
		servers: map[string]map[string]any{"host:ab.example.org": {"rules": "r2"}},
		rules: map[string]map[string]any{"r2": rule("r2", []any{"host:ab.example.org"},
			map[string]any{"address": "v1:80", "label": "v1", "weight": 80.0},
			map[string]any{"address": "v2:80", "label": "v2", "weight": 20.0})},
	}
	var calls []string
	srv := mcpServer(t, false, &calls)
	defer srv.Close()
	c := newLiveClient(t, edge, srv.URL)
	ctx := context.Background()

	ex, err := c.GetExperiment(ctx, "ab.example.org")
	if err != nil {
		t.Fatal(err)
	}
	if !ex.Writable || ex.WritePath != PathREST {
		t.Fatalf("experiment = %+v", ex)
	}
	out, err := c.SetWeights(ctx, "abtesting", []wslproxymcp.BackendWeight{{Label: "v1", Weight: 50}, {Label: "v2", Weight: 50}})
	if err != nil {
		t.Fatal(err)
	}
	if out["write_path"] != PathREST || len(edge.writes) != 1 || len(calls) != 0 {
		t.Fatalf("out=%v rest=%v mcp=%v", out, edge.writes, calls)
	}
	if _, err := c.SetWeights(ctx, "abtesting", []wslproxymcp.BackendWeight{{Label: "v3", Weight: 50}}); !errors.As(err, new(*InvalidRequestError)) {
		t.Fatalf("unknown label: want InvalidRequestError, got %v", err)
	}
	if _, err := c.Promote(ctx, "abtesting", ""); err != nil {
		t.Fatal(err)
	}
	if edge.writes[len(edge.writes)-1] != "promote:r2:v2" {
		t.Fatalf("empty label should promote the second backend: %v", edge.writes)
	}
}

func TestWritesUseMCPWhenToolsEnabled(t *testing.T) {
	edge := &fakeEdge{
		servers: map[string]map[string]any{"host:ab.example.org": {"rules": []any{"r3"}}},
		rules: map[string]map[string]any{"r3": rule("r3", []any{"host:ab.example.org"},
			map[string]any{"address": "a", "label": "stable", "weight": 90.0},
			map[string]any{"address": "b", "label": "canary", "weight": 10.0})},
	}
	var calls []string
	srv := mcpServer(t, true, &calls)
	defer srv.Close()
	c := newLiveClient(t, edge, srv.URL)

	out, err := c.Rollback(context.Background(), "abtesting")
	if err != nil {
		t.Fatal(err)
	}
	if out["write_path"] != PathMCP || len(calls) != 1 || calls[0] != "rollback_backend" || len(edge.writes) != 0 {
		t.Fatalf("out=%v mcp=%v rest=%v", out, calls, edge.writes)
	}
}

func TestMissingServerAndRuleOverride(t *testing.T) {
	edge := &fakeEdge{
		servers: map[string]map[string]any{},
		rules:   map[string]map[string]any{},
	}
	c := newLiveClient(t, edge, "")
	ex, err := c.GetExperiment(context.Background(), "abtesting")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Writable || !strings.Contains(ex.ReadOnlyReason, "has no server") {
		t.Fatalf("experiment = %+v", ex)
	}

	edge.servers["host:ab.example.org"] = map[string]any{"rules": []any{"r1"}}
	edge.rules["other"] = rule("other", []any{}, map[string]any{"label": "a"}, map[string]any{"label": "b"})
	c.ruleID = "other"
	ex, _ = c.GetExperiment(context.Background(), "abtesting")
	if ex.Writable || !strings.Contains(ex.ReadOnlyReason, "not attached") {
		t.Fatalf("unattached override must be read-only: %+v", ex)
	}
}

func TestMCPOnlyWithoutToolsIsReadOnly(t *testing.T) {
	var calls []string
	srv := mcpServer(t, false, &calls)
	defer srv.Close()
	c := newLiveClient(t, nil, srv.URL)
	st := c.Status(context.Background())
	if st["mode"] != ModeReadOnly || st["ok"] != false {
		t.Fatalf("status = %v", st)
	}
	ex, _ := c.GetExperiment(context.Background(), "abtesting")
	if ex.Writable || ex.ReadOnlyReason == "" {
		t.Fatalf("experiment = %+v", ex)
	}
}
