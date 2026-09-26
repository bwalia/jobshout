// Package strix is the cluster's end of the workstation pentest service.
//
// The scanner no longer runs in the API pod. A pod has no Docker daemon to
// sandbox scans in, a scan outlives any deploy or node drain, and its results
// must not land on one replica's disk where another replica cannot find them.
// So Strix moved to the Mac Studio behind the same JWT-gated HTTP arrangement
// that already fronts Ollama and the image service, and this client talks to it.
//
// A scan takes minutes to hours — far too long to hold an HTTP request open
// through Cloudflare and pop0 — so the contract is start-and-poll rather than a
// single blocking call. Start hands a target over and returns immediately;
// Status is polled until the run reaches a terminal state. The polling itself is
// done durably, off Postgres, by pentest_reconciler.go, so a pod that dies
// mid-scan loses nothing.
package strix

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

	"github.com/jobshout/server/internal/gatewayauth"
)

// Sentinel errors the reconciler must be able to tell apart, because one is
// permanent and the rest are not. Callers compare with errors.Is; the wrapped
// message carries the service's own explanation.
var (
	// ErrOutOfScope is the service refusing a target (403 from scope.py on the
	// workstation). Permanent: the same target is refused on every retry, so the
	// run is failed now rather than polled forever.
	ErrOutOfScope = errors.New("strix: target refused as out of scope")

	// ErrBusy is the service's queue being full (503, with a Retry-After). The
	// request was valid and will be accepted once the queue drains, so the run
	// stays queued and is retried after a backoff. Not a transport failure.
	ErrBusy = errors.New("strix: pentest service is busy")

	// ErrUnauthorized is the gateway rejecting the request (401, or a 403 whose
	// body is about the token rather than the target). A configuration or clock
	// problem, not a scan problem: retrying with the same secret produces the
	// same verdict.
	ErrUnauthorized = errors.New("strix: pentest gateway rejected the request")

	// ErrGateway is the edge in front of the service returning 502/504 (or an
	// HTML 503) — typically WSL Proxy while the Mac is still scanning. Transient:
	// the reconciler must keep polling, not fail the run after a handful of
	// flaps. Distinct from ErrBusy, which is the FastAPI queue being full.
	ErrGateway = errors.New("strix: edge gateway temporarily unavailable")
)

// Finding mirrors the service's FindingOut, in the field names the Go side
// already stores, so a result needs no translation layer on the way through.
type Finding struct {
	ID                string  `json:"id"`
	Title             string  `json:"title"`
	Description       string  `json:"description"`
	Severity          string  `json:"severity"`
	CVSSScore         float32 `json:"cvss_score"`
	Category          string  `json:"category"`
	ReproductionSteps string  `json:"reproduction_steps"`
}

// StartRequest is a scan to hand over. Target and ScanMode describe the work;
// RunRef is the caller's own run id and doubles as the idempotency key, so a
// retried Start after a dropped response returns the existing scan instead of
// buying a second hours-long one against a live target.
type StartRequest struct {
	Target      string
	ScanMode    string
	MaxBudget   int    // 0 omits the cap
	RunRef      string // pentest_runs.id — also the idempotency key
	Instruction string // scope note for the audit trail; not passed to the scanner
	RequestedBy string // who authorised the scan, for the service's audit log
}

// RunHandle is what the service returns from a successful Start.
type RunHandle struct {
	RemoteRunID   string
	RunRef        string
	Status        string
	QueuePosition int
	// Existing is true when the service already had a scan for this RunRef and
	// returned it rather than starting a new one — the idempotency path.
	Existing bool
}

// RunResult is one poll of a scan. The reconciler finalises a run from it.
type RunResult struct {
	RemoteRunID    string
	RunRef         string
	Status         string // queued|running|completed|failed|budget_exceeded|cancelled
	Target         string
	ScanMode       string
	StartedAt      *time.Time
	CompletedAt    *time.Time
	DurationMS     int
	ExitCode       *int
	FindingCount   int
	Findings       []Finding
	Error          string
	LogTail        string
	ReportMarkdown string
	TargetEngaged  *bool
}

// Terminal reports whether the run has reached a state that will not change.
func (r *RunResult) Terminal() bool {
	switch r.Status {
	case "completed", "failed", "budget_exceeded", "cancelled":
		return true
	default:
		return false
	}
}

// Capabilities is /api/capabilities — everything a scan needs, checked
// deliberately rather than on every liveness ping. Used in startup logs and ops
// checks, not on the scan path.
type Capabilities struct {
	Strix struct {
		Available bool   `json:"available"`
		Version   string `json:"version"`
	} `json:"strix"`
	Docker struct {
		Available bool `json:"available"`
	} `json:"docker"`
	LLM struct {
		Model     string `json:"model"`
		APIBase   string `json:"api_base"`
		Reachable *bool  `json:"reachable"`
		// ModelPresent is nil for a hosted provider (no local /api/tags to check);
		// for local Ollama it says whether the wanted model is actually pulled, the
		// difference between "Ollama is up" and "the scan can start".
		ModelPresent *bool `json:"model_present"`
	} `json:"llm"`
	Scope struct {
		RuleCount     int  `json:"rule_count"`
		ScansAnything bool `json:"scans_anything"`
	} `json:"scope"`
}

