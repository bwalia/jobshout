package config

import "testing"

func TestAgentModel_Precedence(t *testing.T) {
	// Generic scheme wins over everything, for any agent.
	t.Setenv("AGENT_PROVIDER__CAREER_OPS", "gemini")
	t.Setenv("AGENT_MODEL__CAREER_OPS", "gemini-flash-latest")
	t.Setenv("CAREER_PROVIDER", "ollama")
	t.Setenv("CAREER_MODEL", "careerops:32k")
	if got := AgentModel("career_ops"); got.Provider != "gemini" || got.Model != "gemini-flash-latest" {
		t.Fatalf("generic should win, got %+v", got)
	}
}

func TestAgentModel_LegacyFallback(t *testing.T) {
	// With no generic var, the dedicated legacy var still applies.
	t.Setenv("CAREER_MODEL", "careerops:32k")
	t.Setenv("MAIL_MODEL", "muse-glimmer:latest")
	if got := AgentModel("career_ops"); got.Model != "careerops:32k" {
		t.Fatalf("career legacy model, got %+v", got)
	}
	if got := AgentModel("mail"); got.Model != "muse-glimmer:latest" {
		t.Fatalf("mail legacy model, got %+v", got)
	}
}

func TestAgentModel_GenericForAgentWithNoLegacyVar(t *testing.T) {
	// Research had no dedicated variable before; the generic scheme gives it one.
	t.Setenv("AGENT_MODEL__RESEARCHER", "qwen3-coder:30b")
	t.Setenv("AGENT_PROVIDER__RESEARCHER", "ollama")
	if got := AgentModel("researcher"); got.Model != "qwen3-coder:30b" || got.Provider != "ollama" {
		t.Fatalf("research generic, got %+v", got)
	}
}

func TestAgentModel_UnsetIsEmpty(t *testing.T) {
	// Nothing set → empty, so the caller keeps using the server default.
	if got := AgentModel("linux_patch"); got != (AgentLLM{}) {
		t.Fatalf("want empty, got %+v", got)
	}
}

func TestBuiltinEnvKey(t *testing.T) {
	cases := map[string]string{
		"career_ops":          "CAREER_OPS",
		"article_writer":      "ARTICLE_WRITER",
		"jobshout_com_writer": "JOBSHOUT_COM_WRITER",
		"researcher":          "RESEARCHER",
	}
	for in, want := range cases {
		if got := builtinEnvKey(in); got != want {
			t.Errorf("builtinEnvKey(%q) = %q, want %q", in, got, want)
		}
	}
}
