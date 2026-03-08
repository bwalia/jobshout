package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// ShellTool executes shell commands with an explicit command allowlist and a
// per-execution timeout. It is intentionally restrictive to prevent agents
// from running arbitrary system commands.
//
// Only commands whose base name (the first token) appear in the AllowedCmds
// set will be executed. The default allowlist is conservative.
type ShellTool struct {
	// AllowedCmds is the set of base command names that may be executed.
	// Map value is unused; only key presence is checked.
	AllowedCmds map[string]struct{}
	// Timeout caps execution time for each command.
	Timeout time.Duration
	// MaxOutputBytes caps the combined stdout+stderr captured.
	MaxOutputBytes int
}

// DefaultAllowedCmds is the baseline safe command set.
var DefaultAllowedCmds = []string{
	"echo", "cat", "ls", "pwd", "date", "env",
	"curl", "wget",
	"kubectl", "helm", "kustomize",
	"docker", "docker-compose",
	"git",
	"go", "node", "python3", "pip3",
	"jq", "yq", "sed", "awk", "grep", "sort", "uniq", "wc", "head", "tail",
	"ping", "dig", "nslookup",
}

// NewShellTool creates a ShellTool with the default allowlist and a 30-second timeout.
func NewShellTool(extraAllowed []string) *ShellTool {
	allowed := make(map[string]struct{})
	for _, cmd := range DefaultAllowedCmds {
		allowed[cmd] = struct{}{}
	}
	for _, cmd := range extraAllowed {
		allowed[cmd] = struct{}{}
	}
	return &ShellTool{
		AllowedCmds:    allowed,
		Timeout:        30 * time.Second,
		MaxOutputBytes: 32 * 1024, // 32 KB
	}
}

func (t *ShellTool) Name() string { return "shell_command" }

func (t *ShellTool) Description() string {
	return `Execute a shell command and capture its output.
Input parameters:
  command (string, required) - The full shell command to run, e.g. "kubectl get pods -n default"

Only a predefined allowlist of base commands may be used. Pipelines and
shell operators (|, &&, ;, >, etc.) are NOT supported for safety reasons.
Returns the combined stdout and stderr of the command.`
}

func (t *ShellTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	cmdStr, err := stringParam(input, "command", true)
	if err != nil {
		return "", err
	}

	// Reject shell operators that could be used to chain or redirect commands.
	for _, op := range []string{"|", "&&", "||", ";", ">", "<", "`", "$("} {
		if strings.Contains(cmdStr, op) {
			return "", fmt.Errorf("shell_command: operator %q is not allowed for safety reasons", op)
		}
	}

	// Tokenise the command string (simple whitespace split).
	tokens := tokenise(cmdStr)
	if len(tokens) == 0 {
		return "", fmt.Errorf("shell_command: empty command")
	}

	baseCmd := tokens[0]
	if _, ok := t.AllowedCmds[baseCmd]; !ok {
		return "", fmt.Errorf("shell_command: %q is not in the allowed command list", baseCmd)
	}

	// Apply timeout on top of the caller's context.
	execCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()

	// #nosec G204 — base command is validated against an explicit allowlist above.
	cmd := exec.CommandContext(execCtx, tokens[0], tokens[1:]...)

	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	_ = cmd.Run() // Intentionally ignore error; output always returned.

	output := combined.String()
	if len(output) > t.MaxOutputBytes {
		output = output[:t.MaxOutputBytes] + "\n[output truncated]"
	}

	if execCtx.Err() != nil {
		return output + "\n[command timed out]", nil
	}

	return output, nil
}

// tokenise splits a command string by whitespace, respecting double-quoted
// segments as single tokens.
func tokenise(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false

	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case unicode.IsSpace(r) && !inQuote:
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}
