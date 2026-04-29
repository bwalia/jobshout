package blog

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitRepo wraps a local clone of a GitHub repo with authenticated push.
// The token is injected into the origin URL rather than ~/.netrc to stay
// per-process and avoid leaking credentials to other users on the host.
type gitRepo struct {
	workDir     string
	repoURL     string // with embedded token
	authorName  string
	authorEmail string
}

func newGitRepo(workDir, owner, repo, token, authorName, authorEmail string) *gitRepo {
	// x-access-token is the recommended bot identity for GitHub fine-grained
	// PATs and GitHub App installation tokens; classic PATs also accept it.
	repoURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, owner, repo)
	return &gitRepo{
		workDir:     workDir,
		repoURL:     repoURL,
		authorName:  authorName,
		authorEmail: authorEmail,
	}
}

// clone replaces any existing working directory with a fresh shallow clone of
// baseBranch. Shallow to keep disk + network tiny on the runner VM.
func (g *gitRepo) clone(ctx context.Context, baseBranch string) error {
	if err := os.RemoveAll(g.workDir); err != nil {
		return fmt.Errorf("blog: clean workdir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(g.workDir), 0o755); err != nil {
		return fmt.Errorf("blog: mkdir parent: %w", err)
	}
	return g.run(ctx, "",
		"git", "clone",
		"--depth", "1",
		"--branch", baseBranch,
		g.repoURL, g.workDir,
	)
}

// checkoutNewBranch creates and switches to branch from the current HEAD.
func (g *gitRepo) checkoutNewBranch(ctx context.Context, branch string) error {
	return g.run(ctx, g.workDir, "git", "checkout", "-b", branch)
}

// writeFile writes content to a path relative to the repo root, creating
// intermediate directories as needed.
func (g *gitRepo) writeFile(relPath, content string) error {
	abs := filepath.Join(g.workDir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("blog: mkdir for %q: %w", relPath, err)
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

// commit stages all current changes and creates a commit with the given
// message. Skips if there is nothing to commit.
func (g *gitRepo) commit(ctx context.Context, message string) error {
	if err := g.run(ctx, g.workDir, "git", "add", "-A"); err != nil {
		return err
	}

	// Check for staged changes before committing — an empty commit would
	// surface as a mysterious "nothing to commit" non-zero exit.
	status, err := g.capture(ctx, g.workDir, "git", "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		return fmt.Errorf("blog: commit: no changes staged")
	}

	return g.run(ctx, g.workDir,
		"git",
		"-c", "user.name="+g.authorName,
		"-c", "user.email="+g.authorEmail,
		"commit", "-m", message,
	)
}

// push sends the branch to origin. `-u` is intentionally skipped — we never
// re-use the clone.
func (g *gitRepo) push(ctx context.Context, branch string) error {
	return g.run(ctx, g.workDir, "git", "push", "origin", branch)
}

// cleanup best-effort removes the local clone.
func (g *gitRepo) cleanup() {
	_ = os.RemoveAll(g.workDir)
}

// run executes cmd and returns an error with captured combined output on
// failure. `dir` is optional; empty means inherit the caller's cwd.
func (g *gitRepo) run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("blog: %s %s: %w — output: %s",
			name, strings.Join(args, " "), err, redactToken(out.String()))
	}
	return nil
}

// capture is like run but returns stdout instead of logging it.
func (g *gitRepo) capture(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("blog: %s %s: %w — output: %s",
			name, strings.Join(args, " "), err, redactToken(out.String()))
	}
	return out.String(), nil
}

// redactToken strips any x-access-token:<tok>@ pattern from git output so we
// never log the PAT even if git dumps the URL in an error message.
func redactToken(s string) string {
	start := strings.Index(s, "x-access-token:")
	if start < 0 {
		return s
	}
	end := strings.Index(s[start:], "@")
	if end < 0 {
		return s
	}
	return s[:start] + "x-access-token:***REDACTED***" + s[start+end:]
}
