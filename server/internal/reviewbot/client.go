// Package reviewbot is the API's client for the in-cluster review-bot sidecar.
//
// Reviews take minutes (OpenCode + a 30B model), so the contract is start-and-poll
// rather than one blocking HTTP call. Auth is a shared bearer token on the
// cluster network — not the workstation JWT gateway.
package reviewbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

var (
	ErrNotConfigured = errors.New("reviewbot: no base URL configured; PR review is off for this ring")
	ErrUnauthorized  = errors.New("reviewbot: sidecar rejected the bearer token")
	ErrNotAllowed    = errors.New("reviewbot: repo is not on the allowlist")
	// ErrJobNotFound is a 404 from the sidecar: jobs live in memory, so a
	// restart forgets them. Permanent for this remote id — the reconciler
	// fails the row and the caller resubmits.
	ErrJobNotFound = errors.New("reviewbot: job not found (sidecar may have restarted)")
)

type StartRequest struct {
	Repo     string
	PRNumber int
	DryRun   bool
	Force    bool
	RunRef   string
}

type Handle struct {
	JobID         string
	State         string
	QueuePosition int
	Existing      bool
}

type Snapshot struct {
	JobID         string          `json:"job_id"`
	Kind          string          `json:"kind"`
	State         string          `json:"state"`
	StageLog      []string        `json:"stage_log"`
	QueuePosition *int            `json:"queue_position,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	Error         string          `json:"error,omitempty"`
	Created       string          `json:"created,omitempty"`
	Started       string          `json:"started,omitempty"`
	Finished      string          `json:"finished,omitempty"`
}

func (s *Snapshot) Terminal() bool {
	if s == nil {
		return false
	}
	return s.State == "done" || s.State == "failed"
}

type Capabilities struct {
	Opencode struct {
		Available bool `json:"available"`
	} `json:"opencode"`
	Allowed []string `json:"allowed"`
}

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	logger     *zap.Logger
}

func NewClient(baseURL, token string, timeout time.Duration, logger *zap.Logger) *Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger,
	}
}

func (c *Client) Enabled() bool { return c != nil && c.baseURL != "" }

func (c *Client) Start(ctx context.Context, req StartRequest) (*Handle, error) {
	if !c.Enabled() {
		return nil, ErrNotConfigured
	}
	body := map[string]any{
		"repo":      req.Repo,
		"pr_number": req.PRNumber,
		"dry_run":   req.DryRun,
		"force":     req.Force,
		"run_ref":   req.RunRef,
	}
	data, _, err := c.do(ctx, http.MethodPost, "/api/reviews", body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		JobID         string `json:"job_id"`
		State         string `json:"state"`
		QueuePosition int    `json:"queue_position"`
		Existing      bool   `json:"existing"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("reviewbot: decode start response: %w", err)
	}
	return &Handle{
		JobID:         parsed.JobID,
		State:         parsed.State,
		QueuePosition: parsed.QueuePosition,
		Existing:      parsed.Existing,
	}, nil
}

func (c *Client) Status(ctx context.Context, jobID string) (*Snapshot, error) {
	if !c.Enabled() {
		return nil, ErrNotConfigured
	}
	data, _, err := c.do(ctx, http.MethodGet, "/api/reviews/"+url.PathEscape(jobID), nil)
	if err != nil {
		return nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("reviewbot: decode status: %w", err)
	}
	return &snap, nil
}

func (c *Client) Capabilities(ctx context.Context) (*Capabilities, error) {
	if !c.Enabled() {
		return nil, ErrNotConfigured
	}
	data, _, err := c.do(ctx, http.MethodGet, "/api/capabilities", nil)
	if err != nil {
		return nil, err
	}
	var parsed Capabilities
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("reviewbot: decode capabilities: %w", err)
	}
	return &parsed, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("reviewbot: encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("reviewbot: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("reviewbot: call sidecar at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reviewbot: read response: %w", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return data, resp.StatusCode, nil
	}
	return data, resp.StatusCode, classifyError(resp.StatusCode, data)
}

func classifyError(status int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	if len(msg) > 240 {
		msg = strings.TrimSpace(msg[:240]) + "…"
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", ErrUnauthorized, msg)
	case http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrNotAllowed, msg)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrJobNotFound, msg)
	default:
		return fmt.Errorf("reviewbot: sidecar returned %d: %s", status, msg)
	}
}
