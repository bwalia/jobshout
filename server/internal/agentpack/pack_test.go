package agentpack

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func TestSanitizeStripsSecretKeysKeepsStructuredModel(t *testing.T) {
	in := map[string]any{
		"structured_model": "qwen3-coder:30b",
		"webhook_url":      "https://evil.example",
		"private_key":      "-----BEGIN",
		"graph_definition": map[string]any{
			"nodes": []any{
				map[string]any{"id": "a", "openai_api_key": "sk-secret", "keep": "yes"},
			},
		},
		"openai_api_key": "sk-secret",
		"nested": map[string]any{
			"refresh_token": "abc",
			"keep":          "yes",
		},
	}
	out := SanitizeEngineConfig(in)
	if out["structured_model"] != "qwen3-coder:30b" {
		t.Fatalf("structured_model: %#v", out["structured_model"])
	}
	if _, ok := out["webhook_url"]; ok {
		t.Fatal("non-allowlisted keys must be dropped")
	}
	if _, ok := out["private_key"]; ok {
		t.Fatal("private_key must be stripped")
	}
	if _, ok := out["openai_api_key"]; ok {
		t.Fatal("api_key must be stripped")
	}
	if _, ok := out["nested"]; ok {
		t.Fatal("non-allowlisted nested object must be dropped")
	}
	graph, _ := out["graph_definition"].(map[string]any)
	nodes, _ := graph["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("nodes %#v", nodes)
	}
	node, _ := nodes[0].(map[string]any)
	if node["keep"] != "yes" || node["id"] != "a" {
		t.Fatalf("graph node: %#v", node)
	}
	if _, ok := node["openai_api_key"]; ok {
		t.Fatal("nested array secret must be stripped")
	}
}

func TestSafeFilenameIsSingleSegment(t *testing.T) {
	if got := SafeFilename(`..\..\etc\passwd`); got != "passwd" {
		t.Fatalf("got %q", got)
	}
	if got := SafeFilename("  "); got != "file" {
		t.Fatalf("empty %q", got)
	}
}

