package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ollamaToolServer serves /api/tags reporting the given capabilities for
// "qwen3-coder:30b" and /api/chat replying with the given NDJSON lines,
// capturing the chat request body.
func ollamaToolServer(t *testing.T, capabilities []string, chatLines []string) (*httptest.Server, *map[string]any) {
	t.Helper()
	captured := &map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			resp := map[string]any{"models": []map[string]any{{
				"name":         "qwen3-coder:30b",
				"details":      map[string]any{"family": "qwen3moe", "context_length": 32768},
				"capabilities": capabilities,
			}}}
			_ = json.NewEncoder(w).Encode(resp)
		case "/api/chat":
			_ = json.NewDecoder(r.Body).Decode(captured)
			w.Header().Set("Content-Type", "application/x-ndjson")
			for _, line := range chatLines {
				_, _ = w.Write([]byte(line + "\n"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, captured
}

func TestOllamaGenerate_SendsToolsWhenModelSupportsThem(t *testing.T) {
	srv, captured := ollamaToolServer(t, []string{"completion", "tools"}, []string{
		`{"message":{"role":"assistant","content":"ok"},"done":true}`,
	})
	defer srv.Close()

	_, err := NewOllamaClient(srv.URL, "qwen3-coder:30b").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		ToolDefs: []ToolDef{{
			Name:        "task_create",
			Description: "Create a task",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string"}}},
		}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	tools, ok := (*captured)["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want one entry", (*captured)["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["type"] != "function" {
		t.Errorf("tool type = %v, want function", tool["type"])
	}
	fn := tool["function"].(map[string]any)
	if fn["name"] != "task_create" || fn["description"] != "Create a task" {
		t.Errorf("function = %#v", fn)
	}
	if _, ok := fn["parameters"].(map[string]any); !ok {
		t.Errorf("parameters missing: %#v", fn)
	}
}

func TestOllamaGenerate_OmitsToolsForNonToolModel(t *testing.T) {
	srv, captured := ollamaToolServer(t, []string{"completion"}, []string{
		`{"message":{"role":"assistant","content":"ok"},"done":true}`,
	})
	defer srv.Close()

	_, err := NewOllamaClient(srv.URL, "qwen3-coder:30b").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		ToolDefs: []ToolDef{{Name: "task_create", Parameters: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, present := (*captured)["tools"]; present {
		t.Fatalf("tools sent to a model without the capability: %#v", (*captured)["tools"])
	}
}

func TestOllamaGenerate_RoundTripsToolHistory(t *testing.T) {
	srv, captured := ollamaToolServer(t, []string{"completion", "tools"}, []string{
		`{"message":{"role":"assistant","content":"done"},"done":true}`,
	})
	defer srv.Close()

	_, err := NewOllamaClient(srv.URL, "qwen3-coder:30b").Generate(context.Background(), GenerateRequest{
		Messages: []Message{
			{Role: RoleUser, Content: "list agents"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{
				ID: "call_0", Name: "agent_list", Arguments: map[string]any{"limit": 5},
			}}},
			{Role: RoleTool, ToolCallID: "call_0", Content: `{"agents":["DevOps"]}`},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	msgs := (*captured)["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("messages = %d, want 3", len(msgs))
	}
	asst := msgs[1].(map[string]any)
	calls, ok := asst["tool_calls"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("assistant tool_calls = %#v, want one", asst["tool_calls"])
	}
	fn := calls[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "agent_list" {
		t.Errorf("echoed call name = %v", fn["name"])
	}
	if args := fn["arguments"].(map[string]any); args["limit"] != float64(5) {
		t.Errorf("echoed arguments = %#v", args)
	}
	toolMsg := msgs[2].(map[string]any)
	if toolMsg["role"] != "tool" {
		t.Errorf("tool message role = %v", toolMsg["role"])
	}
	if toolMsg["tool_name"] != "agent_list" {
		t.Errorf("tool_name = %v, want agent_list mapped from ToolCallID", toolMsg["tool_name"])
	}
}

func TestOllamaGenerate_ParsesStreamedToolCalls(t *testing.T) {
	srv, _ := ollamaToolServer(t, []string{"completion", "tools"}, []string{
		`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"agent_list","arguments":{"limit":3}}},{"function":{"name":"task_list","arguments":{}}}]},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":10,"eval_count":4}`,
	})
	defer srv.Close()

	resp, err := NewOllamaClient(srv.URL, "qwen3-coder:30b").Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "what agents do I have?"}},
		ToolDefs: []ToolDef{{Name: "agent_list", Parameters: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call_0" || resp.ToolCalls[1].ID != "call_1" {
		t.Errorf("synthesized IDs = %q, %q", resp.ToolCalls[0].ID, resp.ToolCalls[1].ID)
	}
	if resp.ToolCalls[0].Name != "agent_list" {
		t.Errorf("call 0 name = %q", resp.ToolCalls[0].Name)
	}
	if v, ok := resp.ToolCalls[0].Arguments["limit"].(float64); !ok || v != 3 {
		t.Errorf("call 0 arguments = %#v", resp.ToolCalls[0].Arguments)
	}
	if resp.ToolCalls[1].Arguments == nil {
		t.Errorf("empty arguments should decode to a non-nil map")
	}
}

func TestOllamaSupportsTools_SelfPrimesFromTags(t *testing.T) {
	srv, _ := ollamaToolServer(t, []string{"completion", "tools"}, nil)
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "qwen3-coder:30b")
	if !c.SupportsTools() {
		t.Fatal("SupportsTools = false for a model advertising the tools capability")
	}
}

func TestOllamaSupportsTools_FalseWithoutCapability(t *testing.T) {
	// Pre-0.6 servers report no capabilities at all; ListModels then assumes
	// completion-only, so tools must answer false.
	srv, _ := ollamaToolServer(t, nil, nil)
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "qwen3-coder:30b")
	if c.SupportsTools() {
		t.Fatal("SupportsTools = true for a server that reports no capabilities")
	}
}

func TestOllamaSupportsTools_FalseWhenProbeFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "qwen3-coder:30b")
	if c.SupportsTools() {
		t.Fatal("SupportsTools must answer false when discovery fails")
	}
}
