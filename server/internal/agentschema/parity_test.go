package agentschema_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/agentmodule"
	_ "github.com/jobshout/server/internal/agentmodules"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
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

func TestRegisteredModuleContract(t *testing.T) {
	want := []struct {
		builtin string
		tab     string
		stay    bool
		keys    []string
	}{
		{model.BuiltinPentester, "pentest", false, []string{"target", "scan_mode", "max_budget", "instruction"}},
		{model.BuiltinPRReviewer, "review", false, []string{"repo", "pr_number", "dry_run"}},
		{model.BuiltinMail, "mail", false, []string{"senders", "subject_prefixes", "labels", "knowledge_notes", "knowledge_urls", "research_focus", "reply_instructions"}},
		{model.BuiltinCareerOps, "career", true, []string{"job_url", "jd_text", "mode", "tailor_cv"}},
		{model.BuiltinArticleWriter, "articles", false, []string{"topic", "context", "model"}},
		{model.BuiltinImages, "images", false, []string{"prompt"}},
		{model.BuiltinResearcher, "", false, []string{"topic", "context"}},
	}
	got := agentschema.Builtins()
	if len(got) != len(want) {
		t.Fatalf("builtins = %v; want %d", got, len(want))
	}
	for i, w := range want {
		if got[i] != w.builtin {
			t.Errorf("order[%d] = %q; want %q", i, got[i], w.builtin)
		}
		m, ok := agentmodule.Lookup(w.builtin)
		if !ok {
			t.Fatalf("missing module %s", w.builtin)
		}
		if m.TabSlug != w.tab {
			t.Errorf("%s tab_slug = %q; want %q", w.builtin, m.TabSlug, w.tab)
		}
		if m.StayOnTab != w.stay {
			t.Errorf("%s stay_on_tab = %v; want %v", w.builtin, m.StayOnTab, w.stay)
		}
		s := agentschema.ForBuiltin(w.builtin)
		if len(s.Fields) != len(w.keys) {
			t.Fatalf("%s fields = %d; want %v", w.builtin, len(s.Fields), w.keys)
		}
		for j, k := range w.keys {
			if s.Fields[j].Key != k {
				t.Errorf("%s field[%d] = %q; want %q", w.builtin, j, s.Fields[j].Key, k)
			}
		}
	}
}
