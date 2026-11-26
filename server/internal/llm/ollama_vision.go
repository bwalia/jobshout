package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DescribeImage asks a vision model (for example minicpm-v) about a picture
// and returns its answer. It goes through the same gateway auth as every other
// Ollama call. Message has no image field, so this is its own small request
// rather than a change to the shared chat path.
func (c *OllamaClient) DescribeImage(ctx context.Context, model, prompt string, image []byte) (string, error) {
	if model == "" {
		return "", fmt.Errorf("ollama: no vision model configured")
	}
	if len(image) == 0 {
		return "", fmt.Errorf("ollama: no image to describe")
	}
	body, err := json.Marshal(map[string]any{
		"model":  model,
		"stream": false,
		"messages": []map[string]any{{
			"role":    "user",
			"content": prompt,
			"images":  []string{base64.StdEncoding.EncodeToString(image)},
		}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.auth.apply(req); err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: describe image: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if isAuthStatus(resp.StatusCode) {
		return "", authError(resp.StatusCode, data)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama: describe image returned %d: %s", resp.StatusCode, upstreamSnippet(data))
	}
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("ollama: describe image: unreadable response: %s", upstreamSnippet(data))
	}
	text := strings.TrimSpace(out.Message.Content)
	if text == "" {
		return "", fmt.Errorf("ollama: the vision model returned no description")
	}
	return text, nil
}
