package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentpack"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/tools"
)

type fakePackStore struct {
	bundle       *repository.AgentBundle
	err          error
	taken        bool
	created      *model.Agent
	createdTools []string
	overlaid     *uuid.UUID
	overlayTools []string
}

func (f *fakePackStore) LoadBundle(context.Context, uuid.UUID, uuid.UUID) (*repository.AgentBundle, error) {
	return f.bundle, f.err
}
func (f *fakePackStore) NameTaken(context.Context, uuid.UUID, string, *uuid.UUID) (bool, error) {
	return f.taken, nil
}
func (f *fakePackStore) ApplyCreate(_ context.Context, orgID, userID uuid.UUID, pkg *agentpack.Package, name, provider, modelName string, tools []string) (*model.Agent, error) {
	a := &model.Agent{
		ID: uuid.New(), OrgID: orgID, Name: name, Role: pkg.Agent.Role,
		EngineType: pkg.Agent.EngineType, EngineConfig: pkg.Agent.EngineConfig, CreatedBy: &userID,
	}
	if pkg.Agent.SystemPrompt != "" {
		p := pkg.Agent.SystemPrompt
		a.SystemPrompt = &p
	}
	if provider != "" {
		a.ModelProvider = &provider
	}
	if modelName != "" {
		a.ModelName = &modelName
	}
	f.created = a
	f.createdTools = append([]string(nil), tools...)
	return a, nil
}
func (f *fakePackStore) ApplyOverlay(_ context.Context, _, agentID uuid.UUID, _ *agentpack.Package, provider, modelName string, tools []string) (*model.Agent, error) {
	id := agentID
	f.overlaid = &id
	f.overlayTools = append([]string(nil), tools...)
	a := &model.Agent{ID: agentID, Name: "Existing"}
	if provider != "" {
		a.ModelProvider = &provider
	}
	if modelName != "" {
		a.ModelName = &modelName
	}
	return a, nil
}
func (f *fakePackStore) ExecutionCount(context.Context, uuid.UUID) (int, error) { return 0, nil }
func (f *fakePackStore) KnowledgeFiles(context.Context, uuid.UUID) ([]repository.KnowledgeFileRef, error) {
	return nil, nil
}