// Client talks to the workstation pentest service.
type Client struct {
	baseURL    string
	auth       *gatewayauth.Signer
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient builds a client for the pentest service at baseURL.
//
// timeout bounds ONE HTTP call, not one scan — conflating the two is how a
// two-hour client timeout hides a dead service for two hours. An empty secret
// means no gateway, which is exactly right when the service runs on the same
// machine as this process and checks nothing.
func NewClient(baseURL, jwtSecret string, timeout time.Duration, logger *zap.Logger) *Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		auth:       gatewayauth.New(jwtSecret),
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger,
	}
}

// Enabled reports whether the client has somewhere to send scans. An empty base
// URL means the feature is off for this ring, and every method short-circuits
// rather than trying to reach an address that is not there.
func (c *Client) Enabled() bool { return c != nil && c.baseURL != "" }

// UsesGateway reports whether requests are signed, for startup logs.
func (c *Client) UsesGateway() bool { return c.auth.Enabled() }

// Start hands a scan to the service and returns a handle. The run is not yet
// running when this returns — the service has accepted it into its queue.
func (c *Client) Start(ctx context.Context, req StartRequest) (*RunHandle, error) {
	if !c.Enabled() {
		return nil, errNotConfigured
	}
	scanMode := strings.ToLower(strings.TrimSpace(req.ScanMode))
	if scanMode == "" {
		scanMode = "quick"
	}

	data, _, err := c.do(ctx, http.MethodPost, "/api/scan", startRequestWire{
		Target:      req.Target,
		ScanMode:    scanMode,
		MaxBudget:   req.MaxBudget,
		RunRef:      req.RunRef,
		Instruction: req.Instruction,
		RequestedBy: req.RequestedBy,
	})
	if err != nil {
		return nil, err
	}

	var parsed scanAcceptedWire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("strix: decode scan response: %w", err)
	}
	return &RunHandle{
		RemoteRunID:   parsed.RunID,
		RunRef:        parsed.RunRef,
		Status:        parsed.Status,
		QueuePosition: parsed.QueuePosition,
		Existing:      parsed.Existing,
	}, nil
}

// Status polls one scan by the remote run id Start returned.
func (c *Client) Status(ctx context.Context, remoteRunID string) (*RunResult, error) {
	if !c.Enabled() {
		return nil, errNotConfigured
	}
	data, _, err := c.do(ctx, http.MethodGet, "/api/scan/"+url.PathEscape(remoteRunID), nil)
	if err != nil {
		return nil, err
	}

	var parsed scanStatusWire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("strix: decode scan status: %w", err)
	}
	return &RunResult{
		RemoteRunID:    parsed.RunID,
		RunRef:         parsed.RunRef,
		Status:         parsed.Status,
		Target:         parsed.Target,
		ScanMode:       parsed.ScanMode,
		StartedAt:      parseTime(parsed.StartedAt),
		CompletedAt:    parseTime(parsed.CompletedAt),
		DurationMS:     parsed.DurationMS,
		ExitCode:       parsed.ExitCode,
		FindingCount:   parsed.FindingCount,
		Findings:       parsed.Findings,
		Error:          parsed.Error,
		LogTail:        parsed.LogTail,
		ReportMarkdown: parsed.ReportMarkdown,
		TargetEngaged:  parsed.TargetEngaged,
	}, nil
}

// Cancel asks the service to stop a scan. Best-effort: a run that had already
// finished is not an error here, and the service reports that in its body.
func (c *Client) Cancel(ctx context.Context, remoteRunID string) error {
	if !c.Enabled() {
		return errNotConfigured
	}
	_, _, err := c.do(ctx, http.MethodDelete, "/api/scan/"+url.PathEscape(remoteRunID), nil)
	return err
}

// Capabilities reports what the service can do, for ops and startup logs.
func (c *Client) Capabilities(ctx context.Context) (*Capabilities, error) {
	if !c.Enabled() {
		return nil, errNotConfigured
	}
	data, _, err := c.do(ctx, http.MethodGet, "/api/capabilities", nil)
	if err != nil {
		return nil, err
	}
	var parsed Capabilities
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("strix: decode capabilities: %w", err)
	}
	return &parsed, nil
}

var errNotConfigured = errors.New("strix: no base URL configured; the pentest feature is off for this ring")

// ─── wire types ─────────────────────────────────────────────────────────────

type startRequestWire struct {
	Target      string `json:"target"`
	ScanMode    string `json:"scan_mode"`
	MaxBudget   int    `json:"max_budget,omitempty"`
	RunRef      string `json:"run_ref,omitempty"`
	Instruction string `json:"instruction,omitempty"`
	RequestedBy string `json:"requested_by,omitempty"`
}

