package strix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

type Client struct {
	strixPath string
	runsDir   string
	llmKey    string
	llmModel  string
	logger    *zap.Logger
}

type RunResult struct {
	Status       string     `json:"status"` // completed, failed, budget_exceeded
	Target       string     `json:"target"`
	FindingCount int        `json:"finding_count"`
	Findings     []Finding  `json:"findings"`
	RunID        string     `json:"run_id"`
	ExecutedAt   time.Time  `json:"executed_at"`
}

type Finding struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Severity       string  `json:"severity"` // critical, high, medium, low
	CVSSScore      float32 `json:"cvss_score"`
	Category       string  `json:"category"`
	ReproductionSteps string `json:"reproduction_steps"`
}

// VulnerabilitiesJSON is the structure of vulnerabilities.json from Strix
type VulnerabilitiesJSON struct {
	Vulnerabilities []struct {
		ID                string  `json:"id"`
		Title             string  `json:"title"`
		Description       string  `json:"description"`
		Severity          string  `json:"severity"`
		CVSSScore         float32 `json:"cvss_score"`
		Category          string  `json:"category"`
		ReproductionSteps string  `json:"reproduction_steps"`
	} `json:"vulnerabilities"`
}

// RunJSON is the structure of run.json from Strix
type RunJSON struct {
	Status        string `json:"status"`
	Target        string `json:"target"`
	FindingCount  int    `json:"finding_count"`
	ExecutedAtRaw string `json:"executed_at"`
}

func NewClient(strixPath, runsDir, llmModel, llmKey string, logger *zap.Logger) *Client {
	if strixPath == "" {
		strixPath = "strix"
	}
	if runsDir == "" {
		runsDir = "./strix_runs"
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{
		strixPath: strixPath,
		runsDir:   runsDir,
		llmModel:  llmModel,
		llmKey:    llmKey,
		logger:    logger,
	}
}

// Scan runs Strix in headless mode against target
func (c *Client) Scan(ctx context.Context, target, scanMode string, maxBudget int) (*RunResult, error) {
	// Validate inputs
	if target == "" {
		return nil, fmt.Errorf("target cannot be empty")
	}
	if scanMode != "quick" && scanMode != "standard" && scanMode != "deep" {
		scanMode = "quick"
	}

	c.logger.Info("starting pentest scan", zap.String("target", target), zap.String("scanMode", scanMode))

	// Build command: strix -n --target <target> --scan-mode <mode> --max-budget <budget>
	args := []string{"-n", "--target", target, "--scan-mode", scanMode}
	if maxBudget > 0 {
		args = append(args, "--max-budget", strconv.Itoa(maxBudget))
	}

	cmd := exec.CommandContext(ctx, c.strixPath, args...)

	// Set environment variables
	env := os.Environ()
	env = append(env, fmt.Sprintf("STRIX_LLM=%s", c.llmModel))
	env = append(env, fmt.Sprintf("LLM_API_KEY=%s", c.llmKey))
	cmd.Env = env

	// Run command
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	c.logger.Debug("strix scan output", zap.String("output", outputStr))

	// Distinguish "the binary ran and exited non-zero" from "the binary could
	// not be started at all". A non-nil err that is not an *exec.ExitError means
	// the command never ran — the strix binary is missing, not executable, or the
	// context was cancelled — and there is no output or run to parse. Surfacing
	// that directly avoids the misleading "failed to extract run ID" that comes
	// from trying to parse empty output.
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			if errors.Is(err, exec.ErrNotFound) || strings.Contains(err.Error(), "executable file not found") {
				return nil, fmt.Errorf("strix binary %q not found — install Strix (curl -sSL https://strix.ai/install | bash) and ensure Docker is running: %w", c.strixPath, err)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("strix scan cancelled or timed out: %w", err)
			}
			return nil, fmt.Errorf("strix could not be started: %w", err)
		}
	}

	// Extract run ID from output or find most recent run dir
	runID := extractRunID(outputStr)
	if runID == "" {
		// The binary ran (exit code captured above) but wrote no parseable run
		// path. Include a snippet of its output so the cause is visible.
		snippet := strings.TrimSpace(outputStr)
		if len(snippet) > 300 {
			snippet = snippet[:300] + "…"
		}
		return nil, fmt.Errorf("strix exited with code %d but produced no run ID; output: %s", exitCode, snippet)
	}

	result := &RunResult{
		RunID:      runID,
		Target:     target,
		ExecutedAt: time.Now(),
	}

	// Parse vulnerabilities.json and run.json
	findings, err := c.parseFindings(runID)
	if err != nil {
		c.logger.Warn("failed to parse findings", zap.String("runID", runID), zap.Error(err))
		result.Status = "failed"
		return result, fmt.Errorf("failed to parse findings: %w", err)
	}

	result.Findings = findings
	result.FindingCount = len(findings)

	// Map exit codes: 0 = clean (no vulns), 1 = fatal error, 2 = vulnerabilities found
	switch exitCode {
	case 0:
		result.Status = "completed"
	case 2:
		// Vulnerabilities found - still successful scan
		result.Status = "completed"
	default:
		// Check for budget exceeded or other errors
		if strings.Contains(outputStr, "budget") || strings.Contains(outputStr, "exceeded") {
			result.Status = "budget_exceeded"
		} else {
			result.Status = "failed"
		}
	}

	c.logger.Info("pentest scan completed", zap.String("runID", runID), zap.String("status", result.Status), zap.Int("findings", result.FindingCount))

	return result, nil
}

// parseFindings reads vulnerabilities.json from the run directory
func (c *Client) parseFindings(runID string) ([]Finding, error) {
	vulnFile := filepath.Join(c.runsDir, runID, "vulnerabilities.json")

	data, err := os.ReadFile(vulnFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read vulnerabilities.json: %w", err)
	}

	var vulns VulnerabilitiesJSON
	if err := json.Unmarshal(data, &vulns); err != nil {
		return nil, fmt.Errorf("failed to parse vulnerabilities.json: %w", err)
	}

	findings := make([]Finding, len(vulns.Vulnerabilities))
	for i, v := range vulns.Vulnerabilities {
		findings[i] = Finding{
			ID:                v.ID,
			Title:             v.Title,
			Description:       v.Description,
			Severity:          v.Severity,
			CVSSScore:         v.CVSSScore,
			Category:          v.Category,
			ReproductionSteps: v.ReproductionSteps,
		}
	}

	return findings, nil
}

// extractRunID tries to extract run ID from strix output
// Strix prints something like "Scan complete. Results saved to strix_runs/2024-01-15T10-30-45-abc123"
func extractRunID(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "strix_runs/") {
			parts := strings.Split(line, "strix_runs/")
			if len(parts) > 1 {
				// Extract the run ID (everything after strix_runs/)
				runID := strings.Fields(parts[1])[0]
				return strings.TrimSpace(runID)
			}
		}
	}
	// Fallback: return empty, error will be handled upstream
	return ""
}
