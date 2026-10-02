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

// GeminiDefaultBaseURL is Google's public Gemini API root.
const GeminiDefaultBaseURL = "https://generativelanguage.googleapis.com"

// GeminiDefaultModel is used when neither the call nor GEMINI_DEFAULT_MODEL
// names one. "-latest" is Google's moving alias for the current Flash model.
const GeminiDefaultModel = "gemini-flash-latest"

// GeminiClient calls the Gemini generateContent API.
//
// The API key travels in the x-goog-api-key header, never in the URL, so it
// cannot surface in an error string, an access log or a trace.
type GeminiClient struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	httpClient   *http.Client
	retry        retryPolicy
}

// NewGeminiClient builds a client. An empty baseURL means Google's public host;
// timeout bounds one HTTP attempt (zero means 3 minutes).
func NewGeminiClient(baseURL, apiKey, defaultModel string, timeout time.Duration) *GeminiClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = GeminiDefaultBaseURL
	}
	if strings.TrimSpace(defaultModel) == "" {
		defaultModel = GeminiDefaultModel
	}
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	return &GeminiClient{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		APIKey:       apiKey,
		DefaultModel: defaultModel,
		httpClient:   &http.Client{Timeout: timeout},
		retry:        defaultRetryPolicy,
	}
}

func (c *GeminiClient) ProviderName() string { return "gemini" }

// ModelName is the model a call without an explicit Model will use.
func (c *GeminiClient) ModelName() string { return c.DefaultModel }

type geminiPart struct {
	Text    string `json:"text,omitempty"`
	Thought bool   `json:"thought,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens  int      `json:"maxOutputTokens,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	ResponseMIMEType string   `json:"responseMimeType,omitempty"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	// Pointers: a count Gemini leaves out is unknown, not 0.
	UsageMetadata struct {
		PromptTokenCount     *int `json:"promptTokenCount"`
		CandidatesTokenCount *int `json:"candidatesTokenCount"`
		ThoughtsTokenCount   *int `json:"thoughtsTokenCount"`
		TotalTokenCount      *int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	// ModelVersion is the concrete model that answered; for an alias such as
	// gemini-flash-latest it names the version the alias resolved to.
	ModelVersion string `json:"modelVersion"`
	ResponseID   string `json:"responseId"`
}

// Generate sends one generateContent call.
//
// Native tool-calling is not implemented for Gemini: the client does not
// advertise ToolCapableClient, so the executor drives it through the ReAct
// path and ToolDefs are ignored. OnToken is ignored too — the call does not
// stream.
func (c *GeminiClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, &ProviderError{Provider: "gemini", Kind: ErrProviderNotConfigured,
			Message: "GEMINI_API_KEY is not set"}
	}
	model := strings.TrimPrefix(strings.TrimSpace(req.Model), "models/")
	if model == "" {
		model = c.DefaultModel
	}

	payload, err := json.Marshal(buildGeminiRequest(req))
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent", c.BaseURL, url.PathEscape(model))

	body, err := doHosted(ctx, c.httpClient, "gemini", c.retry,
		func(ctx context.Context) (*http.Request, error) {
			r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
			if err != nil {
				return nil, err
			}
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("x-goog-api-key", c.APIKey)
			return r, nil
		},
		classifyGeminiError,
	)
	if err != nil {
		return nil, err
	}
	return parseGeminiResponse(ctx, body, model)
}

// buildGeminiRequest maps the provider-neutral conversation onto Gemini's
// shape: system turns become systemInstruction, the assistant is "model", and
// a tool result (which only a native tool loop would send) is passed as user
// text so nothing is silently dropped. Adjacent turns from the same side are
// merged, since Gemini expects the conversation to alternate.
func buildGeminiRequest(req GenerateRequest) geminiRequest {
	var system []string
	var contents []geminiContent
	for _, m := range req.Messages {
		role, text := "user", m.Content
		switch m.Role {
		case RoleSystem:
			if strings.TrimSpace(m.Content) != "" {
				system = append(system, m.Content)
			}
			continue
		case RoleAssistant:
			role = "model"
		case RoleTool:
			text = "Tool result: " + m.Content
		}
		if n := len(contents); n > 0 && contents[n-1].Role == role {
			contents[n-1].Parts = append(contents[n-1].Parts, geminiPart{Text: text})
			continue
		}
		contents = append(contents, geminiContent{Role: role, Parts: []geminiPart{{Text: text}}})
	}

	out := geminiRequest{Contents: contents}
	if len(system) > 0 {
		sys := strings.Join(system, "\n\n")
		if len(contents) == 0 {
			// A system prompt alone is not a valid request; send it as the
			// user turn so the call still asks for something.
			out.Contents = []geminiContent{{Role: "user", Parts: []geminiPart{{Text: sys}}}}
		} else {
			out.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: sys}}}
		}
	}
	if req.MaxTokens > 0 {
		out.GenerationConfig.MaxOutputTokens = req.MaxTokens
	}
	if req.Temperature > 0 {
		t := req.Temperature
		out.GenerationConfig.Temperature = &t
	}
	if req.JSON {
		out.GenerationConfig.ResponseMIMEType = "application/json"
	}
	return out
}

