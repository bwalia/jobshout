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

// OllamaEmbedder calls the Ollama embeddings API (POST /api/embeddings) running
// at BaseURL. The classic /api/embeddings endpoint embeds a single prompt per
// call, so batches are embedded sequentially.
type OllamaEmbedder struct {
	BaseURL    string
	Model      string
	dimensions int
	auth       *ollamaAuth
	httpClient *http.Client
}

// NewOllamaEmbedder creates an OllamaEmbedder talking to a plain Ollama server.
func NewOllamaEmbedder(baseURL, model string, dimensions int) *OllamaEmbedder {
	return NewOllamaEmbedderWithAuth(baseURL, model, dimensions, "")
}

// NewOllamaEmbedderWithAuth creates an OllamaEmbedder that signs each request
// with a freshly minted JWT when gatewaySecret is non-empty. Embeddings hit the
// same host as chat, so they go through the same gateway and need the same
// credential.
func NewOllamaEmbedderWithAuth(baseURL, model string, dimensions int, gatewaySecret string) *OllamaEmbedder {
	if model == "" {
		model = "nomic-embed-text"
	}
	if dimensions <= 0 {
		dimensions = 1536
	}
	return &OllamaEmbedder{
		BaseURL:    baseURL,
		Model:      model,
		dimensions: dimensions,
		auth:       newOllamaAuth(gatewaySecret),
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (e *OllamaEmbedder) Dimensions() int      { return e.dimensions }
func (e *OllamaEmbedder) EmbedderName() string { return "ollama" }

// ollamaEmbeddingRequest mirrors the Ollama /api/embeddings request body.
type ollamaEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error"`
}

func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	out := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := e.embedOne(ctx, text)
		if err != nil {
			return nil, err
		}
		out[i] = vec
	}
	return out, nil
}

func (e *OllamaEmbedder) embedOne(ctx context.Context, text string) ([]float32, error) {
	body := ollamaEmbeddingRequest{Model: e.Model, Prompt: text}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal embedding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/api/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ollama: build embedding request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if err := e.auth.apply(httpReq); err != nil {
		return nil, err
	}

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: embedding HTTP error: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read embedding response body: %w", err)
	}

	if isAuthStatus(resp.StatusCode) {
		return nil, authError(resp.StatusCode, rawBody)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: unexpected embedding status %d: %s", resp.StatusCode, upstreamSnippet(rawBody))
	}

	var embResp ollamaEmbeddingResponse
	if err := json.Unmarshal(rawBody, &embResp); err != nil {
		return nil, fmt.Errorf("ollama: decode embedding response: %w", err)
	}
	if embResp.Error != "" {
		return nil, fmt.Errorf("ollama: embedding API error: %s", embResp.Error)
	}
	if len(embResp.Embedding) == 0 {
		return nil, fmt.Errorf("ollama: embedding response contained no vector")
	}
	return embResp.Embedding, nil
}
