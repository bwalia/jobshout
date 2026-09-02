package agentschema_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jobshout/server/internal/agentmodules"
	"github.com/jobshout/server/internal/agentschema"
)

const tsSchemaPath = "../../../web/nextjs/lib/agents/input-schemas.ts"

func TestNoDuplicateTypeScriptSchemas(t *testing.T) {
	src, err := os.ReadFile(filepath.Clean(tsSchemaPath))
	if err != nil {
		t.Skipf("TypeScript contract not present (%v); skipping", err)
	}
	if strings.Contains(string(src), "const SCHEMAS") {
		t.Fatal("delete the TypeScript SCHEMAS map; consume GET /api/v1/agent-schemas")
	}
}

func TestRegisteredSchemasHaveFields(t *testing.T) {
	if len(agentschema.Builtins()) == 0 {
		t.Fatal("agent registry is empty — import agentmodules")
	}
	for _, b := range agentschema.Builtins() {
		s := agentschema.ForBuiltin(b)
		if len(s.Fields) == 0 {
			t.Errorf("%s has no fields", b)
		}
		if s.Builtin != b {
			t.Errorf("%s builtin = %q", b, s.Builtin)
		}
	}
}
