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

// OpenAIClient calls the OpenAI chat completions API (or any compatible
// endpoint such as LM Studio, vLLM, or Groq).
type OpenAIClient struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	httpClient   *http.Client
}

// NewOpenAIClient creates an OpenAIClient with a sensible HTTP timeout.
// baseURL should be the root URL, e.g. "https://api.openai.com".
func NewOpenAIClient(baseURL, apiKey, defaultModel string) *OpenAIClient {
	return &OpenAIClient{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		DefaultModel: defaultModel,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *OpenAIClient) ProviderName() string { return "openai" }

// SupportsTools reports that this client can use native tool-calling
// (GenerateRequest.ToolDefs / GenerateResponse.ToolCalls).
func (c *OpenAIClient) SupportsTools() bool { return true }

// openAIChatRequest mirrors the OpenAI /v1/chat/completions request body.
type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Tools       []openAITool    `json:"tools,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// openAITool is a function definition in the /chat/completions "tools" array.
type openAITool struct {
	Type     string           `json:"type"`
	Function openAIToolSchema `json:"function"`
}

type openAIToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// openAIToolCall is one entry in an assistant message's tool_calls array (both
// on requests we echo back and on parsed responses).
type openAIToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function openAIToolCallFunction `json:"function"`
}

type openAIToolCallFunction struct {
	Name string `json:"name"`
	// Arguments is a JSON-encoded string per the OpenAI wire format.
	Arguments string `json:"arguments"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (c *OpenAIClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = c.DefaultModel
	}

	msgs := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		om := openAIMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			args, _ := json.Marshal(tc.Arguments)
			om.ToolCalls = append(om.ToolCalls, openAIToolCall{
				ID:       tc.ID,
				Type:     "function",
				Function: openAIToolCallFunction{Name: tc.Name, Arguments: string(args)},
			})
		}
		msgs[i] = om
	}

	body := openAIChatRequest{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	// Native tool-calling: advertise function definitions when provided.
	for _, td := range req.ToolDefs {
		body.Tools = append(body.Tools, openAITool{
			Type: "function",
			Function: openAIToolSchema{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.Parameters,
			},
		})
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: HTTP error: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: unexpected status %d: %s", resp.StatusCode, string(rawBody))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return nil, fmt.Errorf("openai: decode response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("openai: API error (%s): %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: response contained no choices")
	}

	choice := chatResp.Choices[0]

	// Parse any native tool calls. Arguments arrive as a JSON-encoded string.
	var toolCalls []ToolCall
	for _, tc := range choice.Message.ToolCalls {
		args := map[string]any{}
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return &GenerateResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Model:        model,
		InputTokens:  chatResp.Usage.PromptTokens,
		OutputTokens: chatResp.Usage.CompletionTokens,
		ToolCalls:    toolCalls,
	}, nil
}
