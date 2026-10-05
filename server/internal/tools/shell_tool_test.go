package tools

import (
	"context"
	"strings"
	"testing"
)

func TestShellTool_DangerousCommandsNotAllowed(t *testing.T) {
	st := NewShellTool(nil)
	for _, cmd := range []string{"env", "python3", "pip3", "node", "go"} {
		if _, ok := st.AllowedCmds[cmd]; ok {
			t.Errorf("%q must not be in the default allowlist: it defeats the allowlist", cmd)
		}
		out, err := st.Execute(context.Background(), map[string]any{"command": cmd + " --version"})
		if err == nil {
			t.Errorf("%q should be rejected, got output %q", cmd, out)
		}
	}
}

func TestShellTool_SafeEnvExcludesSecrets(t *testing.T) {
	// A secret in the server environment must not reach the child.
	t.Setenv("JWT_SECRET", "super-secret-value")
	t.Setenv("PATH", "/usr/bin:/bin")

	for _, kv := range safeEnv() {
		if strings.HasPrefix(kv, "JWT_SECRET=") {
			t.Fatal("safeEnv leaked JWT_SECRET to the child environment")
		}
	}
	// PATH is passed through so allowed commands can still be found.
	var hasPath bool
	for _, kv := range safeEnv() {
		if strings.HasPrefix(kv, "PATH=") {
			hasPath = true
		}
	}
	if !hasPath {
		t.Fatal("safeEnv dropped PATH; allowed commands could not run")
	}
}

func TestShellTool_AllowedCommandRuns(t *testing.T) {
	st := NewShellTool(nil)
	out, err := st.Execute(context.Background(), map[string]any{"command": "echo hello-shell"})
	if err != nil {
		t.Fatalf("echo should run: %v", err)
	}
	if !strings.Contains(out, "hello-shell") {
		t.Fatalf("unexpected output %q", out)
	}
}

func TestShellTool_SecretNotReadableViaChildEnv(t *testing.T) {
	// End to end: even reading the child's own environment must not reveal the
	// secret, because Execute replaces the environment with safeEnv.
	t.Setenv("JWT_SECRET", "super-secret-value")
	// Allow a command that prints the environment so we can prove it is clean.
	st := NewShellTool([]string{"printenv"})
	out, _ := st.Execute(context.Background(), map[string]any{"command": "printenv"})
	if strings.Contains(out, "super-secret-value") {
		t.Fatalf("child saw the server secret: %q", out)
	}
}
