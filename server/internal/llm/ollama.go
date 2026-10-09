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
	// retryWait overrides the pause before re-sending after a gateway error;
	// zero uses defaultOllamaGatewayRetryWait. Tests shorten it.
	retryWait time.Duration
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
//
// requested is a per-call window (GenerateRequest.NumCtx); zero means the
// client's configured one. Either way the model's own limit still caps it.
func (c *OllamaClient) effectiveNumCtx(model string, requested int) int {
	want := requested
	if want <= 0 {
		want = c.NumCtx
	}
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

// ModelName is the model a call without an explicit Model will use.
func (c *OllamaClient) ModelName() string { return c.DefaultModel }

// SupportsTools reports whether the DEFAULT model does native tool-calling.
// Tool support on Ollama is per-model, not per-server (see ollama_models.go),
// so the answer comes from the discovery cache. False is the safe answer: a
// false negative costs only the ReAct fallback, which works.
func (c *OllamaClient) SupportsTools() bool {
	return c.modelSupportsTools(context.Background(), c.DefaultModel)
}

// modelSupportsTools resolves a model's "tools" capability, self-priming the
// discovery cache on a miss so the answer does not depend on whether anything
// happened to call ListModels first. A failed probe answers false.
func (c *OllamaClient) modelSupportsTools(ctx context.Context, model string) bool {
	info, ok := c.primedLookup(ctx, model)
	return ok && info.SupportsTools()
}

// modelSupportsThinking resolves a model's "thinking" capability the same way.
// False is the safe answer here too: requesting thinking from a model that
// lacks it is an error on some Ollama builds, while skipping it merely means a
// plain completion.
func (c *OllamaClient) modelSupportsThinking(ctx context.Context, model string) bool {
	info, ok := c.primedLookup(ctx, model)
	return ok && info.SupportsThinking()
}

// primedLookup is lookupModel with a self-priming discovery call on a miss.
func (c *OllamaClient) primedLookup(ctx context.Context, model string) (ModelInfo, bool) {
	info, ok := c.lookupModel(model)
	if !ok {
		primeCtx, cancel := context.WithTimeout(ctx, modelDiscoveryTimeout)
		defer cancel()
		if _, err := c.ListModels(primeCtx); err != nil {
			return ModelInfo{}, false
		}
		info, ok = c.lookupModel(model)
	}
	return info, ok
}

// ollamaChatRequest mirrors the Ollama /api/chat request body.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	// Think controls a reasoning model's thinking phase.
	//
	// It defaults to false because only message.content is read, while the
	// thinking costs real time and, worse, counts against num_predict. A
	// reasoning model given a long prompt and a bounded budget can spend the
	// whole budget thinking and return empty content, which surfaces as
	// "empty response from ollama" and looks like the model failing rather
	// than the request being mis-shaped.
	// (Measured on muse-glimmer: 47s with thinking vs 12s without on the same
	// prompt.)
	//
	// A caller that wants the reasoning quality anyway sets
	// GenerateRequest.Think and accepts those costs; it is honoured only for
	// models whose discovery capabilities include "thinking", because sending
	// think:true to a model without the capability is an error on some Ollama
	// builds. Models with no thinking phase ignore the field when false.
	Think bool `json:"think"`
	// Format is "json" when the caller set GenerateRequest.JSON: Ollama then
	// constrains decoding to a JSON value. Empty leaves the reply free-form.
	Format  string        `json:"format,omitempty"`
	Options ollamaOptions `json:"options,omitempty"`
	// Tools carries native function definitions. Only attached when the
	// resolved model advertises the "tools" capability — sending them to a
	// model that lacks it is an error on some Ollama builds.
	Tools []ollamaTool `json:"tools,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Thinking is a reasoning model's internal monologue. It is decoded so a
	// response that contains only thinking can be reported as such rather than
	// as a mysteriously empty reply — see Generate.
	Thinking string `json:"thinking,omitempty"`
	// ToolCalls echoes an assistant turn's tool requests back on follow-up
	// requests, and carries the model's requests on responses.
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	// ToolName names the tool a role:"tool" result message answers. Ollama has
	// no call IDs, so the name is the whole correlation.
	ToolName string `json:"tool_name,omitempty"`
}

// ollamaTool mirrors one entry of the /api/chat "tools" array.
type ollamaTool struct {
	Type     string             `json:"type"` // always "function"
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ollamaToolCall mirrors one tool request in a chat message. Unlike OpenAI,
// Ollama returns arguments as a JSON object, not an encoded string.
type ollamaToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
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
	// DoneReason is why a finished reply stopped: "stop", or "length" when it
	// ran into num_predict. A reply cut at the limit still arrives done=true.
	DoneReason string `json:"done_reason"`
	// Ollama reports token counts only when done=true, and may leave
	// prompt_eval_count out (a fully cached prompt). Pointers keep a missing
	// count unknown rather than 0.
	PromptEvalCount *int `json:"prompt_eval_count"`
	EvalCount       *int `json:"eval_count"`
}

func (c *OllamaClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = c.DefaultModel
	}

	msgs := make([]ollamaMessage, len(req.Messages))
	// Ollama correlates tool results by name, not ID, so remember what each
	// synthesized ToolCallID referred to as the history is walked in order.
	callNames := map[string]string{}
	for i, m := range req.Messages {
		om := ollamaMessage{Role: m.Role, Content: m.Content}
		if len(m.ToolCalls) > 0 {
			om.ToolCalls = make([]ollamaToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				om.ToolCalls[j].Function.Name = tc.Name
				om.ToolCalls[j].Function.Arguments = tc.Arguments
				if om.ToolCalls[j].Function.Arguments == nil {
					om.ToolCalls[j].Function.Arguments = map[string]any{}
				}
				callNames[tc.ID] = tc.Name
			}
		}
		if m.Role == RoleTool {
			om.ToolName = callNames[m.ToolCallID]
		}
		msgs[i] = om
	}

	opts := ollamaOptions{Temperature: resolvedTemperature(req)}
	if req.MaxTokens > 0 {
		opts.NumPredict = req.MaxTokens
	}
	opts.NumCtx = c.effectiveNumCtx(model, req.NumCtx)

	// Stream so response headers arrive with the first token. With stream:false
	// Ollama holds the connection silent until the whole reply is ready, and
	// Go's Client.Timeout then reports "awaiting headers" on long drafts even
	// when the host is working — which is exactly the article-generator
	// failure mode on the shared CPU box.
	body := ollamaChatRequest{
		Model:    model,
		Messages: msgs,
		Stream:   true,
		Think:    req.Think && !req.JSON && c.modelSupportsThinking(ctx, model),
		Options:  opts,
	}
	if req.JSON {
		body.Format = "json"
	}
	if len(req.ToolDefs) > 0 && c.modelSupportsTools(ctx, model) {
		body.Tools = make([]ollamaTool, len(req.ToolDefs))
		for i, d := range req.ToolDefs {
			body.Tools[i] = ollamaTool{
				Type: "function",
				Function: ollamaToolFunction{
					Name:        d.Name,
					Description: d.Description,
					Parameters:  d.Parameters,
				},
			}
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	// A gateway in front of Ollama answers 502/503/504 before any of the reply
	// has been sent, so the request can safely go again. On the shared CPU host
	// the usual cause is a cold model that did not produce its first token
	// inside the proxy's read timeout; the load carries on after the proxy
	// gives up, so the second attempt usually finds the model resident.
	var (
		resp    *http.Response
		attempt *apiAttempt
	)
	for try := 1; ; try++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("ollama: build request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if err := c.auth.apply(httpReq); err != nil {
			return nil, err
		}

		// The attempt lasts until the stream is fully read.
		attempt = beginAttempt(ctx)
		resp, err = c.httpClient.Do(httpReq)
		if err != nil {
			err = fmt.Errorf("ollama: HTTP error: %w", err)
			attempt.end(err)
			return nil, err
		}
		attempt.response(resp)
		if !isGatewayStatus(resp.StatusCode) || try >= ollamaGatewayAttempts {
			break
		}

		rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		resp.Body.Close()
		attempt.end(fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode, upstreamSnippet(rawBody)))
		t := time.NewTimer(c.gatewayRetryWait())
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
		noteRetry(ctx)
	}
	defer resp.Body.Close()

	if isAuthStatus(resp.StatusCode) {
		rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		err := authError(resp.StatusCode, rawBody)
		attempt.end(err)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		err := fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode, upstreamSnippet(rawBody))
		attempt.end(err)
		return nil, err
	}

	out, err := c.readStream(ctx, resp.Body, model, opts.NumPredict, req.OnToken)
	attempt.end(err)
	return out, err
}

// ollamaGatewayAttempts is how many times Generate sends a request that a
// gateway answered with 502/503/504, counting the first. Each attempt can cost
// the proxy's full read timeout, so one retry is the most that is worth it.
const ollamaGatewayAttempts = 2

// defaultOllamaGatewayRetryWait is the pause before re-sending after a gateway
// error, long enough for a model load the first attempt started to settle.
const defaultOllamaGatewayRetryWait = 5 * time.Second

func (c *OllamaClient) gatewayRetryWait() time.Duration {
	if c.retryWait > 0 {
		return c.retryWait
	}
	return defaultOllamaGatewayRetryWait
}

// isGatewayStatus reports whether status is a gateway or overload answer that
// arrived instead of a reply.
func isGatewayStatus(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

// readStream accumulates an Ollama NDJSON chat stream into one GenerateResponse.
func (c *OllamaClient) readStream(ctx context.Context, body io.Reader, model string, numPredict int, onToken func(string)) (*GenerateResponse, error) {
	var (
		content   strings.Builder
		thinking  strings.Builder
		toolCalls []ollamaToolCall
		done      bool
		reason    string
		inTok     *int
		outTok    *int
		served    string
	)
	// Stream tokens through the leak guard so leaked tool-call markup is never
	// forwarded to a live client.
	guard := &leakStreamGuard{onToken: onToken}

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
		guard.feed(chunk.Message.Content)
		thinking.WriteString(chunk.Message.Thinking)
		// Usually one chunk carries every tool call, but append rather than
		// overwrite in case they arrive split across chunks.
		toolCalls = append(toolCalls, chunk.Message.ToolCalls...)
		if chunk.Model != "" {
			served = chunk.Model
		}
		if chunk.Done {
			done = true
			reason = chunk.DoneReason
			inTok = chunk.PromptEvalCount
			outTok = chunk.EvalCount
		}
	}
	// Noted before the reply is judged (and even when the stream broke), so
	// a thinking-only or cut reply still records what it reported.
	noteReply(ctx, served, "", Usage{InputTokens: inTok, OutputTokens: outTok})
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("ollama: read stream: %w", err)
	}

	// "length" when the stream ended without a done chunk, or when Ollama
	// says it stopped at num_predict. The second case used to be reported as
	// "stop", so a caller could not tell a finished reply from a cut one.
	finishReason := "stop"
	if !done || reason == "length" {
		finishReason = "length"
	}

	text := content.String()
	// A reply that is only thinking means the model spent its whole budget
	// reasoning. Callers see an empty Content and report "empty response",
	// which is true but says nothing about the cause — so name it here, where
	// the evidence is. A reply carrying tool calls is not empty.
	if strings.TrimSpace(text) == "" && len(toolCalls) == 0 && thinking.Len() > 0 {
		return nil, fmt.Errorf(
			"ollama: model %q %w — it exhausted num_predict (%d) before answering",
			model, ErrOnlyThinking, numPredict)
	}

	// Ollama supplies no call IDs; synthesize stable ones so callers can echo
	// ToolCallID on their RoleTool replies (Generate maps it back to tool_name).
	var calls []ToolCall
	for i, tc := range toolCalls {
		args := tc.Function.Arguments
		if args == nil {
			args = map[string]any{}
		}
		calls = append(calls, ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	// The model sometimes writes its tool-call markup into content instead of
	// issuing structured tool calls. Recover those so the call still executes
	// and the markup never reaches the user.
	if len(calls) == 0 {
		if recovered, cleaned, ok := recoverLeakedToolCalls(text); ok {
			calls = recovered
			text = cleaned
		}
	}

	// Ollama reports no total and no request ID.
	return &GenerateResponse{
		Content:       text,
		FinishReason:  finishReason,
		Model:         model,
		InputTokens:   intOr(inTok),
		OutputTokens:  intOr(outTok),
		ToolCalls:     calls,
		ProviderModel: served,
		Usage:         Usage{InputTokens: inTok, OutputTokens: outTok},
	}, nil
}
