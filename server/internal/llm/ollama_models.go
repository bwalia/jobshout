package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
)

// ollamaTagsResponse mirrors GET /api/tags.
//
// Note the shape: capabilities is a SIBLING of details, while context_length and
// parameter_size live INSIDE it. Getting that nesting backwards silently yields
// zero-value metadata for every model.
type ollamaTagsResponse struct {
	Models []struct {
		Name    string `json:"name"`
		Model   string `json:"model"`
		Details struct {
			Family            string `json:"family"`
			ParameterSize     string `json:"parameter_size"`
			QuantizationLevel string `json:"quantization_level"`
			ContextLength     int    `json:"context_length"`
		} `json:"details"`
		Capabilities []string `json:"capabilities"`
	} `json:"models"`
}

// ListModels reports the models this Ollama server can actually run.
//
// It reuses the client's HTTP client and gateway auth so a JWT-fronted Ollama
// is discovered the same way it is called. The caller is expected to bound the
// context — Router does so at modelDiscoveryTimeout, because this client's own
// timeout is minutes long by design.
func (c *OllamaClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("ollama: build tags request: %w", err)
	}
	if err := c.auth.apply(httpReq); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: tags HTTP error: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read tags body: %w", err)
	}
	if isAuthStatus(resp.StatusCode) {
		return nil, authError(resp.StatusCode, rawBody)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: tags unexpected status %d: %s", resp.StatusCode, upstreamSnippet(rawBody))
	}

	var tags ollamaTagsResponse
	if err := json.Unmarshal(rawBody, &tags); err != nil {
		return nil, fmt.Errorf("ollama: decode tags: %w", err)
	}

	out := make([]ModelInfo, 0, len(tags.Models))
	for _, m := range tags.Models {
		name := m.Name
		if name == "" {
			name = m.Model
		}
		if name == "" {
			continue
		}

		caps := m.Capabilities
		if len(caps) == 0 {
			// Ollama before 0.6 reports no capabilities. Assume the model can
			// chat and assume it cannot use tools: a false negative costs only
			// the ReAct fallback, which works, while a false positive would
			// silently break a run.
			caps = []string{CapCompletion}
		}

		out = append(out, ModelInfo{
			Provider:      "ollama",
			Name:          name,
			ContextTokens: m.Details.ContextLength,
			ParameterSize: m.Details.ParameterSize,
			Quantization:  m.Details.QuantizationLevel,
			Family:        m.Details.Family,
			Capabilities:  caps,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	c.rememberModels(out)
	return out, nil
}

// ─── Per-model capability cache ─────────────────────────────────────────────
//
// Ollama tool support is a property of the MODEL, not the server: qwen3-coder:30b
// does native tool-calling and llama3:latest does not, on the same client. The
// provider-wide ToolCapableClient answer cannot express that, so the client keeps
// what ListModels learned.

type ollamaModelCache struct {
	mu      sync.RWMutex
	byName  map[string]ModelInfo
	fetched time.Time
}

func (c *OllamaClient) rememberModels(models []ModelInfo) {
	c.models.mu.Lock()
	defer c.models.mu.Unlock()
	c.models.byName = make(map[string]ModelInfo, len(models))
	for _, m := range models {
		c.models.byName[m.Name] = m
	}
	c.models.fetched = time.Now()
}

// lookupModel resolves a model name against the discovery cache, tolerating the
// ":latest" suffix Ollama treats as implicit.
func (c *OllamaClient) lookupModel(name string) (ModelInfo, bool) {
	if name == "" {
		name = c.DefaultModel
	}
	c.models.mu.RLock()
	defer c.models.mu.RUnlock()
	if c.models.byName == nil {
		return ModelInfo{}, false
	}
	if m, ok := c.models.byName[name]; ok {
		return m, true
	}
	if m, ok := c.models.byName[name+":latest"]; ok {
		return m, true
	}
	return ModelInfo{}, false
}

// ContextTokensFor reports the discovered architectural context window for a
// model, or 0 when it is unknown.
func (c *OllamaClient) ContextTokensFor(model string) int {
	m, ok := c.lookupModel(model)
	if !ok {
		return 0
	}
	return m.ContextTokens
}
