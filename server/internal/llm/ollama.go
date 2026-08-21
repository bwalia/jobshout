package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
//
// Ten minutes matches the shared workstation host: it is CPU-only, cold model
// loads take minutes, and a long article draft (streamed, 900–1400 words) can
// legitimately sit past three minutes while queued behind other callers. The
// previous 3m default produced recurring
// "Client.Timeout exceeded while awaiting headers" failures on draft.
const defaultOllamaTimeout = 30 * time.Minute

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
	// Think turns off a reasoning model's visible thinking phase.
	//
	// It is always false because nothing here reads the thinking — only
	// message.content is used — while the thinking costs real time and, worse,
	// counts against num_predict. A reasoning model given a long prompt and a
	// bounded budget can spend the whole budget thinking and return empty
	// content, which surfaces as "empty response from ollama" and looks like
	// the model failing rather than the request being mis-shaped.
	//
	// Measured on muse-glimmer, a reasoning model: the same prompt took 47s
	// with thinking and 12s without, and the thinking it produced was
	// degenerate — the prompt echoed back to itself.
	//
	// Models with no thinking phase ignore the field.
	Think   bool          `json:"think"`
	Options ollamaOptions `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Thinking is a reasoning model's internal monologue. It is decoded so a
	// response that contains only thinking can be reported as such rather than
	// as a mysteriously empty reply — see Generate.
	Thinking string `json:"thinking,omitempty"`
}

type ollamaOptions struct {
	NumPredict  int     `json:"num_predict,omitempty"`
	NumCtx      int     `json:"num_ctx,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// ollamaChatResponse mirrors one Ollama /api/chat NDJSON chunk (stream:true)
// or the single object returned when stream:false.
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

	// Stream so response headers arrive with the first token. With stream:false
	// Ollama holds the connection silent until the whole reply is ready, and
	// Go's Client.Timeout then reports "awaiting headers" on long drafts even
	// when the host is working — which is exactly the article-generator
	// failure mode on the shared CPU box.
	body := ollamaChatRequest{
		Model:    model,
		Messages: msgs,
		Stream:   true,
		Think:    false,
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

	if isAuthStatus(resp.StatusCode) {
		rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, authError(resp.StatusCode, rawBody)
	}
	if resp.StatusCode != http.StatusOK {
		rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode, upstreamSnippet(rawBody))
	}

	return c.readStream(resp.Body, model, opts.NumPredict)
}

// readStream accumulates an Ollama NDJSON chat stream into one GenerateResponse.
func (c *OllamaClient) readStream(body io.Reader, model string, numPredict int) (*GenerateResponse, error) {
	var (
		content  strings.Builder
		thinking strings.Builder
		done     bool
		inTok    int
		outTok   int
	)

	scanner := bufio.NewScanner(body)
	// Article drafts can emit large single-line JSON chunks; the default
	// 64 KiB scanner buffer truncates them mid-object.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var chunk ollamaChatResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			return nil, fmt.Errorf("ollama: decode stream chunk: %w", err)
		}
		content.WriteString(chunk.Message.Content)
		thinking.WriteString(chunk.Message.Thinking)
		if chunk.Done {
			done = true
			inTok = chunk.PromptEvalCount
			outTok = chunk.EvalCount
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("ollama: read stream: %w", err)
	}

	finishReason := "stop"
	if !done {
		finishReason = "length"
	}

	text := content.String()
	// A reply that is only thinking means the model spent its whole budget
	// reasoning. Callers see an empty Content and report "empty response",
	// which is true but says nothing about the cause — so name it here, where
	// the evidence is.
	if strings.TrimSpace(text) == "" && thinking.Len() > 0 {
		return nil, fmt.Errorf(
			"ollama: model %q returned only reasoning and no content — it exhausted num_predict (%d) before answering",
			model, numPredict)
	}

	return &GenerateResponse{
		Content:      text,
		FinishReason: finishReason,
		Model:        model,
		InputTokens:  inTok,
		OutputTokens: outTok,
	}, nil
}
