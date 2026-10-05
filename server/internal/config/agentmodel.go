package config

import (
	"os"
	"strings"
)

// AgentLLM is the provider and model an agent should run on. Either field may
// be empty, which means "fall back to the server default" (LLM_PROVIDER and the
// provider's own default model). The caller decides that fallback; this package
// only reads what the operator set.
type AgentLLM struct {
	Provider string
	Model    string
}

// legacyModelEnv and legacyProviderEnv map a builtin to the dedicated variable
// it used before the generic scheme existed. They stay supported so no ring's
// current configuration changes meaning. A builtin with no entry simply has no
// legacy fallback.
var legacyModelEnv = map[string]string{
	"career_ops":       "CAREER_MODEL",
	"mail":             "MAIL_MODEL",
	"course_generator": "COURSE_MODEL",
	"linkedin_poster":  "LINKEDIN_MODEL",
	"article_writer":   "BLOG_MODEL",
}

var legacyProviderEnv = map[string]string{
	"career_ops": "CAREER_PROVIDER",
	"mail":       "MAIL_PROVIDER",
}

// AgentModel resolves the LLM provider and model for a builtin agent from the
// environment, so an operator can repoint any agent at a different model by
// setting a variable — no code change and no redeploy of the binary, just a
// restart that re-reads the environment.
//
// Precedence, applied independently to provider and model (first non-empty wins):
//
//  1. AGENT_PROVIDER__<BUILTIN> / AGENT_MODEL__<BUILTIN> — the generic, uniform
//     scheme that works for every agent, including ones with no dedicated var.
//  2. the builtin's legacy dedicated variable, if it has one (CAREER_MODEL,
//     MAIL_MODEL, COURSE_MODEL, LINKEDIN_MODEL, BLOG_MODEL, CAREER_PROVIDER,
//     MAIL_PROVIDER). Kept for back-compatibility.
//  3. empty — the caller falls back to LLM_PROVIDER and the provider's default.
//
// <BUILTIN> is the builtin id upper-cased with every non-alphanumeric character
// turned into "_": career_ops becomes AGENT_MODEL__CAREER_OPS.
func AgentModel(builtin string) AgentLLM {
	key := builtinEnvKey(builtin)
	return AgentLLM{
		Provider: firstEnv(
			"AGENT_PROVIDER__"+key,
			legacyProviderEnv[builtin],
		),
		Model: firstEnv(
			"AGENT_MODEL__"+key,
			legacyModelEnv[builtin],
		),
	}
}

// builtinEnvKey converts a builtin id into the suffix used in its env keys.
func builtinEnvKey(builtin string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(builtin)) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// firstEnv returns the trimmed value of the first set, non-empty environment
// variable among names. An empty name is skipped (a builtin with no legacy var).
func firstEnv(names ...string) string {
	for _, n := range names {
		if n == "" {
			continue
		}
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}
