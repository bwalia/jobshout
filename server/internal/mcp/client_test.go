package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// rpcEnvelope mirrors the incoming JSON-RPC request so the stub can echo the id.
type rpcEnvelope struct {
	ID     int64  `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

// newStubServer returns an httptest.Server that answers initialize, tools/list
// and tools/call. The auth argument, when non-empty, is required on requests.
func newStubServer(t *testing.T, auth string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth != "" && r.Header.Get("Authorization") != auth {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req rpcEnvelope
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": protocolVersion}
		case "tools/list":
			result = map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "echoes input",
						"inputSchema": map[string]any{"type": "object"},
					},
				},
			}
		case "tools/call":
			params, _ := req.Params.(map[string]any)
			name, _ := params["name"].(string)
			result = map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "called " + name},
				},
			}
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32601, "message": "method not found"},
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": req.ID, "result": result,
		})
	}))
}

func TestClientInitialize(t *testing.T) {
	srv := newStubServer(t, "")
	defer srv.Close()

	c := NewClient(srv.URL, "")
	if err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
}

func TestClientListTools(t *testing.T) {
	srv := newStubServer(t, "")
	defer srv.Close()

	c := NewClient(srv.URL, "")
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
	if tools[0].Description != "echoes input" {
		t.Fatalf("unexpected description: %q", tools[0].Description)
	}
	if tools[0].InputSchema["type"] != "object" {
		t.Fatalf("unexpected input schema: %+v", tools[0].InputSchema)
	}
}

func TestClientCallTool(t *testing.T) {
	srv := newStubServer(t, "")
	defer srv.Close()

	c := NewClient(srv.URL, "")
	out, err := c.CallTool(context.Background(), "echo", map[string]any{"msg": "hi"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if out != "called echo" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestClientAuthHeader(t *testing.T) {
	srv := newStubServer(t, "Bearer secret")
	defer srv.Close()

	// Without auth the stub returns 401 which surfaces as an error.
	if err := NewClient(srv.URL, "").Initialize(context.Background()); err == nil {
		t.Fatal("expected auth failure, got nil")
	}
	// With the right header it succeeds.
	if err := NewClient(srv.URL, "Bearer secret").Initialize(context.Background()); err != nil {
		t.Fatalf("authed initialize: %v", err)
	}
}

func TestClientRPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom"}}`)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "").ListTools(context.Background())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want rpc error containing boom, got %v", err)
	}
}

func TestClientSSEResponse(t *testing.T) {
	// Server replies with an SSE frame instead of bare JSON.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[{\"name\":\"sse_tool\"}]}}\n\n")
	}))
	defer srv.Close()

	tools, err := NewClient(srv.URL, "").ListTools(context.Background())
	if err != nil {
		t.Fatalf("sse list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "sse_tool" {
		t.Fatalf("unexpected sse tools: %+v", tools)
	}
}
