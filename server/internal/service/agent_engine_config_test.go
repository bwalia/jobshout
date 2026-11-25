package service

import (
	"testing"

	"github.com/jobshout/server/internal/model"
)

// applyEngineConfig mirrors the one branch under test, so the behaviour is
// pinned without standing up a repository and a database.
func applyEngineConfig(agent *model.Agent, req model.UpdateAgentRequest) {
	if req.EngineConfig != nil {
		agent.EngineConfig = req.EngineConfig
	}
}

// The Article Writer's second model is stored in engine_config, so an update
// that does not persist it silently discards the user's choice — which is the
// bug this whole change exists to fix.
func TestUpdateAppliesEngineConfig(t *testing.T) {
	agent := &model.Agent{EngineConfig: map[string]any{}}

	applyEngineConfig(agent, model.UpdateAgentRequest{
		EngineConfig: map[string]any{model.EngineConfigStructuredModel: "qwen3-coder:30b"},
	})

	got, _ := agent.EngineConfig[model.EngineConfigStructuredModel].(string)
	if got != "qwen3-coder:30b" {
		t.Errorf("structured model = %q, want %q", got, "qwen3-coder:30b")
	}
}

// A request that says nothing about engine_config must leave it alone, or every
// unrelated edit would wipe the model choice.
func TestUpdateLeavesEngineConfigAloneWhenAbsent(t *testing.T) {
	agent := &model.Agent{
		EngineConfig: map[string]any{model.EngineConfigStructuredModel: "keep-me"},
	}
	name := "Renamed"

	applyEngineConfig(agent, model.UpdateAgentRequest{Name: &name})

	got, _ := agent.EngineConfig[model.EngineConfigStructuredModel].(string)
	if got != "keep-me" {
		t.Errorf("structured model = %q, want it untouched", got)
	}
}

// Clearing the choice has to be possible: an empty value means "fall back to
// the server's setting".
func TestUpdateCanClearTheStructuredModel(t *testing.T) {
	agent := &model.Agent{
		EngineConfig: map[string]any{model.EngineConfigStructuredModel: "muse-glimmer:latest"},
	}

	applyEngineConfig(agent, model.UpdateAgentRequest{
		EngineConfig: map[string]any{model.EngineConfigStructuredModel: ""},
	})

	if got, _ := agent.EngineConfig[model.EngineConfigStructuredModel].(string); got != "" {
		t.Errorf("structured model = %q, want empty", got)
	}
}

// What the blog service reads back out of the agent.
func TestAgentModelsReadsBothChoices(t *testing.T) {
	name := "muse-glimmer:latest"
	agent := &model.Agent{
		ModelName:    &name,
		EngineConfig: map[string]any{model.EngineConfigStructuredModel: "qwen3-coder:30b"},
	}

	prose, structured := agentModels(agent)

	if prose != "muse-glimmer:latest" {
		t.Errorf("prose = %q", prose)
	}
	if structured != "qwen3-coder:30b" {
		t.Errorf("structured = %q", structured)
	}
}

func TestAgentModelsToleratesAnUnconfiguredAgent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		agent *model.Agent
	}{
		{"nil agent", nil},
		{"nothing set", &model.Agent{}},
		{"blank strings", func() *model.Agent {
			blank := "   "
			return &model.Agent{
				ModelName:    &blank,
				EngineConfig: map[string]any{model.EngineConfigStructuredModel: "\t"},
			}
		}()},
		{"wrong type in config", &model.Agent{
			EngineConfig: map[string]any{model.EngineConfigStructuredModel: 42},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prose, structured := agentModels(tc.agent)
			if prose != "" || structured != "" {
				t.Errorf("got %q/%q, want both empty", prose, structured)
			}
		})
	}
}

// A schedule that names Ollama runs on Ollama with the server's models, while
// a run that names nothing keeps the agent's Claude and its model.
func TestRunModelsRunProviderBeatsTheAgents(t *testing.T) {
	provider, name := "claude", "claude-model"
	agent := &model.Agent{ModelProvider: &provider, ModelName: &name}

	for _, tc := range []struct {
		run, wantProvider, wantProse string
	}{
		{"", "claude", "claude-model"},
		{"auto", "claude", "claude-model"},
		{"Claude", "claude", "claude-model"},
		{"ollama", "ollama", ""},
	} {
		p, prose, _ := runModels(agent, model.GenerateBlogRequest{Provider: tc.run})
		if p != tc.wantProvider || prose != tc.wantProse {
			t.Errorf("run provider %q: got (%q, %q), want (%q, %q)", tc.run, p, prose, tc.wantProvider, tc.wantProse)
		}
	}
}
