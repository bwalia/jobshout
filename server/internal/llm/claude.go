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

// claudeRequest mirrors the Anthropic /v1/messages request body.
type claudeRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    string           `json:"system,omitempty"`
	Messages  []claudeMessage  `json:"messages"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
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

	// Separate system message from conversation messages.
	var systemPrompt string
	msgs := make([]claudeMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			systemPrompt = m.Content
			continue
		}
		msgs = append(msgs, claudeMessage{Role: m.Role, Content: m.Content})
	}

	body := claudeRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages:  msgs,
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

	// Concatenate all text content blocks.
	var content string
	for _, block := range chatResp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return &GenerateResponse{
		Content:      content,
		FinishReason: chatResp.StopReason,
		InputTokens:  chatResp.Usage.InputTokens,
		OutputTokens: chatResp.Usage.OutputTokens,
	}, nil
}
