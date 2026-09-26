package wslproxymcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateTrafficSplit(t *testing.T) {
	var gotName string
	var gotArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-MCP-API-Key") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			ID     int64          `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18"}
		case "tools/call":
			gotName, _ = req.Params["name"].(string)
			gotArgs, _ = req.Params["arguments"].(map[string]any)
			result = map[string]any{
				"content": []map[string]any{{"type": "text", "text": `{"ok":true}`}},
			}
		default:
			result = map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": req.ID, "result": result,
		})
	}))
	defer srv.Close()

	c := NewClient(Config{
		Enabled:      true,
		BaseURL:      srv.URL,
		JSONRPCPath:  "",
		APIKey:       "test-key",
		APIKeyHeader: "X-MCP-API-Key",
	})
	text, err := c.UpdateTrafficSplit(context.Background(), "rule-1", "prod", []BackendWeight{
		{Label: "stable", Weight: 80},
		{Label: "canary", Weight: 20},
	})
	if err != nil {
		t.Fatalf("UpdateTrafficSplit: %v", err)
	}
	if text == "" {
		t.Fatal("expected text result")
	}
	if gotName != "update_traffic_split" {
		t.Fatalf("tool name = %q", gotName)
	}
	if gotArgs["rule_id"] != "rule-1" {
		t.Fatalf("rule_id = %v", gotArgs["rule_id"])
	}
}

func TestDisabledClient(t *testing.T) {
	c := NewClient(Config{})
	if c.Enabled() {
		t.Fatal("expected disabled")
	}
	if _, err := c.CallTool(context.Background(), "x", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestProbeToolsDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":{}}}`))
		case "tools/call":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32002,"message":"MCP tools are disabled"}}`))
		default:
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		}
	}))
	defer srv.Close()

	c := NewClient(Config{Enabled: true, BaseURL: srv.URL, JSONRPCPath: "/mcp/jsonrpc"})
	p := c.ProbeTools(context.Background())
	if !p.Reachable || p.Err != nil || len(p.Tools) != 0 || p.Has("update_traffic_split") {
		t.Fatalf("probe = %+v", p)
	}
	st := c.Status(context.Background())
	if st["ok"] != false || st["reachable"] != true || st["tools_enabled"] != false {
		t.Fatalf("status = %v", st)
	}
	_, err := c.UpdateTrafficSplit(context.Background(), "r", "prod", []BackendWeight{{Label: "a", Weight: 1}})
	if !IsToolsDisabled(err) {
		t.Fatalf("want tools-disabled error, got %v", err)
	}
}
