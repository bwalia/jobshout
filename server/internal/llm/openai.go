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

// ModelName is the model a call without an explicit Model will use.
func (c *OpenAIClient) ModelName() string { return c.DefaultModel }

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
	// ID is OpenAI's ID for the completion; Model the model that served it.
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	// Pointers: a count the reply leaves out (or a missing usage object) is
	// unknown, not 0.
	Usage *struct {
		PromptTokens            *int `json:"prompt_tokens"`
		CompletionTokens        *int `json:"completion_tokens"`
		TotalTokens             *int `json:"total_tokens"`
		CompletionTokensDetails *struct {
			ReasoningTokens *int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
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

	attempt := beginAttempt(ctx)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		err = fmt.Errorf("openai: HTTP error: %w", err)
		attempt.end(err)
		return nil, err
	}
	defer resp.Body.Close()
	attempt.response(resp)

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("openai: read response body: %w", err)
		attempt.end(err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("openai: unexpected status %d: %s", resp.StatusCode, string(rawBody))
		attempt.end(err)
		return nil, err
	}
	attempt.end(nil)

	var chatResp openAIChatResponse
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return nil, fmt.Errorf("openai: decode response: %w", err)
	}
	usage := openAIUsage(chatResp)
	// Noted before the reply is judged, so an error reply still records what
	// it reported.
	noteReply(ctx, chatResp.Model, chatResp.ID, usage)

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
		Content:       choice.Message.Content,
		FinishReason:  choice.FinishReason,
		Model:         model,
		InputTokens:   intOr(usage.InputTokens),
		OutputTokens:  intOr(usage.OutputTokens),
		ToolCalls:     toolCalls,
		ProviderModel: chatResp.Model,
		RequestID:     chatResp.ID,
		Usage:         usage,
	}, nil
}

// openAIUsage maps the reply's usage object as reported.
func openAIUsage(r openAIChatResponse) Usage {
	if r.Usage == nil {
		return Usage{}
	}
	u := Usage{
		InputTokens:  r.Usage.PromptTokens,
		OutputTokens: r.Usage.CompletionTokens,
		TotalTokens:  r.Usage.TotalTokens,
	}
	if d := r.Usage.CompletionTokensDetails; d != nil {
		u.ReasoningTokens = d.ReasoningTokens
	}
	return u
}
