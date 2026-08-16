package blog

import "testing"

func routingRunner(cfg Config) *Runner {
	return NewRunner(cfg, &stubLLM{}, nil, &fakeResearcher{}, testLogger())
}

// Unset prose/structured models must behave exactly as before the split.
func TestModelRoutingDefaultsToTheWritingModel(t *testing.T) {
	r := routingRunner(Config{Model: "writer"})

	if got := r.proseModel(""); got != "writer" {
		t.Errorf("proseModel = %q, want %q", got, "writer")
	}
	if got := r.structuredModel(""); got != "writer" {
		t.Errorf("structuredModel = %q, want %q", got, "writer")
	}
}

func TestModelRoutingUsesEachSettingWhenSet(t *testing.T) {
	r := routingRunner(Config{Model: "writer", ProseModel: "poet", StructuredModel: "clerk"})

	if got := r.proseModel(""); got != "poet" {
		t.Errorf("proseModel = %q, want %q", got, "poet")
	}
	if got := r.structuredModel(""); got != "clerk" {
		t.Errorf("structuredModel = %q, want %q", got, "clerk")
	}
}

// Setting only one of the pair must leave the other on the writing model.
func TestModelRoutingFallsBackPerSetting(t *testing.T) {
	r := routingRunner(Config{Model: "writer", ProseModel: "poet"})

	if got := r.proseModel(""); got != "poet" {
		t.Errorf("proseModel = %q, want %q", got, "poet")
	}
	if got := r.structuredModel(""); got != "writer" {
		t.Errorf("structuredModel = %q, want %q", got, "writer")
	}
}

// Someone naming a model in the request is asking for that model, not for a
// routing policy — the override has to beat both settings.
func TestModelRoutingRequestOverrideBeatsBothSettings(t *testing.T) {
	r := routingRunner(Config{Model: "writer", ProseModel: "poet", StructuredModel: "clerk"})

	if got := r.proseModel("chosen"); got != "chosen" {
		t.Errorf("proseModel = %q, want %q", got, "chosen")
	}
	if got := r.structuredModel("chosen"); got != "chosen" {
		t.Errorf("structuredModel = %q, want %q", got, "chosen")
	}
}

// Whitespace is not a model name.
func TestModelRoutingIgnoresBlankValues(t *testing.T) {
	r := routingRunner(Config{Model: "writer", ProseModel: "   ", StructuredModel: "\t"})

	if got := r.proseModel("  "); got != "writer" {
		t.Errorf("proseModel = %q, want %q", got, "writer")
	}
	if got := r.structuredModel("  "); got != "writer" {
		t.Errorf("structuredModel = %q, want %q", got, "writer")
	}
}
