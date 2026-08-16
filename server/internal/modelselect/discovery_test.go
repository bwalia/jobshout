package modelselect

import (
	"testing"

	"github.com/jobshout/server/internal/llm"
)

func TestParseParameterSize(t *testing.T) {
	tests := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"30.5B", 30.5, true},
		{"8.0B", 8.0, true},
		{"7.6B", 7.6, true},
		{"23M", 0.023, true},
		{"27.9B", 27.9, true},
		{"", 0, false},
		{"huge", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseParameterSize(tt.in)
		if ok != tt.ok {
			t.Errorf("parseParameterSize(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			continue
		}
		if ok && (got < tt.want-0.001 || got > tt.want+0.001) {
			t.Errorf("parseParameterSize(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// The 8-15B band is calibrated so llama3, the one local model that was already
// hand-written into the catalog, keeps the exact quality it always had.
func TestHeuristicKeepsLlama3AtItsHistoricQuality(t *testing.T) {
	q, _ := qualitySpeedFor(llm.ModelInfo{
		Provider: "ollama", Name: "llama3:latest", ParameterSize: "8.0B",
	})
	if q != 4 {
		t.Errorf("llama3 quality = %d, want 4 to match the previous hardcoded entry", q)
	}
}

func TestHeuristicCapsLocalQuality(t *testing.T) {
	// A 405B local model must still not be trusted with planning: the cap
	// exists because parameter count is a weak proxy for reasoning.
	q, _ := qualitySpeedFor(llm.ModelInfo{
		Provider: "ollama", Name: "giant:405b", ParameterSize: "405B",
	})
	if q > maxLocalQuality {
		t.Errorf("quality = %d, want it capped at %d", q, maxLocalQuality)
	}
	if q >= minQualityFor(KindPlan) {
		t.Errorf("a size-scored local model (%d) must not clear the planning bar (%d)",
			q, minQualityFor(KindPlan))
	}
}

func TestHeuristicAdjustsSpeedForMixtureOfExperts(t *testing.T) {
	dense := llm.ModelInfo{Provider: "ollama", Name: "dense:30b", ParameterSize: "30.5B", Family: "llama"}
	moeByFamily := llm.ModelInfo{Provider: "ollama", Name: "qwen3:30b", ParameterSize: "30.5B", Family: "qwen3moe"}
	moeByName := llm.ModelInfo{Provider: "ollama", Name: "qwen3:30b-a3b", ParameterSize: "30.5B", Family: "llama"}

	_, denseSpeed := qualitySpeedFor(dense)
	_, famSpeed := qualitySpeedFor(moeByFamily)
	_, nameSpeed := qualitySpeedFor(moeByName)

	if famSpeed <= denseSpeed {
		t.Errorf("MoE (by family) speed %d should exceed dense %d", famSpeed, denseSpeed)
	}
	if nameSpeed <= denseSpeed {
		t.Errorf("MoE (by -a3b name) speed %d should exceed dense %d", nameSpeed, denseSpeed)
	}
}

func TestHeuristicUsesKnownValuesForCloudModels(t *testing.T) {
	q, s := qualitySpeedFor(llm.ModelInfo{Provider: "openai", Name: "gpt-4o"})
	if q != 8 || s != 6 {
		t.Errorf("gpt-4o = (%d,%d), want (8,6) from the known table", q, s)
	}
	// A cloud model must not be subject to the local-only quality cap.
	q, _ = qualitySpeedFor(llm.ModelInfo{Provider: "claude", Name: "claude-opus-4-20250514"})
	if q != 10 {
		t.Errorf("opus quality = %d, want 10", q)
	}
}

func TestFromModelInfoSkipsEmbeddingOnly(t *testing.T) {
	_, ok := FromModelInfo(llm.ModelInfo{
		Provider: "ollama", Name: "all-minilm:latest",
		Capabilities: []string{llm.CapEmbedding},
	}, 8192)
	if ok {
		t.Error("an embedding-only model must never become a candidate")
	}
}

// The ceiling is what keeps the selector's belief equal to what the client will
// actually request via num_ctx.
func TestFromModelInfoAppliesNumCtxCeiling(t *testing.T) {
	c, ok := FromModelInfo(llm.ModelInfo{
		Provider: "ollama", Name: "qwen3-coder:30b", ParameterSize: "30.5B",
		ContextTokens: 262_144, Capabilities: []string{llm.CapCompletion, llm.CapTools},
	}, 32_768)
	if !ok {
		t.Fatal("expected a candidate")
	}
	if c.ContextTokens != 32_768 {
		t.Errorf("ContextTokens = %d, want it capped to the num_ctx ceiling 32768", c.ContextTokens)
	}
	if !c.SupportsTools {
		t.Error("tool capability should carry through from discovery")
	}

	// Cloud models are not subject to the local ceiling.
	c, _ = FromModelInfo(llm.ModelInfo{
		Provider: "claude", Name: "claude-sonnet-4-20250514", ContextTokens: 200_000,
	}, 8192)
	if c.ContextTokens != 200_000 {
		t.Errorf("cloud ContextTokens = %d, want 200000 (no local ceiling)", c.ContextTokens)
	}
}

func TestFromModelInfoDefaultsUnknownContextConservatively(t *testing.T) {
	c, ok := FromModelInfo(llm.ModelInfo{
		Provider: "ollama", Name: "mystery:8b", ParameterSize: "8.0B",
		Capabilities: []string{llm.CapCompletion},
	}, 131_072)
	if !ok {
		t.Fatal("expected a candidate")
	}
	if c.ContextTokens != 4096 {
		t.Errorf("ContextTokens = %d, want a conservative 4096 when unknown", c.ContextTokens)
	}
}

// A provider that reports live models replaces its static entries entirely —
// otherwise uninstalled models would be resurrected from the old catalog.
func TestMergeCatalogsLetsDiscoveryReplaceItsProvider(t *testing.T) {
	static := []Candidate{
		{Provider: "ollama", Model: "llama3:latest", Quality: 4},
		{Provider: "openai", Model: "gpt-4o", Quality: 8},
	}
	dynamic := []Candidate{
		{Provider: "ollama", Model: "qwen3-coder:30b", Quality: 6},
	}

	got := mergeCatalogs(static, dynamic)

	var sawUninstalled, sawDiscovered, sawOpenAI bool
	for _, c := range got {
		switch {
		case c.Provider == "ollama" && c.Model == "llama3:latest":
			sawUninstalled = true
		case c.Provider == "ollama" && c.Model == "qwen3-coder:30b":
			sawDiscovered = true
		case c.Provider == "openai":
			sawOpenAI = true
		}
	}
	if sawUninstalled {
		t.Error("a static ollama model must not survive once discovery has spoken")
	}
	if !sawDiscovered {
		t.Error("the discovered model should be present")
	}
	if !sawOpenAI {
		t.Error("a provider with no discovery must keep its static entries")
	}
}

func TestMergeCatalogsWithNoDiscoveryIsTheStaticCatalog(t *testing.T) {
	static := DefaultCatalog()
	if got := mergeCatalogs(static, nil); len(got) != len(static) {
		t.Errorf("merge with empty discovery changed the catalog: %d vs %d", len(got), len(static))
	}
}

// End to end: with the real local models discovered, Auto must reach a good one
// rather than falling through to a paid provider.
func TestLiveCatalogLetsAutoReachLocalModels(t *testing.T) {
	src := fakeModelSource{models: []llm.ProviderModels{{
		Provider: "ollama",
		Models: []llm.ModelInfo{
			{Provider: "ollama", Name: "llama3:latest", ParameterSize: "8.0B", ContextTokens: 8192,
				Capabilities: []string{llm.CapCompletion}},
			{Provider: "ollama", Name: "qwen3-coder:30b", ParameterSize: "30.5B", ContextTokens: 262_144,
				Family: "qwen3moe", Capabilities: []string{llm.CapCompletion, llm.CapTools}},
			{Provider: "ollama", Name: "all-minilm:latest", ParameterSize: "23M",
				Capabilities: []string{llm.CapEmbedding}},
		},
	}}}

	s := newSelector(allProviders()).WithDynamicCatalog(LiveCatalog(src, 131_072))

	// A 20k-token prompt: llama3's 8k window cannot hold it, but the 30B can.
	got, err := s.Select(TaskSignals{Kind: KindChat, PromptTokens: 20_000}, Constraints{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.Provider != "ollama" || got.Model != "qwen3-coder:30b" {
		t.Errorf("Select() = %s/%s, want ollama/qwen3-coder:30b", got.Provider, got.Model)
	}

	// The embedding model must never be selectable.
	for _, c := range append(got.Fallbacks, got.Chosen) {
		if c.Model == "all-minilm:latest" {
			t.Error("embedding-only model leaked into the candidate set")
		}
	}
}

type fakeModelSource struct{ models []llm.ProviderModels }

func (f fakeModelSource) CachedModels() []llm.ProviderModels { return f.models }