func parseGeminiResponse(ctx context.Context, body []byte, model string) (*GenerateResponse, error) {
	var gr geminiResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, malformed("gemini", "decode response: "+err.Error())
	}
	usage := geminiUsage(gr)
	providerModel := strings.TrimPrefix(gr.ModelVersion, "models/")
	// Noted before the reply is judged, so a blocked or thinking-only reply
	// that fails below still records what it reported.
	noteReply(ctx, providerModel, gr.ResponseID, usage)
	if len(gr.Candidates) == 0 {
		if gr.PromptFeedback != nil && gr.PromptFeedback.BlockReason != "" {
			return nil, &ProviderError{Provider: "gemini", Kind: ErrProviderBadRequest,
				Message: "prompt blocked: " + gr.PromptFeedback.BlockReason}
		}
		return nil, malformed("gemini", "response contained no candidates")
	}

	cand := gr.Candidates[0]
	var text strings.Builder
	for _, p := range cand.Content.Parts {
		if p.Thought {
			continue
		}
		text.WriteString(p.Text)
	}
	finish := geminiFinishReason(cand.FinishReason)

	if strings.TrimSpace(text.String()) == "" {
		switch {
		case finish == "length" && intOr(gr.UsageMetadata.ThoughtsTokenCount) > 0:
			return nil, fmt.Errorf("gemini: model %s %w (raise MaxTokens)", model, ErrOnlyThinking)
		case finish != "stop" && finish != "length" && finish != "":
			return nil, &ProviderError{Provider: "gemini", Kind: ErrProviderBadRequest,
				Message: "response blocked: " + cand.FinishReason}
		}
	}

	return &GenerateResponse{
		Content:       text.String(),
		FinishReason:  finish,
		Model:         model,
		InputTokens:   intOr(usage.InputTokens),
		OutputTokens:  intOr(usage.OutputTokens),
		ProviderModel: providerModel,
		RequestID:     gr.ResponseID,
		Usage:         usage,
	}, nil
}

// geminiUsage maps usageMetadata as reported. Output is candidates plus
// thoughts, because thinking tokens are billed as output; it stays nil only
// when Gemini reported neither.
func geminiUsage(gr geminiResponse) Usage {
	um := gr.UsageMetadata
	u := Usage{
		InputTokens:     um.PromptTokenCount,
		TotalTokens:     um.TotalTokenCount,
		ReasoningTokens: um.ThoughtsTokenCount,
	}
	if um.CandidatesTokenCount != nil || um.ThoughtsTokenCount != nil {
		out := intOr(um.CandidatesTokenCount) + intOr(um.ThoughtsTokenCount)
		u.OutputTokens = &out
	}
	return u
}

// geminiFinishReason maps Gemini's reasons onto the OpenAI-style values the
// rest of the platform reads ("stop", "length"); anything else is passed
// through lower-cased.
func geminiFinishReason(r string) string {
	switch r {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	default:
		return strings.ToLower(r)
	}
}

// classifyGeminiError reads Google's error envelope:
//
//	{"error":{"code":429,"message":"…","status":"RESOURCE_EXHAUSTED",
//	  "details":[{"@type":"…RetryInfo","retryDelay":"35s"},
//	             {"@type":"…ErrorInfo","reason":"API_KEY_INVALID"}]}}
//
// An invalid key arrives as a 400, so the reason is checked as well as the
// status.
func classifyGeminiError(status int, header http.Header, body []byte) *ProviderError {
	var env struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
			Details []struct {
				Reason     string `json:"reason"`
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &env)

	pe := &ProviderError{Provider: "gemini", Kind: kindForStatus(status), Status: status,
		RetryAfter: retryAfterHeader(header)}
	pe.Message = snippet(env.Error.Message, 300)
	if pe.Message == "" {
		pe.Message = snippet(string(body), 300)
	}
	for _, d := range env.Error.Details {
		if d.Reason == "API_KEY_INVALID" || d.Reason == "API_KEY_SERVICE_BLOCKED" {
			pe.Kind = ErrProviderAuth
		}
		if d.RetryDelay != "" && pe.RetryAfter == 0 {
			if dur, err := time.ParseDuration(d.RetryDelay); err == nil {
				pe.RetryAfter = dur
			}
		}
	}
	if env.Error.Status == "UNAUTHENTICATED" || env.Error.Status == "PERMISSION_DENIED" {
		pe.Kind = ErrProviderAuth
	}
	return pe
}
