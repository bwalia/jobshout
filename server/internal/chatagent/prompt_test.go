package chatagent

import (
	"strings"
	"testing"
	"time"
)

// The anti-eagerness guard is a live-model behaviour (Suite A case 8, a Fatal
// case): qwen3-coder over-triggers the help tool on a bare "hi" or "how are
// you?" until the prompt both states the rule and shows one worked example — an
// abstract rule alone did not move it, a concrete example did. This test does
// not exercise the model; it pins the two pieces of prompt text the live fix
// depends on so a later prompt edit cannot quietly delete them and let the
// greeting-calls-help regression back in unnoticed.
func TestSystemPromptCarriesSmallTalkGuard(t *testing.T) {
	p := systemPrompt(time.Now(), "", nil, nil, nil, "")
	for _, want := range []string{
		// the rule: small talk gets words and no tool, not even help
		"small talk",
		"no tool call — not even help",
		// the worked example the model actually imitates
		"Example of small talk",
		"how are you?",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt lost the small-talk guard: missing %q", want)
		}
	}
}