func TestAgentPackExportStripsSecretsAndOrg(t *testing.T) {
	org := uuid.New()
	other := uuid.New()
	prompt := "Be helpful."
	key := "sk-live"
	agent := &model.Agent{
		ID: uuid.New(), OrgID: org, Name: "Helper", Role: "Tester",
		SystemPrompt: &prompt, EngineType: model.EngineGoNative,
		EngineConfig: map[string]any{"openai_api_key": key, "structured_model": "qwen"},
	}
	store := &fakePackStore{bundle: &repository.AgentBundle{Agent: agent, Tools: []string{"http_request"}}}
	reg := tools.NewRegistry()
	reg.Register(&stubTool{name: "http_request"})
	svc := NewAgentPackService(store, nil, nil, reg, nil, nil, nil, false, nil)

	pkg, filename, err := svc.Export(context.Background(), org, uuid.New(), agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Agent.EngineConfig["openai_api_key"] != nil {
		t.Fatal("secret leaked")
	}
	if pkg.Agent.EngineConfig["structured_model"] != "qwen" {
		t.Fatalf("keep structured_model: %#v", pkg.Agent.EngineConfig)
	}
	if !strings.Contains(filename, "helper") {
		t.Fatalf("filename %q", filename)
	}

	store.err = repository.ErrAgentPackForbidden
	if _, _, err := svc.Export(context.Background(), other, uuid.New(), agent.ID); err == nil {
		t.Fatal("expected forbidden for other org")
	}
}

func TestAgentPackImportCreatePersistsEngineConfig(t *testing.T) {
	org := uuid.New()
	user := uuid.New()
	store := &fakePackStore{}
	reg := tools.NewRegistry()
	reg.Register(&stubTool{name: "http_request"})
	reg.Register(&stubTool{name: "shell_command"})
	svc := NewAgentPackService(store, nil, nil, reg, nil, nil, nil, false, nil)
	pkg := &agentpack.Package{
		Kind: agentpack.Kind, SchemaVersion: 1,
		Agent: agentpack.Body{
			Name: "Importer", Role: "QA", EngineType: model.EngineGoNative,
			EngineConfig: map[string]any{"structured_model": "qwen", "api_key": "nope"},
		},
		Tools: []string{"http_request", "shell_command"},
	}
	out, err := svc.Import(context.Background(), org, user, ImportAgentRequest{Package: pkg})
	if err != nil {
		t.Fatal(err)
	}
	if out.Agent.Name != "Importer" || out.Mode != agentpack.ModeCreate {
		t.Fatalf("out %+v", out)
	}
	if out.Agent.EngineConfig["api_key"] != nil {
		t.Fatal("api_key persisted")
	}
	if out.Agent.EngineConfig["structured_model"] != "qwen" {
		t.Fatalf("engine %#v", out.Agent.EngineConfig)
	}
	if !out.CanUndo {
		t.Fatal("create should be undoable")
	}
	for _, n := range store.createdTools {
		if n == "shell_command" {
			t.Fatal("shell_command enabled without opt-in")
		}
	}
}

func TestAgentPackImportGatedToolOptIn(t *testing.T) {
	org := uuid.New()
	user := uuid.New()
	store := &fakePackStore{}
	reg := tools.NewRegistry()
	reg.Register(&stubTool{name: "http_request"})
	reg.Register(&stubTool{name: "shell_command"})
	svc := NewAgentPackService(store, nil, nil, reg, nil, nil, nil, false, nil)
	pkg := &agentpack.Package{
		Kind: agentpack.Kind, SchemaVersion: 1,
		Agent: agentpack.Body{Name: "Sheller", Role: "Ops"},
		Tools: []string{"http_request", "shell_command"},
	}
	if _, err := svc.Import(context.Background(), org, user, ImportAgentRequest{
		Package:  pkg,
		Bindings: agentpack.Bindings{IncludeGatedTools: true},
	}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range store.createdTools {
		if n == "shell_command" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected shell_command when opted in, got %#v", store.createdTools)
	}
}

func TestAgentPackImportUnknownSpecialistBlocked(t *testing.T) {
	org := uuid.New()
	user := uuid.New()
	store := &fakePackStore{}
	svc := NewAgentPackService(store, nil, nil, tools.NewRegistry(), nil, nil, nil, false, nil)
	pkg := &agentpack.Package{
		Kind: agentpack.Kind, SchemaVersion: 1,
		Agent: agentpack.Body{Name: "Ghost", Role: "Mail", Builtin: "not_a_real_specialist"},
	}
	if _, err := svc.Import(context.Background(), org, user, ImportAgentRequest{Package: pkg}); err == nil {
		t.Fatal("expected specialist missing")
	}
	if store.created != nil || store.overlaid != nil {
		t.Fatal("must not write when specialist is missing")
	}
}

func TestAgentPackImportOverlayDoesNotCreate(t *testing.T) {
	org := uuid.New()
	user := uuid.New()
	existingID := uuid.New()
	existing := &model.Agent{ID: existingID, OrgID: org, Name: "Pack Overlay"}
	agentmodule.Register(agentmodule.Module{
		Builtin: "pack_overlay_test",
		Label:   "Pack Overlay",
		Schema:  agentschema.Schema{Builtin: "pack_overlay_test"},
	})
	store := &fakePackStore{bundle: &repository.AgentBundle{Agent: existing, Tools: []string{}}}
	finder := &fakeBuiltinFinder{agent: existing}
	svc := NewAgentPackService(store, finder, nil, tools.NewRegistry(), nil, nil, nil, false, nil)
	pkg := &agentpack.Package{
		Kind: agentpack.Kind, SchemaVersion: 1,
		Agent: agentpack.Body{Name: "Pack Overlay", Role: "QA", Builtin: "pack_overlay_test", SystemPrompt: "new prompt"},
	}
	out, err := svc.Import(context.Background(), org, user, ImportAgentRequest{Package: pkg})
	if err != nil {
		t.Fatal(err)
	}
	if out.Mode != agentpack.ModeOverlay || out.CanUndo {
		t.Fatalf("out %+v", out)
	}
	if store.created != nil {
		t.Fatal("overlay must not insert")
	}
	if store.overlaid == nil || *store.overlaid != existingID {
		t.Fatalf("overlay target %+v", store.overlaid)
	}
}

type fakeBuiltinFinder struct{ agent *model.Agent }

func (f *fakeBuiltinFinder) FindBuiltin(context.Context, uuid.UUID, string) (*model.Agent, error) {
	return f.agent, nil
}

func TestAgentPackRoundTripNewIDNoSecrets(t *testing.T) {
	orgA := uuid.New()
	orgB := uuid.New()
	user := uuid.New()
	prompt := "Be precise."
	src := &model.Agent{
		ID: uuid.New(), OrgID: orgA, Name: "Roundtrip", Role: "QA",
		SystemPrompt: &prompt, EngineType: model.EngineGoNative,
		EngineConfig: map[string]any{"openai_api_key": "sk-live", "structured_model": "qwen"},
	}
	exportStore := &fakePackStore{bundle: &repository.AgentBundle{Agent: src, Tools: []string{"http_request"}}}
	reg := tools.NewRegistry()
	reg.Register(&stubTool{name: "http_request"})
	exportSvc := NewAgentPackService(exportStore, nil, nil, reg, nil, nil, nil, false, nil)
	pkg, _, err := exportSvc.Export(context.Background(), orgA, user, src.ID)
	if err != nil {
		t.Fatal(err)
	}

	importStore := &fakePackStore{}
	importSvc := NewAgentPackService(importStore, nil, nil, reg, nil, nil, nil, false, nil)
	out, err := importSvc.Import(context.Background(), orgB, user, ImportAgentRequest{Package: pkg})
	if err != nil {
		t.Fatal(err)
	}
	if out.Agent.ID == src.ID {
		t.Fatal("destination must get a new id")
	}
	if out.Agent.OrgID != orgB {
		t.Fatal("imported into wrong org")
	}
	if out.Agent.EngineConfig["openai_api_key"] != nil {
		t.Fatal("secret persisted on import")
	}
	if deref(out.Agent.SystemPrompt) != prompt {
		t.Fatalf("prompt %q", deref(out.Agent.SystemPrompt))
	}
}

func TestAgentPackOverlayKeepsDestModelAndTools(t *testing.T) {
	agentmodule.Register(agentmodule.Module{
		Builtin: "pack_overlay_test",
		Label:   "Pack Overlay",
		Schema:  agentschema.Schema{Builtin: "pack_overlay_test"},
	})
	org := uuid.New()
	user := uuid.New()
	existingID := uuid.New()
	destProvider := "ollama"
	destModel := "llama3"
	existing := &model.Agent{
		ID: existingID, OrgID: org, Name: "Pack Overlay",
		ModelProvider: &destProvider, ModelName: &destModel,
	}
	store := &fakePackStore{bundle: &repository.AgentBundle{
		Agent: existing, Tools: []string{"http_request"},
	}}
	finder := &fakeBuiltinFinder{agent: existing}
	svc := NewAgentPackService(store, finder, nil, tools.NewRegistry(), nil, nil, nil, false, nil)
	pkg := &agentpack.Package{
		Kind: agentpack.Kind, SchemaVersion: 1,
		Agent: agentpack.Body{
			Name: "Pack Overlay", Role: "QA", Builtin: "pack_overlay_test",
			ModelProvider: "missing", ModelName: "nope",
		},
		Tools: []string{"not_a_tool", "shell_command"},
	}
	prev, err := svc.Preview(context.Background(), org, pkg)
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.Import(context.Background(), org, user, ImportAgentRequest{
		PreviewID: prev.PreviewID,
		Bindings:  agentpack.Bindings{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if deref(out.Agent.ModelProvider) != "ollama" || deref(out.Agent.ModelName) != "llama3" {
		t.Fatalf("overlay wiped dest model: %+v %+v", out.Agent.ModelProvider, out.Agent.ModelName)
	}
	if len(store.overlayTools) != 1 || store.overlayTools[0] != "http_request" {
		t.Fatalf("overlay wiped dest tools: %#v", store.overlayTools)
	}
}

func TestAgentPackImportFallsBackToPackageWhenPreviewMissing(t *testing.T) {
	org := uuid.New()
	user := uuid.New()
	store := &fakePackStore{}
	reg := tools.NewRegistry()
	reg.Register(&stubTool{name: "http_request"})
	svc := NewAgentPackService(store, nil, nil, reg, nil, nil, nil, false, nil)
	pkg := &agentpack.Package{
		Kind: agentpack.Kind, SchemaVersion: 1,
		Agent: agentpack.Body{Name: "Fallback", Role: "QA"},
		Tools: []string{"http_request"},
	}
	out, err := svc.Import(context.Background(), org, user, ImportAgentRequest{
		PreviewID: uuid.New().String(),
		Package:   pkg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Agent.Name != "Fallback" {
		t.Fatalf("name %q", out.Agent.Name)
	}
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

type stubTool struct{ name string }

func (s *stubTool) Name() string                                            { return s.name }
func (s *stubTool) Description() string                                     { return s.name }
func (s *stubTool) Execute(context.Context, map[string]any) (string, error) { return "", nil }
