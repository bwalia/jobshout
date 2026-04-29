package blog

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestGitRepo_HappyPath exercises clone → branch → write → commit → push
// against a bare repo on the local filesystem, so no network is required.
func TestGitRepo_HappyPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	dir := t.TempDir()
	bareRepo := filepath.Join(dir, "origin.git")
	if err := runCmd(t, "", "git", "init", "--bare", "--initial-branch=main", bareRepo); err != nil {
		t.Fatalf("init bare: %v", err)
	}

	// Seed the bare repo with an initial commit on main so `git clone
	// --branch main` works.
	seed := filepath.Join(dir, "seed")
	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRun(t, seed, "git", "init", "--initial-branch=main")
	mustRun(t, seed, "git", "config", "user.email", "seed@example.com")
	mustRun(t, seed, "git", "config", "user.name", "Seed")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("# seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, seed, "git", "add", "README.md")
	mustRun(t, seed, "git", "commit", "-m", "seed")
	mustRun(t, seed, "git", "remote", "add", "origin", bareRepo)
	mustRun(t, seed, "git", "push", "origin", "main")

	// Hand-build a gitRepo pointing at the bare origin. The constructor
	// normally embeds x-access-token — we bypass that because local file
	// URLs don't take auth.
	workDir := filepath.Join(dir, "clone")
	g := &gitRepo{
		workDir:     workDir,
		repoURL:     bareRepo,
		authorName:  "Tester",
		authorEmail: "tester@example.com",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := g.clone(ctx, "main"); err != nil {
		t.Fatalf("clone: %v", err)
	}
	if err := g.checkoutNewBranch(ctx, "ai/blog-test"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if err := g.writeFile("content/blogs/2026-04-29-test.md", "# Hello\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := g.commit(ctx, "Add test blog"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := g.push(ctx, "ai/blog-test"); err != nil {
		t.Fatalf("push: %v", err)
	}

	// Confirm the branch landed in the bare repo.
	out, err := exec.Command("git", "--git-dir="+bareRepo, "branch", "--list", "ai/blog-test").Output()
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(string(out), "ai/blog-test") {
		t.Errorf("branch not pushed: %q", out)
	}
}

func TestRedactToken(t *testing.T) {
	in := "fatal: could not read from https://x-access-token:ghp_REDACTME@github.com/foo/bar.git"
	out := redactToken(in)
	if strings.Contains(out, "ghp_REDACTME") {
		t.Errorf("token leaked: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Errorf("redaction marker missing: %s", out)
	}
}

// TestCommit_NoChanges verifies we fail loudly if nothing was staged — a
// silent "empty commit" would lead to a PR with no files.
func TestCommit_NoChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	mustRun(t, dir, "git", "init", "--initial-branch=main")
	mustRun(t, dir, "git", "config", "user.email", "t@x")
	mustRun(t, dir, "git", "config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "git", "add", ".")
	mustRun(t, dir, "git", "commit", "-m", "init")

	g := &gitRepo{workDir: dir, authorName: "T", authorEmail: "t@x"}
	if err := g.commit(context.Background(), "noop"); err == nil {
		t.Fatal("expected error when nothing to commit")
	}
}

func runCmd(t *testing.T, dir, name string, args ...string) error {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return wrapErr(name+" "+strings.Join(args, " ")+": "+buf.String(), err)
	}
	return nil
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	if err := runCmd(t, dir, name, args...); err != nil {
		t.Fatal(err)
	}
}

func wrapErr(prefix string, err error) error { return &prefixErr{p: prefix, e: err} }

type prefixErr struct {
	p string
	e error
}

func (p *prefixErr) Error() string { return p.p + ": " + p.e.Error() }
func (p *prefixErr) Unwrap() error { return p.e }
