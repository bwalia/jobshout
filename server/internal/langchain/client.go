// Package langchain provides an HTTP client that bridges Go agent execution
// requests to the Python sidecar's LangChain runner endpoint.
package langchain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
)

// sidecarRequest mirrors the Python RunRequest schema.
type sidecarRequest struct {
	ExecutionID  string         `json:"execution_id"`
	AgentID      string         `json:"agent_id"`
	Prompt       string         `json:"prompt"`
	SystemPrompt string         `json:"system_prompt,omitempty"`
	Model        string         `json:"model,omitempty"`
	Provider     string         `json:"provider,omitempty"`
	Tools        []string       `json:"tools"`
	Config       map[string]any `json:"config,omitempty"`
}

// sidecarResponse mirrors the Python RunResponse schema.
type sidecarResponse struct {
	ExecutionID string          `json:"execution_id"`
	FinalAnswer string          `json:"final_answer"`
	Iterations  int             `json:"iterations"`
	TotalTokens int             `json:"total_tokens"`
	ToolCalls   []sidecarTool   `json:"tool_calls"`
	Error       *string         `json:"error"`
}

type sidecarTool struct {
	ToolName   string         `json:"tool_name"`
	Input      map[string]any `json:"input"`
	Output     string         `json:"output"`
	Error      *string        `json:"error"`
	DurationMs int            `json:"duration_ms"`
}

// Client calls the Python sidecar's /run/langchain endpoint.
type Client struct {
	baseURL    string
	secret     string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient creates a LangChain sidecar client.
func NewClient(baseURL string, secret string, logger *zap.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		secret:  secret,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		logger: logger,
	}
}

// Run implements engine.Runner by proxying to the Python sidecar.
func (c *Client) Run(
	ctx context.Context,
	execID uuid.UUID,
	agent *model.Agent,
	taskPrompt string,
	agentTools []string,
) executor.Result {
	systemPrompt := ""
	if agent.SystemPrompt != nil {
		systemPrompt = *agent.SystemPrompt
	}
	modelName := ""
	if agent.ModelName != nil {
		modelName = *agent.ModelName
	}
	provider := ""
	if agent.ModelProvider != nil {
		provider = *agent.ModelProvider
	}

	reqBody := sidecarRequest{
		ExecutionID:  execID.String(),
		AgentID:      agent.ID.String(),
		Prompt:       taskPrompt,
		SystemPrompt: systemPrompt,
		Model:        modelName,
		Provider:     provider,
		Tools:        agentTools,
		Config:       agent.EngineConfig,
	}

	return c.call(ctx, "/run/langchain", reqBody)
}

func (c *Client) call(ctx context.Context, path string, reqBody sidecarRequest) executor.Result {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return executor.Result{Err: fmt.Errorf("langchain: marshal request: %w", err)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return executor.Result{Err: fmt.Errorf("langchain: create request: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sidecar-Secret", c.secret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return executor.Result{Err: fmt.Errorf("langchain: sidecar request failed: %w", err)}
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return executor.Result{Err: fmt.Errorf("langchain: read response: %w", err)}
	}

	if resp.StatusCode != http.StatusOK {
		return executor.Result{Err: fmt.Errorf("langchain: sidecar returned %d: %s", resp.StatusCode, string(respBytes))}
	}

	var sResp sidecarResponse
	if err := json.Unmarshal(respBytes, &sResp); err != nil {
		return executor.Result{Err: fmt.Errorf("langchain: unmarshal response: %w", err)}
	}

	result := executor.Result{
		FinalAnswer: sResp.FinalAnswer,
		Iterations:  sResp.Iterations,
		TotalTokens: sResp.TotalTokens,
	}

	if sResp.Error != nil && *sResp.Error != "" {
		result.Err = fmt.Errorf("langchain: %s", *sResp.Error)
	}

	for _, tc := range sResp.ToolCalls {
		record := executor.ToolCallRecord{
			ToolName:   tc.ToolName,
			Input:      tc.Input,
			Output:     tc.Output,
			DurationMs: tc.DurationMs,
		}
		if tc.Error != nil {
			record.Err = fmt.Errorf("%s", *tc.Error)
		}
		result.ToolCalls = append(result.ToolCalls, record)
	}

	return result
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
