package video

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jobshout/server/internal/gatewayauth"
)

var (
	// ErrBusy: the workstation's video queue is full. Not a fault; later works.
	ErrBusy = errors.New("the video workstation is busy")
	// ErrNotFound: the workstation has no such job (pruned, or another host).
	ErrNotFound = errors.New("the video workstation has no such job")
	// ErrClaimed: another caller is collecting this finished render.
	ErrClaimed = errors.New("another server is collecting this video")
)

// Job statuses reported by the workstation.
const (
	JobQueued  = "queued"
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
	// JobCancelled: the render was superseded (its task was launched again).
	JobCancelled = "cancelled"
)

// Shot is one beat of the film: what is said, and what is seen while it is.
type Shot struct {
	Narration string `json:"narration"`
	Visual    string `json:"visual"`
}

// SubmitRequest asks the workstation to render a video.
type SubmitRequest struct {
	ID       string            `json:"id"`
	Title    string            `json:"title,omitempty"`
	Shots    []Shot            `json:"shots"`
	Format   string            `json:"format"`
	Voice    string            `json:"voice,omitempty"`
	Style    string            `json:"style,omitempty"`
	Motion   bool              `json:"motion"`
	Captions bool              `json:"captions"`
	Ref      map[string]string `json:"ref,omitempty"`
}

// Job is a render as the workstation reports it.
type Job struct {
	ID              string            `json:"id"`
	Title           string            `json:"title"`
	Status          string            `json:"status"`
	Stage           string            `json:"stage"`
	Progress        float64           `json:"progress"`
	Message         string            `json:"message"`
	Error           string            `json:"error"`
	Notes           []string          `json:"notes"`
	Shots           int               `json:"shots"`
	DurationSeconds float64           `json:"duration_seconds"`
	MotionModel     string            `json:"motion_model"`
	Ref             map[string]string `json:"ref"`
}

// Capabilities is what the workstation can do right now.
type Capabilities struct {
	FFmpeg      bool     `json:"ffmpeg"`
	Narration   bool     `json:"narration"`
	Motion      bool     `json:"motion"`
	MotionModel string   `json:"motion_model"`
	Voices      []string `json:"voices"`
	MaxShots    int      `json:"max_shots"`
}

// Client talks to the video endpoints of the workstation image service.
type Client struct {
	baseURL string
	auth    *gatewayauth.Signer
	http    *http.Client
}

// NewClient returns nil when there is no workstation to talk to.
func NewClient(baseURL, jwtSecret string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	// No client-wide timeout: a download is minutes on a slow link. Every call
	// carries its own deadline instead.
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), auth: gatewayauth.New(jwtSecret), http: &http.Client{}}
}

func (c *Client) do(ctx context.Context, timeout time.Duration, method, path string, body any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("video: encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("video: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.auth.Apply(req); err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("video: call the workstation at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("video: read response: %w", err)
	}
	switch {
	case gatewayauth.IsAuthStatus(resp.StatusCode):
		return nil, fmt.Errorf("video: the workstation gateway rejected the request (status %d) — check IMAGE_JWT_SECRET matches the gateway's secret", resp.StatusCode)
	case resp.StatusCode == http.StatusServiceUnavailable:
		return nil, fmt.Errorf("%w: %s", ErrBusy, snippet(data))
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, snippet(data))
	case resp.StatusCode == http.StatusConflict:
		return nil, fmt.Errorf("%w: %s", ErrClaimed, snippet(data))
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("video: the workstation returned %d: %s", resp.StatusCode, snippet(data))
	}
	return data, nil
}

func (c *Client) json(ctx context.Context, method, path string, body, out any) error {
	data, err := c.do(ctx, 2*time.Minute, method, path, body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("video: decode %s: %w", path, err)
	}
	return nil
}

// Capabilities reports which stages the workstation has installed.
func (c *Client) Capabilities(ctx context.Context) (*Capabilities, error) {
	var out Capabilities
	return &out, c.json(ctx, http.MethodGet, "/api/videos/capabilities", nil, &out)
}

// Submit queues a render. Submitting an ID twice returns the first job.
func (c *Client) Submit(ctx context.Context, req SubmitRequest) (*Job, error) {
	var out Job
	return &out, c.json(ctx, http.MethodPost, "/api/videos", req, &out)
}

// Job reads one render's state.
func (c *Client) Job(ctx context.Context, id string) (*Job, error) {
	var out Job
	return &out, c.json(ctx, http.MethodGet, "/api/videos/"+id, nil, &out)
}

// Jobs lists the renders the workstation still holds, newest first.
func (c *Client) Jobs(ctx context.Context) ([]Job, error) {
	var out struct {
		Videos []Job `json:"videos"`
	}
	return out.Videos, c.json(ctx, http.MethodGet, "/api/videos", nil, &out)
}

// Claim asks to be the one caller that collects a finished render, for a
// 15-minute lease. ErrClaimed means another caller holds it.
func (c *Client) Claim(ctx context.Context, id, holder string) error {
	_, err := c.do(ctx, 2*time.Minute, http.MethodPost, "/api/videos/"+id+"/claim",
		map[string]any{"holder": holder, "lease_seconds": 900})
	return err
}

// Cancel stops a render: at once if queued, at its next step if running.
func (c *Client) Cancel(ctx context.Context, id string) error {
	_, err := c.do(ctx, time.Minute, http.MethodDelete, "/api/videos/"+id, nil)
	return err
}

// Download fetches a finished render's MP4.
func (c *Client) Download(ctx context.Context, id string) ([]byte, error) {
	return c.do(ctx, 15*time.Minute, http.MethodGet, "/api/videos/"+id+"/file", nil)
}

// snippet reduces an error body to one readable line; the gateway answers
// with an HTML page, and the useful sentence is buried in it.
func snippet(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		return "(empty response)"
	}
	return s
}
