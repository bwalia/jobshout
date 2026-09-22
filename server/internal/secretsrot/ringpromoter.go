package secretsrot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RPClient restarts rings through Ring Promoter so pods pick up rotated
// secrets. Endpoints (POST .../rings/{ring}/restart?async=1) come from the
// ring-promoter restart change.
type RPClient struct {
	base         string
	token        string
	prodPassword string
	http         *http.Client
	poll         time.Duration
	timeout      time.Duration
}

// NewRPClient builds a client from Config.
func NewRPClient(cfg Config) *RPClient {
	poll := cfg.RPPollInterval
	if poll <= 0 {
		poll = 5 * time.Second
	}
	timeout := cfg.RPTimeout
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}
	base := cfg.RPURL
	if base == "" {
		base = DefaultRPURL
	}
	return &RPClient{
		base:         strings.TrimRight(base, "/"),
		token:        cfg.RPToken,
		prodPassword: cfg.RPProdPassword,
		http:         &http.Client{Timeout: 30 * time.Second},
		poll:         poll,
		timeout:      timeout,
	}
}

func (c *RPClient) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Actor", "jobshout-secrets-rotation")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = http.StatusText(resp.StatusCode)
		}
		if len(e.Error) > 240 {
			e.Error = e.Error[:240] + "…"
		}
		return resp.StatusCode, fmt.Errorf("ring promoter HTTP %d: %s", resp.StatusCode, e.Error)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("ring promoter decode: %w", err)
		}
	}
	return resp.StatusCode, nil
}

// StartRestart queues an async restart for one ring and returns the job id;
// deployments narrows the restart to those Deployments; nil lets Ring
// Promoter use its default (jobshout-api + jobshout-web for jobshout).
func (c *RPClient) StartRestart(ctx context.Context, app, ring, reason string, deployments []string) (string, error) {
	body := map[string]any{"reason": reason}
	if ring == "prod" && c.prodPassword != "" {
		body["password"] = c.prodPassword
	}
	if len(deployments) > 0 {
		body["deployments"] = deployments
	}
	var out struct {
		JobID string `json:"job_id"`
	}
	path := fmt.Sprintf("/api/apps/%s/rings/%s/restart?async=1", url.PathEscape(app), url.PathEscape(ring))
	code, err := c.do(ctx, http.MethodPost, path, body, &out)
	if err != nil {
		return "", err
	}
	if code != http.StatusAccepted || out.JobID == "" {
		return "", fmt.Errorf("ring promoter restart %s: expected 202 with job_id, got HTTP %d", ring, code)
	}
	return out.JobID, nil
}

func rpJobTerminal(status string) (done, ok bool) {
	switch strings.ToLower(status) {
	case "success", "succeeded", "completed":
		return true, true
	case "failed", "failure", "error", "cancelled", "canceled":
		return true, false
	}
	return false, false
}

// WaitJob polls the job until it is terminal or the timeout elapses.
func (c *RPClient) WaitJob(ctx context.Context, app, jobID string) error {
	deadline := time.Now().Add(c.timeout)
	path := fmt.Sprintf("/api/apps/%s/jobs/%s", url.PathEscape(app), url.PathEscape(jobID))
	for {
		var job struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if _, err := c.do(ctx, http.MethodGet, path, nil, &job); err != nil {
			return err
		}
		if done, ok := rpJobTerminal(job.Status); done {
			if ok {
				return nil
			}
			msg := job.Error
			if len(msg) > 240 {
				msg = msg[:240] + "…"
			}
			if msg != "" {
				return fmt.Errorf("job %s %s: %s", jobID, job.Status, msg)
			}
			return fmt.Errorf("job %s %s", jobID, job.Status)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("job %s still %q after %s", jobID, job.Status, c.timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.poll):
		}
	}
}

// RingHealthy reads GET /api/apps/{app}/rings and reports ring.healthy.
func (c *RPClient) RingHealthy(ctx context.Context, app, ring string) (bool, error) {
	var out struct {
		Rings []struct {
			Ring    json.RawMessage `json:"ring"`
			Healthy bool            `json:"healthy"`
		} `json:"rings"`
	}
	if _, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/apps/%s/rings", url.PathEscape(app)), nil, &out); err != nil {
		return false, err
	}
	for _, r := range out.Rings {
		var named struct {
			Name string `json:"name"`
		}
		name := ""
		if json.Unmarshal(r.Ring, &named) == nil && named.Name != "" {
			name = named.Name
		} else {
			_ = json.Unmarshal(r.Ring, &name)
		}
		if name == ring {
			return r.Healthy, nil
		}
	}
	return false, fmt.Errorf("ring %s not reported by ring promoter", ring)
}
