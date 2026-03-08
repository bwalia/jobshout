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

// OllamaClient calls the Ollama REST API running at BaseURL.
// It uses the /api/chat endpoint which supports multi-turn message history.
type OllamaClient struct {
	BaseURL      string
	DefaultModel string
	httpClient   *http.Client
}

// NewOllamaClient creates an OllamaClient with a sensible HTTP timeout.
func NewOllamaClient(baseURL, defaultModel string) *OllamaClient {
	return &OllamaClient{
		BaseURL:      baseURL,
		DefaultModel: defaultModel,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *OllamaClient) ProviderName() string { return "ollama" }

// ollamaChatRequest mirrors the Ollama /api/chat request body.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  ollamaOptions   `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaOptions struct {
	NumPredict  int     `json:"num_predict,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// ollamaChatResponse mirrors the Ollama /api/chat response body (stream:false).
type ollamaChatResponse struct {
	Model   string        `json:"model"`
	Message ollamaMessage `json:"message"`
	Done    bool          `json:"done"`
	// Ollama reports token counts only when done=true.
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

func (c *OllamaClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = c.DefaultModel
	}

	msgs := make([]ollamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = ollamaMessage{Role: m.Role, Content: m.Content}
	}

	opts := ollamaOptions{Temperature: req.Temperature}
	if req.MaxTokens > 0 {
		opts.NumPredict = req.MaxTokens
	}

	body := ollamaChatRequest{
		Model:    model,
		Messages: msgs,
		Stream:   false,
		Options:  opts,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: HTTP error: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode, string(rawBody))
	}

	var chatResp ollamaChatResponse
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return nil, fmt.Errorf("ollama: decode response: %w", err)
	}

	finishReason := "stop"
	if !chatResp.Done {
		finishReason = "length"
	}

	return &GenerateResponse{
		Content:      chatResp.Message.Content,
		FinishReason: finishReason,
		InputTokens:  chatResp.PromptEvalCount,
		OutputTokens: chatResp.EvalCount,
	}, nil
}
