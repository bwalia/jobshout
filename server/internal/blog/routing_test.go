package blog

import "testing"

func routingRunner(cfg Config) *Runner {
	return NewRunner(cfg, &stubLLM{}, nil, &fakeResearcher{}, testLogger())
}

// Unset prose/structured settings must behave exactly as before the split.
func TestModelRoutingDefaultsToTheWritingModel(t *testing.T) {
	r := routingRunner(Config{Model: "writer"})

	if got := r.proseModel(GenerateRequest{}); got != "writer" {
		t.Errorf("proseModel = %q, want %q", got, "writer")
	}
	if got := r.structuredModel(GenerateRequest{}); got != "writer" {
		t.Errorf("structuredModel = %q, want %q", got, "writer")
	}
}

func TestModelRoutingUsesEachSettingWhenSet(t *testing.T) {
	r := routingRunner(Config{Model: "writer", ProseModel: "poet", StructuredModel: "clerk"})

	if got := r.proseModel(GenerateRequest{}); got != "poet" {
		t.Errorf("proseModel = %q, want %q", got, "poet")
	}
	if got := r.structuredModel(GenerateRequest{}); got != "clerk" {
		t.Errorf("structuredModel = %q, want %q", got, "clerk")
	}
}

// Setting only one of the pair must leave the other on the writing model.
func TestModelRoutingFallsBackPerSetting(t *testing.T) {
	r := routingRunner(Config{Model: "writer", ProseModel: "poet"})

	if got := r.proseModel(GenerateRequest{}); got != "poet" {
		t.Errorf("proseModel = %q, want %q", got, "poet")
	}
	if got := r.structuredModel(GenerateRequest{}); got != "writer" {
		t.Errorf("structuredModel = %q, want %q", got, "writer")
	}
}

// The whole point of the agent fields: a model chosen in the UI has to reach
// the pipeline. Before this, the picker saved a value nothing ever read.
func TestModelRoutingUsesTheAgentsConfiguredModels(t *testing.T) {
	r := routingRunner(Config{Model: "writer"})
	req := GenerateRequest{AgentProseModel: "agent-poet", AgentStructuredModel: "agent-clerk"}

	if got := r.proseModel(req); got != "agent-poet" {
		t.Errorf("proseModel = %q, want %q", got, "agent-poet")
	}
	if got := r.structuredModel(req); got != "agent-clerk" {
		t.Errorf("structuredModel = %q, want %q", got, "agent-clerk")
	}
}

// The agent's standing choice is more specific than how the server was started.
func TestModelRoutingAgentBeatsEnvironmentSettings(t *testing.T) {
	r := routingRunner(Config{Model: "writer", ProseModel: "env-poet", StructuredModel: "env-clerk"})
	req := GenerateRequest{AgentProseModel: "agent-poet", AgentStructuredModel: "agent-clerk"}

	if got := r.proseModel(req); got != "agent-poet" {
		t.Errorf("proseModel = %q, want %q", got, "agent-poet")
	}
	if got := r.structuredModel(req); got != "agent-clerk" {
		t.Errorf("structuredModel = %q, want %q", got, "agent-clerk")
	}
}

// An agent that sets only one of the two leaves the other to fall through.
func TestModelRoutingAgentCanSetJustOne(t *testing.T) {
	r := routingRunner(Config{Model: "writer", StructuredModel: "env-clerk"})
	req := GenerateRequest{AgentProseModel: "agent-poet"}

	if got := r.proseModel(req); got != "agent-poet" {
		t.Errorf("proseModel = %q, want %q", got, "agent-poet")
	}
	if got := r.structuredModel(req); got != "env-clerk" {
		t.Errorf("structuredModel = %q, want %q", got, "env-clerk")
	}
}

// Someone naming a model for this run is asking for that model, not for a
// policy — it beats the agent's setting and the environment's.
func TestModelRoutingRunOverrideBeatsEverything(t *testing.T) {
	r := routingRunner(Config{Model: "writer", ProseModel: "env-poet", StructuredModel: "env-clerk"})
	req := GenerateRequest{
		Model:                "chosen",
		AgentProseModel:      "agent-poet",
		AgentStructuredModel: "agent-clerk",
	}

	if got := r.proseModel(req); got != "chosen" {
		t.Errorf("proseModel = %q, want %q", got, "chosen")
	}
	if got := r.structuredModel(req); got != "chosen" {
		t.Errorf("structuredModel = %q, want %q", got, "chosen")
	}
}

// Whitespace is not a model name, at any level.
func TestModelRoutingIgnoresBlankValues(t *testing.T) {
	r := routingRunner(Config{Model: "writer", ProseModel: "   ", StructuredModel: "\t"})
	req := GenerateRequest{Model: "  ", AgentProseModel: "\n", AgentStructuredModel: " "}

	if got := r.proseModel(req); got != "writer" {
		t.Errorf("proseModel = %q, want %q", got, "writer")
	}
	if got := r.structuredModel(req); got != "writer" {
		t.Errorf("structuredModel = %q, want %q", got, "writer")
	}
}

// An agent set to another provider must not be sent the environment's models:
// BLOG_PROSE_MODEL names an Ollama tag that Gemini would reject on every call.
func TestModelRoutingOtherProviderSkipsEnvironmentModels(t *testing.T) {
	r := routingRunner(Config{Provider: "ollama", Model: "qwen3:8b", ProseModel: "qwen3.8:latest", StructuredModel: "qwen3:8b"})

	req := GenerateRequest{AgentProvider: "gemini"}
	if got := r.proseModel(req); got != "" {
		t.Errorf("proseModel = %q, want the provider default (empty)", got)
	}
	if got := r.structuredModel(req); got != "" {
		t.Errorf("structuredModel = %q, want the provider default (empty)", got)
	}

	req = GenerateRequest{AgentProvider: "gemini", AgentProseModel: "gemini-2.5-pro"}
	if got := r.proseModel(req); got != "gemini-2.5-pro" {
		t.Errorf("proseModel = %q, want the agent's model", got)
	}
}

// Naming the server's own provider (or "auto") on the agent changes nothing.
func TestModelRoutingSameProviderKeepsEnvironmentModels(t *testing.T) {
	r := routingRunner(Config{Provider: "ollama", Model: "writer"})
	for _, p := range []string{"", "ollama", "OLLAMA", "auto"} {
		if got := r.proseModel(GenerateRequest{AgentProvider: p}); got != "writer" {
			t.Errorf("AgentProvider %q: proseModel = %q, want %q", p, got, "writer")
		}
	}
}