func TestPackOmitsIdentityAndSecrets(t *testing.T) {
	org := uuid.New()
	desc := "A helper"
	prompt := "You write tests."
	provider := "ollama"
	name := "llama3"
	score := 42.0
	agent := &model.Agent{
		ID:               uuid.New(),
		OrgID:            org,
		Name:             "Research Helper",
		Role:             "Researcher",
		Description:      &desc,
		SystemPrompt:     &prompt,
		ModelProvider:    &provider,
		ModelName:        &name,
		Status:           "active",
		PerformanceScore: score,
		EngineType:       model.EngineGoNative,
		EngineConfig: map[string]any{
			"structured_model": "qwen",
			"api_key":          "nope",
		},
		CreatedBy: &org,
	}
	pkg, err := Pack(Input{
		Agent: agent,
		Tools: []string{"http_request", "shell_command"},
		Skills: []Skill{{
			Slug: "cite-sources", Origin: "builtin",
		}},
		Knowledge: []File{{Filename: "style.md", Content: "# voice"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Kind != Kind || pkg.SchemaVersion != SchemaVersion {
		t.Fatalf("header: %+v", pkg)
	}
	if pkg.Agent.Name != "Research Helper" || pkg.Agent.SystemPrompt != prompt {
		t.Fatalf("body: %+v", pkg.Agent)
	}
	if pkg.Agent.EngineConfig["api_key"] != nil {
		t.Fatal("api_key leaked into package")
	}
	if pkg.Agent.EngineConfig["structured_model"] != "qwen" {
		t.Fatalf("engine_config: %#v", pkg.Agent.EngineConfig)
	}
	if strings.Contains(mustJSON(t, pkg), org.String()) && strings.Contains(mustJSON(t, pkg), `"org_id"`) {
		t.Fatal("org_id must not be packed")
	}
	raw := mustJSON(t, pkg)
	if strings.Contains(raw, `"org_id"`) || strings.Contains(raw, `"created_by"`) || strings.Contains(raw, `"performance_score"`) {
		t.Fatalf("identity leaked: %s", raw)
	}
	if len(pkg.Warnings) == 0 {
		t.Fatal("expected credential warning")
	}
}

func TestEvaluateCustomNameClashAndGatedTool(t *testing.T) {
	pkg := &Package{
		Kind: Kind, SchemaVersion: 1,
		Agent: Body{Name: "Helper", Role: "Tester", ModelProvider: "missing", ModelName: "nope"},
		Tools: []string{"http_request", "shell_command", "not_a_tool"},
	}
	rep := Evaluate(pkg, Dest{
		NameTaken: true,
		ToolNames: map[string]bool{"http_request": true, "shell_command": true},
		ModelOK:   false,
	})
	if rep.Mode != ModeCreate {
		t.Fatalf("mode %s", rep.Mode)
	}
	if rep.Bindings.Name != "Helper (imported)" {
		t.Fatalf("name %q", rep.Bindings.Name)
	}
	if !rep.HasError() && issueCode(rep, "specialist_missing") {
		t.Fatal("custom agent must not require a specialist")
	}
	if !issueCode(rep, "gated_tool") || !issueCode(rep, "unknown_tool") || !issueCode(rep, "model_unavailable") {
		t.Fatalf("issues: %+v", rep.Issues)
	}
	got := EffectiveTools(pkg, rep.Bindings, map[string]bool{"http_request": true, "shell_command": true})
	if len(got) != 1 || got[0] != "http_request" {
		t.Fatalf("tools %#v", got)
	}
	opted := EffectiveTools(pkg, Bindings{IncludeGatedTools: true, SkipTools: rep.Bindings.SkipTools}, map[string]bool{"http_request": true, "shell_command": true})
	if len(opted) != 2 {
		t.Fatalf("opt-in gated tools %#v", opted)
	}
}

func TestEvaluateBuiltinMissingIsError(t *testing.T) {
	pkg := &Package{
		Kind: Kind, SchemaVersion: 1,
		Agent: Body{Name: "Mail Agent", Role: "Mail", Builtin: "mail"},
	}
	rep := Evaluate(pkg, Dest{ModuleOK: false})
	if !rep.HasError() || !issueCode(rep, "specialist_missing") {
		t.Fatalf("want specialist_missing, got %+v", rep.Issues)
	}
}

func TestEvaluateBuiltinNotSeededIsError(t *testing.T) {
	pkg := &Package{
		Kind: Kind, SchemaVersion: 1,
		Agent: Body{Name: "Mail Agent", Role: "Mail", Builtin: "mail"},
	}
	rep := Evaluate(pkg, Dest{ModuleOK: true, ExistingBuiltin: nil})
	if !rep.HasError() || !issueCode(rep, "builtin_not_seeded") {
		t.Fatalf("want builtin_not_seeded, got %+v", rep.Issues)
	}
}

func TestEvaluateBuiltinOverlay(t *testing.T) {
	id := uuid.New()
	prompt := "old"
	pkg := &Package{
		Kind: Kind, SchemaVersion: 1,
		Agent: Body{Name: "Mail Agent", Role: "Mail", Builtin: "mail", SystemPrompt: "new"},
		Tools: []string{"http_request"},
	}
	rep := Evaluate(pkg, Dest{
		ModuleOK:        true,
		ExistingBuiltin: &model.Agent{ID: id, Name: "Mail Agent", SystemPrompt: &prompt},
		ExistingTools:   []string{"http_request", "web_search"},
		ToolNames:       map[string]bool{"http_request": true},
		ModelOK:         true,
	})
	if rep.Mode != ModeOverlay || rep.CanUndo {
		t.Fatalf("report %+v", rep)
	}
	if rep.TargetAgentID == nil || *rep.TargetAgentID != id {
		t.Fatalf("target %+v", rep.TargetAgentID)
	}
	if rep.Diff == nil || !rep.Diff.PromptChanged {
		t.Fatalf("diff %+v", rep.Diff)
	}
}

func TestFilenameSlug(t *testing.T) {
	got := FilenameSlug("Research Helper!", parseDay(t))
	if !strings.HasPrefix(got, "research-helper-") || !strings.HasSuffix(got, ".jobshout-agent.json") {
		t.Fatalf("got %q", got)
	}
}

func TestValidateRejectsUnknownKind(t *testing.T) {
	err := ValidateKind(&Package{Kind: "nope", SchemaVersion: 1, Agent: Body{Name: "A", Role: "B"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateRejectsTooNewSchema(t *testing.T) {
	err := ValidateKind(&Package{Kind: Kind, SchemaVersion: SchemaVersion + 1, Agent: Body{Name: "A", Role: "B"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCheckSizeRejectsTooManyKnowledgeFiles(t *testing.T) {
	pkg := &Package{Kind: Kind, SchemaVersion: 1, Agent: Body{Name: "A", Role: "B"}}
	pkg.Knowledge = make([]File, MaxKnowledgeFiles+1)
	for i := range pkg.Knowledge {
		pkg.Knowledge[i] = File{Filename: "f.md", Content: "x"}
	}
	if err := CheckSize(pkg); err == nil {
		t.Fatal("expected too many files")
	}
}

func TestCheckSizeCountsEngineConfig(t *testing.T) {
	pkg := &Package{
		Kind: Kind, SchemaVersion: 1,
		Agent: Body{
			Name: "A", Role: "B",
			EngineConfig: map[string]any{"graph_definition": strings.Repeat("x", MaxJSONBytes)},
		},
	}
	if err := CheckSize(pkg); err == nil {
		t.Fatal("expected oversize engine_config to fail")
	}
}

func TestSanitizeTruncatesPromptOnRuneBoundary(t *testing.T) {
	pkg := &Package{Agent: Body{SystemPrompt: strings.Repeat("é", MaxSystemPrompt)}}
	SanitizePackage(pkg)
	if len(pkg.Agent.SystemPrompt) > MaxSystemPrompt {
		t.Fatalf("len %d", len(pkg.Agent.SystemPrompt))
	}
	if !utf8.ValidString(pkg.Agent.SystemPrompt) {
		t.Fatal("truncated mid-rune")
	}
}

func TestHeaderSafeStripsNewlines(t *testing.T) {
	if got := HeaderSafe("a\r\nb"); got != "ab" {
		t.Fatalf("got %q", got)
	}
}

func TestEvaluateEmptyToolsWarns(t *testing.T) {
	pkg := &Package{Kind: Kind, SchemaVersion: 1, Agent: Body{Name: "Helper", Role: "Tester"}}
	rep := Evaluate(pkg, Dest{ModelOK: true, ToolNames: map[string]bool{}})
	if !issueCode(rep, "empty_tools") {
		t.Fatalf("issues %+v", rep.Issues)
	}
}

func issueCode(r Report, code string) bool {
	for _, i := range r.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func mustJSON(t *testing.T, pkg *Package) string {
	t.Helper()
	b, err := jsonForTest(pkg)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
