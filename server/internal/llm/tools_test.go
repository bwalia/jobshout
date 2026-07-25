package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSupportsToolsCapability(t *testing.T) {
	cases := []struct {
		client Client
		want   bool
	}{
		{NewOpenAIClient("http://x", "k", "m"), true},
		{NewClaudeClient("http://x", "k", "m"), true},
		{NewOllamaClient("http://x", "m"), false},
	}
	for _, c := range cases {
		tc, ok := c.client.(ToolCapableClient)
		if !ok {
			t.Fatalf("%s does not implement ToolCapableClient", c.client.ProviderName())
		}
		if got := tc.SupportsTools(); got != c.want {
			t.Fatalf("%s SupportsTools = %v, want %v", c.client.ProviderName(), got, c.want)
		}
	}
}

func TestOpenAIToolsSerializationAndParsing(t *testing.T) {
	var captured map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "",
					"tool_calls": [{
						"id": "call_1",
						"type": "function",
						"function": {"name": "shell_command", "arguments": "{\"command\":\"ls\"}"}
					}]
				},
				"finish_reason": "tool_calls"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5}
		}`))
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "test-key", "gpt-test")
	resp, err := client.Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: RoleUser, Content: "list files"}},
		ToolDefs: []ToolDef{{
			Name:        "shell_command",
			Description: "run a command",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}, "required": []string{"command"}},
		}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Request must carry a tools array with the function definition.
	toolsArr, ok := captured["tools"].([]any)
	if !ok || len(toolsArr) != 1 {
		t.Fatalf("request tools not serialized: %#v", captured["tools"])
	}
	fn := toolsArr[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "shell_command" {
		t.Fatalf("tool name: %v", fn["name"])
	}
	if fn["parameters"] == nil {
		t.Fatalf("tool parameters missing")
	}

	// Response tool_calls must be parsed into GenerateResponse.ToolCalls.
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "shell_command" || tc.Arguments["command"] != "ls" {
		t.Fatalf("parsed tool call wrong: %#v", tc)
	}
}

func TestOpenAIEchoesToolCallAndResult(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{}}`))
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "k", "gpt-test")
	_, err := client.Generate(context.Background(), GenerateRequest{
		Messages: []Message{
			{Role: RoleUser, Content: "list files"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_1", Name: "shell_command", Arguments: map[string]any{"command": "ls"}}}},
			{Role: RoleTool, ToolCallID: "call_1", Content: "a.txt"},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	msgs := captured["messages"].([]any)
	assistant := msgs[1].(map[string]any)
	tcs, ok := assistant["tool_calls"].([]any)
	if !ok || len(tcs) != 1 {
		t.Fatalf("assistant tool_calls not echoed: %#v", assistant)
	}
	if got := tcs[0].(map[string]any)["function"].(map[string]any)["arguments"]; got != `{"command":"ls"}` {
		t.Fatalf("arguments not JSON-encoded string: %v", got)
	}
	toolMsg := msgs[2].(map[string]any)
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "call_1" {
		t.Fatalf("tool result message wrong: %#v", toolMsg)
	}
}

func TestClaudeToolsSerializationAndParsing(t *testing.T) {
	var captured map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content": [
				{"type": "text", "text": "let me check"},
				{"type": "tool_use", "id": "toolu_1", "name": "jira_get_issue", "input": {"external_id": "PROJ-7"}}
			],
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 12, "output_tokens": 8}
		}`))
	}))
	defer srv.Close()

	client := NewClaudeClient(srv.URL, "test-key", "claude-test")
	resp, err := client.Generate(context.Background(), GenerateRequest{
		Messages: []Message{
			{Role: RoleSystem, Content: "you are helpful"},
			{Role: RoleUser, Content: "fetch PROJ-7"},
		},
		ToolDefs: []ToolDef{{
			Name:        "jira_get_issue",
			Description: "get an issue",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{"external_id": map[string]any{"type": "string"}}, "required": []string{"external_id"}},
		}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Request must carry tools[] with input_schema, and system extracted.
	toolsArr, ok := captured["tools"].([]any)
	if !ok || len(toolsArr) != 1 {
		t.Fatalf("request tools not serialized: %#v", captured["tools"])
	}
	tool0 := toolsArr[0].(map[string]any)
	if tool0["name"] != "jira_get_issue" || tool0["input_schema"] == nil {
		t.Fatalf("claude tool wrong: %#v", tool0)
	}
	if captured["system"] != "you are helpful" {
		t.Fatalf("system not extracted: %v", captured["system"])
	}

	// Response must parse text content + tool_use into ToolCalls.
	if resp.Content != "let me check" {
		t.Fatalf("content: %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "toolu_1" || tc.Name != "jira_get_issue" || tc.Arguments["external_id"] != "PROJ-7" {
		t.Fatalf("parsed tool call wrong: %#v", tc)
	}
}

func TestClaudeEchoesToolUseAndResultBlocks(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{}}`))
	}))
	defer srv.Close()

	client := NewClaudeClient(srv.URL, "k", "claude-test")
	_, err := client.Generate(context.Background(), GenerateRequest{
		Messages: []Message{
			{Role: RoleUser, Content: "fetch PROJ-7"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "toolu_1", Name: "jira_get_issue", Arguments: map[string]any{"external_id": "PROJ-7"}}}},
			{Role: RoleTool, ToolCallID: "toolu_1", Content: `{"status":"open"}`},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	msgs := captured["messages"].([]any)
	// assistant turn -> content is a list with a tool_use block
	assistantBlocks := msgs[1].(map[string]any)["content"].([]any)
	toolUse := assistantBlocks[len(assistantBlocks)-1].(map[string]any)
	if toolUse["type"] != "tool_use" || toolUse["id"] != "toolu_1" {
		t.Fatalf("tool_use block wrong: %#v", toolUse)
	}
	// tool result -> a user message with a tool_result block
	resultMsg := msgs[2].(map[string]any)
	if resultMsg["role"] != "user" {
		t.Fatalf("tool result role: %v", resultMsg["role"])
	}
	resultBlock := resultMsg["content"].([]any)[0].(map[string]any)
	if resultBlock["type"] != "tool_result" || resultBlock["tool_use_id"] != "toolu_1" {
		t.Fatalf("tool_result block wrong: %#v", resultBlock)
	}
}