type scanAcceptedWire struct {
	RunID         string `json:"run_id"`
	RunRef        string `json:"run_ref"`
	Status        string `json:"status"`
	QueuePosition int    `json:"queue_position"`
	Existing      bool   `json:"existing"`
}

type scanStatusWire struct {
	RunID          string    `json:"run_id"`
	RunRef         string    `json:"run_ref"`
	Status         string    `json:"status"`
	Target         string    `json:"target"`
	ScanMode       string    `json:"scan_mode"`
	StartedAt      *string   `json:"started_at"`
	CompletedAt    *string   `json:"completed_at"`
	DurationMS     int       `json:"duration_ms"`
	ExitCode       *int      `json:"exit_code"`
	FindingCount   int       `json:"finding_count"`
	Findings       []Finding `json:"findings"`
	Error          string    `json:"error"`
	LogTail        string    `json:"log_tail"`
	ReportMarkdown string    `json:"report_markdown"`
	TargetEngaged  *bool     `json:"target_engaged"`
}

// ─── transport ──────────────────────────────────────────────────────────────

// do performs a signed request and returns the body, the status, and — for a
// non-2xx — a typed error the reconciler can branch on.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("strix: encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("strix: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.auth.Apply(req); err != nil {
		return nil, 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("strix: call pentest service at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("strix: read response: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return data, resp.StatusCode, nil
	}
	return data, resp.StatusCode, classifyError(resp.StatusCode, data)
}

// classifyError maps an HTTP status onto one of the sentinel errors the
// reconciler branches on.
func classifyError(status int, body []byte) error {
	switch status {
	case http.StatusBadGateway, http.StatusGatewayTimeout:
		return fmt.Errorf("%w (status %d): %s", ErrGateway, status, gatewaySnippet(body))
	case http.StatusServiceUnavailable:
		// FastAPI's queue-full 503 is JSON. An HTML 503 is the edge in front of
		// it (WSL Proxy), which is a flap, not a full queue.
		if isGatewayBody(body) {
			return fmt.Errorf("%w (status 503): %s", ErrGateway, gatewaySnippet(body))
		}
		return fmt.Errorf("%w: %s", ErrBusy, snippet(body))
	case http.StatusUnauthorized:
		return fmt.Errorf(
			"%w (status %d) — the token was missing or not accepted; check that "+
				"STRIX_JWT_SECRET matches the service's secret and this host's clock "+
				"is accurate. Response: %s",
			ErrUnauthorized, status, snippet(body),
		)
	case http.StatusForbidden:
		// The service returns 403 for two unrelated reasons: scope.py refusing a
		// target, and auth.py rejecting a token (expired, bad signature, no app
		// claim). They must not be conflated — an out-of-scope target is permanent
		// and fails the run, while a token problem is a fixable misconfiguration —
		// so the body is what tells them apart.
		if isAuthBody(body) {
			return fmt.Errorf(
				"%w (status 403) — the token did not verify; check STRIX_JWT_SECRET "+
					"and this host's clock. Response: %s",
				ErrUnauthorized, snippet(body),
			)
		}
		return fmt.Errorf("%w: %s", ErrOutOfScope, snippet(body))
	default:
		if isGatewayBody(body) {
			return fmt.Errorf("%w (status %d): %s", ErrGateway, status, gatewaySnippet(body))
		}
		return fmt.Errorf("strix: pentest service returned %d: %s", status, snippet(body))
	}
}

// isGatewayBody is true when the response is an HTML error page from the edge
// (WSL Proxy / nginx) rather than JSON from FastAPI.
func isGatewayBody(body []byte) bool {
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "wsl proxy") ||
		strings.Contains(lower, "<!doctype html") ||
		strings.Contains(lower, "<html")
}

// gatewaySnippet keeps HTML error pages out of the UI. Operators need to know
// it was the edge, not a 240-character dump of <!DOCTYPE html>.
func gatewaySnippet(body []byte) string {
	if isGatewayBody(body) {
		return "WSL Proxy / edge returned an HTML error page (the scanner on the workstation is likely still running)"
	}
	return snippet(body)
}

// isAuthBody distinguishes an auth rejection from a scope rejection when both
// arrive as 403. The service's auth messages all name the token or the header;
// its scope messages name the target and never do.
func isAuthBody(body []byte) bool {
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "token") || strings.Contains(lower, "x-api-key")
}

// parseTime turns the service's ISO-8601 timestamps into *time.Time, tolerating
// a missing field (a scan that has not started has no started_at).
func parseTime(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	// RFC3339 parsing accepts the fractional seconds Python's isoformat emits.
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil
	}
	return &t
}

// snippetLimit bounds how much of an upstream body reaches an error message:
// long enough to identify the failure, short enough to stay readable on a card
// in the UI, which is where these are read.
const snippetLimit = 240

// snippet reduces a response body to something worth showing a human.
func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "(empty response)"
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > snippetLimit {
		s = strings.TrimSpace(s[:snippetLimit]) + "…"
	}
	return s
}
