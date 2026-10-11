package agentpack

import (
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func writerPack(name string) *Package {
	return &Package{
		Kind:          Kind,
		SchemaVersion: SchemaVersion,
		Agent: Body{
			Name:    name,
			Role:    "Content Writer",
			Builtin: model.BuiltinArticleWriter,
			EngineConfig: map[string]any{
				"writer_style":       "Business and tech, with tips.",
				"writer_focus":       []any{"Agentic AI", "FinTech"},
				"writer_destination": "Workstation Website",
			},
		},
		Tools: []string{"web_search"},
	}
}

// Importing a clone must never overwrite the org's original Article Writer.
func TestEvaluateWriterCloneIsCreatedNotOverlaid(t *testing.T) {
	original := &model.Agent{ID: uuid.New(), Name: "Article Writer"}
	rep := Evaluate(writerPack("Workstation Blog Writer"), Dest{
		ModuleOK: true, ModelOK: true, ExistingBuiltin: original,
	})
	if rep.Mode != ModeCreate {
		t.Fatalf("mode = %s, want create; issues %+v", rep.Mode, rep.Issues)
	}
	if rep.TargetAgentID != nil {
		t.Fatalf("a new clone must not target %s", *rep.TargetAgentID)
	}
	if rep.Bindings.Name != "Workstation Blog Writer" {
		t.Errorf("name = %q", rep.Bindings.Name)
	}
}

// Re-importing a clone updates that clone.
func TestEvaluateWriterCloneOverlaysSameName(t *testing.T) {
	original := &model.Agent{ID: uuid.New(), Name: "Article Writer"}
	clone := &model.Agent{ID: uuid.New(), Name: "Workstation Blog Writer"}
	rep := Evaluate(writerPack("Workstation Blog Writer"), Dest{
		ModuleOK: true, ModelOK: true, ExistingBuiltin: original, ExistingNamed: clone,
	})
	if rep.Mode != ModeOverlay || rep.TargetAgentID == nil || *rep.TargetAgentID != clone.ID {
		t.Fatalf("mode %s target %v, want overlay of the clone %s", rep.Mode, rep.TargetAgentID, clone.ID)
	}
}

// Writer settings travel; they hold names, never keys.
func TestSanitizeKeepsWriterSettings(t *testing.T) {
	out := SanitizeEngineConfig(writerPack("x").Agent.EngineConfig)
	for _, k := range []string{"writer_style", "writer_focus", "writer_destination"} {
		if _, ok := out[k]; !ok {
			t.Errorf("%s was dropped", k)
		}
	}
}
