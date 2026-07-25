package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ClaudeClient calls the Anthropic Messages API.
type ClaudeClient struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	httpClient   *http.Client
}

// NewClaudeClient creates a ClaudeClient with a sensible HTTP timeout.
func NewClaudeClient(baseURL, apiKey, defaultModel string) *ClaudeClient {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	if defaultModel == "" {
		defaultModel = "claude-sonnet-4-20250514"
	}
	return &ClaudeClient{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		DefaultModel: defaultModel,
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

func (c *ClaudeClient) ProviderName() string { return "claude" }

// SupportsTools reports that this client can use native tool-calling
// (GenerateRequest.ToolDefs / GenerateResponse.ToolCalls).
func (c *ClaudeClient) SupportsTools() bool { return true }

// claudeRequest mirrors the Anthropic /v1/messages request body.
type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
	Messages  []claudeMessage `json:"messages"`
	Tools     []claudeTool    `json:"tools,omitempty"`
}

// claudeMessage's Content is either a plain string or an array of content
// blocks (text / tool_use / tool_result) — hence the any type.
type claudeMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// claudeTool is a function definition in the Anthropic "tools" array.
type claudeTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type claudeResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *ClaudeClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = c.DefaultModel
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	// Separate system message from conversation messages, translating any
	// native tool-calling fields into Anthropic content blocks.
	var systemPrompt string
	msgs := make([]claudeMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch {
		case m.Role == RoleSystem:
			systemPrompt = m.Content
		case m.Role == RoleTool:
			// A tool result becomes a user message with a tool_result block.
			msgs = append(msgs, claudeMessage{
				Role: RoleUser,
				Content: []any{map[string]any{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     m.Content,
				}},
			})
		case len(m.ToolCalls) > 0:
			// An assistant turn that requested tools: optional text + tool_use blocks.
			blocks := make([]any, 0, len(m.ToolCalls)+1)
			if m.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := tc.Arguments
				if input == nil {
					input = map[string]any{}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			msgs = append(msgs, claudeMessage{Role: m.Role, Content: blocks})
		default:
			msgs = append(msgs, claudeMessage{Role: m.Role, Content: m.Content})
		}
	}

	body := claudeRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages:  msgs,
	}

	// Native tool-calling: advertise function definitions when provided.
	for _, td := range req.ToolDefs {
		schema := td.Parameters
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		body.Tools = append(body.Tools, claudeTool{
			Name:        td.Name,
			Description: td.Description,
			InputSchema: schema,
		})
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("claude: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("claude: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("claude: HTTP error: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("claude: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude: unexpected status %d: %s", resp.StatusCode, string(rawBody))
	}

	var chatResp claudeResponse
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return nil, fmt.Errorf("claude: decode response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("claude: API error (%s): %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	// Concatenate text blocks; collect any tool_use blocks as native tool calls.
	var content string
	var toolCalls []ToolCall
	for _, block := range chatResp.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			args := map[string]any{}
			if len(block.Input) > 0 {
				_ = json.Unmarshal(block.Input, &args)
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}

	return &GenerateResponse{
		Content:      content,
		FinishReason: chatResp.StopReason,
		InputTokens:  chatResp.Usage.InputTokens,
		OutputTokens: chatResp.Usage.OutputTokens,
		ToolCalls:    toolCalls,
	}, nil
}
