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

// OpenAIEmbedder calls the OpenAI embeddings API (or any compatible endpoint).
// The default model is text-embedding-3-small (1536 dimensions).
type OpenAIEmbedder struct {
	BaseURL    string
	APIKey     string
	Model      string
	dimensions int
	httpClient *http.Client
}

// NewOpenAIEmbedder creates an OpenAIEmbedder with a sensible HTTP timeout.
// baseURL should be the root URL, e.g. "https://api.openai.com".
func NewOpenAIEmbedder(baseURL, apiKey, model string, dimensions int) *OpenAIEmbedder {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	if dimensions <= 0 {
		dimensions = 1536
	}
	return &OpenAIEmbedder{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		Model:      model,
		dimensions: dimensions,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (e *OpenAIEmbedder) Dimensions() int      { return e.dimensions }
func (e *OpenAIEmbedder) EmbedderName() string { return "openai" }

// openAIEmbeddingRequest mirrors the OpenAI /v1/embeddings request body.
type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body := openAIEmbeddingRequest{Model: e.Model, Input: texts}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal embedding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/v1/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("openai: build embedding request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.APIKey)
	}

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: embedding HTTP error: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read embedding response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: unexpected embedding status %d: %s", resp.StatusCode, string(rawBody))
	}

	var embResp openAIEmbeddingResponse
	if err := json.Unmarshal(rawBody, &embResp); err != nil {
		return nil, fmt.Errorf("openai: decode embedding response: %w", err)
	}

	if embResp.Error != nil {
		return nil, fmt.Errorf("openai: embedding API error (%s): %s", embResp.Error.Type, embResp.Error.Message)
	}

	if len(embResp.Data) != len(texts) {
		return nil, fmt.Errorf("openai: embedding count mismatch: got %d, want %d", len(embResp.Data), len(texts))
	}

	// The API may not guarantee ordering, so index into the result by Index.
	out := make([][]float32, len(texts))
	for _, d := range embResp.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return nil, fmt.Errorf("openai: embedding index out of range: %d", d.Index)
		}
		out[d.Index] = d.Embedding
	}
	return out, nil
}
