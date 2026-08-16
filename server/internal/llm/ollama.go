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
//
// BaseURL may be a plain Ollama server or a gateway that verifies a JWT. When
// a gateway secret is configured every request is signed; otherwise requests go
// out unsigned, which is what a local Ollama expects.
type OllamaClient struct {
	BaseURL      string
	DefaultModel string
	// NumCtx is the context window requested per call via Ollama's num_ctx
	// option. Without it Ollama silently applies its own server-side default
	// regardless of what the model supports, so a large prompt is truncated
	// rather than refused — see effectiveNumCtx.
	NumCtx     int
	auth       *ollamaAuth
	httpClient *http.Client
	// models caches what ListModels learned, so per-model capability and
	// context questions can be answered without another round trip.
	models ollamaModelCache
}

// NewOllamaClient creates an OllamaClient talking to a plain Ollama server.
func NewOllamaClient(baseURL, defaultModel string) *OllamaClient {
	return NewOllamaClientWithAuth(baseURL, defaultModel, "", 0)
}

// NewOllamaClientWithAuth creates an OllamaClient that signs each request with
// a freshly minted JWT when gatewaySecret is non-empty.
//
// timeout is generous by design: a large model that is not resident has to be
// loaded before the first token appears, and that can take minutes. A zero
// timeout falls back to defaultOllamaTimeout.
func NewOllamaClientWithAuth(baseURL, defaultModel, gatewaySecret string, timeout time.Duration) *OllamaClient {
	if timeout <= 0 {
		timeout = defaultOllamaTimeout
	}
	return &OllamaClient{
		BaseURL:      baseURL,
		DefaultModel: defaultModel,
		auth:         newOllamaAuth(gatewaySecret),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// defaultOllamaTimeout applies when none is configured.
const defaultOllamaTimeout = 3 * time.Minute

// DefaultOllamaNumCtx is the context window requested when none is configured.
// It matches Ollama's own historical default, so behaviour is unchanged for
// anyone who does not set OLLAMA_NUM_CTX.
const DefaultOllamaNumCtx = 8192

// effectiveNumCtx decides the num_ctx to request for a model.
//
// It is the configured ceiling, lowered to the model's architectural limit when
// discovery knows it — asking for more than a model can hold is an error on some
// Ollama builds and wasted VRAM on the rest. When the model is unknown the
// configured value is used as-is.
//
// modelselect applies this SAME ceiling when building candidates, so the
// selector's belief about a context window matches what will actually be
// requested. If these two ever disagree, the selector will approve prompts that
// get silently truncated.
func (c *OllamaClient) effectiveNumCtx(model string) int {
	want := c.NumCtx
	if want <= 0 {
		want = DefaultOllamaNumCtx
	}
	if limit := c.ContextTokensFor(model); limit > 0 && limit < want {
		return limit
	}
	return want
}

// WithNumCtx sets the context window requested per call. Zero or negative keeps
// DefaultOllamaNumCtx. Returns the client so it can be chained onto a
// constructor, which is why this is a setter rather than another positional
// parameter on NewOllamaClientWithAuth.
func (c *OllamaClient) WithNumCtx(n int) *OllamaClient {
	c.NumCtx = n
	return c
}

// UsesGateway reports whether requests are being signed for a JWT gateway.
// Surfaced so startup logging can state which mode is in effect.
func (c *OllamaClient) UsesGateway() bool { return c.auth.enabled() }

func (c *OllamaClient) ProviderName() string { return "ollama" }

// SupportsTools reports that this client does NOT use native tool-calling; the
// executor falls back to the ReAct JSON-in-prompt loop for Ollama.
func (c *OllamaClient) SupportsTools() bool { return false }

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
	NumCtx      int     `json:"num_ctx,omitempty"`
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
	opts.NumCtx = c.effectiveNumCtx(model)

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
	if err := c.auth.apply(httpReq); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: HTTP error: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read response body: %w", err)
	}

	if isAuthStatus(resp.StatusCode) {
		return nil, authError(resp.StatusCode, rawBody)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode, upstreamSnippet(rawBody))
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
