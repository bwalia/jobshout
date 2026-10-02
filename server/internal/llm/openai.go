package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpenAIClient calls the OpenAI chat completions API (or any compatible
// endpoint such as LM Studio, vLLM, or Groq).
type OpenAIClient struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	httpClient   *http.Client
	retry        retryPolicy
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
		retry: defaultRetryPolicy,
	}
}

// WithTimeout sets the per-attempt HTTP timeout. Zero keeps the current one.
func (c *OpenAIClient) WithTimeout(d time.Duration) *OpenAIClient {
	if d > 0 {
		c.httpClient.Timeout = d
	}
	return c
}

func (c *OpenAIClient) ProviderName() string { return "openai" }

// ModelName is the model a call without an explicit Model will use.
func (c *OpenAIClient) ModelName() string { return c.DefaultModel }

// isOpenAIProper reports whether BaseURL is OpenAI itself rather than a
// compatible server (LM Studio, vLLM, Groq). Parameters only OpenAI is known to
// accept are sent only there, so a compatible endpoint keeps the request shape
// it has always had.
func (c *OpenAIClient) isOpenAIProper() bool {
	u, err := url.Parse(c.BaseURL)
	return err == nil && strings.EqualFold(u.Hostname(), "api.openai.com")
}

// SupportsTools reports that this client can use native tool-calling
// (GenerateRequest.ToolDefs / GenerateResponse.ToolCalls).
func (c *OpenAIClient) SupportsTools() bool { return true }

// openAIChatRequest mirrors the OpenAI /v1/chat/completions request body.
type openAIChatRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	// MaxCompletionTokens replaces max_tokens on OpenAI itself; reasoning
	// models (o-series, gpt-5) reject max_tokens outright.
	MaxCompletionTokens int               `json:"max_completion_tokens,omitempty"`
	Temperature         float64           `json:"temperature,omitempty"`
	Tools               []openAITool      `json:"tools,omitempty"`
	ResponseFormat      *openAIRespFormat `json:"response_format,omitempty"`
}

type openAIRespFormat struct {
	Type string `json:"type"`
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
		Temperature: req.Temperature,
	}
	if c.isOpenAIProper() {
		body.MaxCompletionTokens = req.MaxTokens
		// json_object mode is refused unless the conversation mentions JSON,
		// so it is only asked for when a prompt already does.
		if req.JSON && mentionsJSON(req.Messages) {
			body.ResponseFormat = &openAIRespFormat{Type: "json_object"}
		}
	} else {
		body.MaxTokens = req.MaxTokens
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

	rawBody, err := doHosted(ctx, c.httpClient, "openai", c.retry,
		func(ctx context.Context) (*http.Request, error) {
			r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(payload))
			if err != nil {
				return nil, err
			}
			r.Header.Set("Content-Type", "application/json")
			if c.APIKey != "" {
				r.Header.Set("Authorization", "Bearer "+c.APIKey)
			}
			return r, nil
		},
		classifyOpenAIError,
	)
	if err != nil {
		return nil, err
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return nil, malformed("openai", "decode response: "+err.Error())
	}

	if chatResp.Error != nil {
		return nil, &ProviderError{Provider: "openai", Kind: ErrProviderBadRequest,
			Message: snippet(chatResp.Error.Type+": "+chatResp.Error.Message, 300)}
	}

	if len(chatResp.Choices) == 0 {
		return nil, malformed("openai", "response contained no choices")
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

func mentionsJSON(msgs []Message) bool {
	for _, m := range msgs {
		if strings.Contains(strings.ToLower(m.Content), "json") {
			return true
		}
	}
	return false
}

// classifyOpenAIError reads OpenAI's error envelope:
//
//	{"error":{"message":"…","type":"insufficient_quota","code":"insufficient_quota"}}
//
// An exhausted quota is a 429 like a rate limit, but waiting will not clear
// it, so it is marked not retryable.
func classifyOpenAIError(status int, header http.Header, body []byte) *ProviderError {
	var env struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &env)

	pe := &ProviderError{Provider: "openai", Kind: kindForStatus(status), Status: status,
		RetryAfter: retryAfterHeader(header)}
	pe.Message = snippet(env.Error.Message, 300)
	if pe.Message == "" {
		pe.Message = snippet(string(body), 300)
	}
	if code, _ := env.Error.Code.(string); code == "insufficient_quota" || env.Error.Type == "insufficient_quota" {
		pe.noRetry = true
	}
	if code, _ := env.Error.Code.(string); code == "invalid_api_key" {
		pe.Kind = ErrProviderAuth
	}
	return pe
}
