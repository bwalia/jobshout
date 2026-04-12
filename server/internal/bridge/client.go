// Package bridge provides the enhanced Go ↔ Python bridge with SSE streaming.
package bridge

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

	"go.uber.org/zap"
)

// Client is the Python sidecar bridge client with streaming support.
type Client struct {
	baseURL    string
	secret     string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient creates a bridge client.
func NewClient(baseURL string, secret string, logger *zap.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		secret:  secret,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
		logger: logger,
	}
}

// StreamRequest is sent to the Python sidecar stream endpoints.
type StreamRequest struct {
	ExecutionID  string         `json:"execution_id"`
	AgentID      string         `json:"agent_id"`
	Prompt       string         `json:"prompt"`
	SystemPrompt string         `json:"system_prompt,omitempty"`
	Model        string         `json:"model,omitempty"`
	Provider     string         `json:"provider,omitempty"`
	Tools        []string       `json:"tools"`
	Config       map[string]any `json:"config,omitempty"`
	Engine       string         `json:"engine"` // "langchain" | "langgraph"
}

// StreamExecution opens an SSE connection to the sidecar and yields events.
// The caller receives events on the returned channel; the channel is closed
// when the stream ends or the context is cancelled.
func (c *Client) StreamExecution(ctx context.Context, req StreamRequest) (<-chan StreamEvent, error) {
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("bridge: marshal stream request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/stream/%s", c.baseURL, req.Engine)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("bridge: create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("X-Sidecar-Secret", c.secret)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("bridge: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("bridge: sidecar returned %d: %s", resp.StatusCode, string(body))
	}

	events := make(chan StreamEvent, 64)

	go func() {
		defer close(events)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			var event StreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				c.logger.Warn("bridge: failed to parse SSE event", zap.Error(err))
				continue
			}

			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}

		if err := scanner.Err(); err != nil {
			c.logger.Warn("bridge: SSE stream read error", zap.Error(err))
		}
	}()

	return events, nil
}

// Healthy checks if the sidecar is reachable.
func (c *Client) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
