package metalead

import (
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func TestModuleSchemaRequiresConnection(t *testing.T) {
	m := Module(nil)
	if m.Builtin != model.BuiltinMetaLead {
		t.Fatalf("builtin = %q", m.Builtin)
	}
	if m.TabSlug != TabSlug {
		t.Fatalf("tab = %q", m.TabSlug)
	}
	found := false
	for _, f := range m.Schema.Fields {
		if f.Key == "connection" && f.Required {
			found = true
		}
	}
	if !found {
		t.Fatal("connection field required")
	}
	seed := m.Seed(uuid.New())
	if seed.Metadata[model.MetadataKeyBuiltin] != model.BuiltinMetaLead {
		t.Fatalf("seed metadata = %v", seed.Metadata)
	}
}
