package chatagent

import (
	"strings"
	"testing"
)

func TestSanitiseMessage_StripsLeakedFunctionMarkup(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			"closed function block",
			"I'll create that image.\n\n<function=image_generate>\n<parameter=prompt>\ntiger\n</parameter>\n</function>",
			"I'll create that image.",
		},
		{
			"unclosed block stripped to end",
			"Done!\n<function=task_create>\n<parameter=title>\ncut off",
			"Done!",
		},
		{
			"tool_call json block",
			"<tool_call>\n{\"name\": \"help\"}\n</tool_call>\nAnything else?",
			"Anything else?",
		},
		{
			"prose with angle brackets survives",
			"Note that a < b and b > c here.",
			"Note that a < b and b > c here.",
		},
	}
	for _, c := range cases {
		if got := SanitiseMessage(c.in); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestContainsToolScaffolding_LeakedMarkup(t *testing.T) {
	if !ContainsToolScaffolding("<function=image_generate>") {
		t.Error("function markup must count as scaffolding")
	}
	if !ContainsToolScaffolding("<tool_call>{}</tool_call>") {
		t.Error("tool_call markup must count as scaffolding")
	}
	if ContainsToolScaffolding(strings.Repeat("plain text ", 3)) {
		t.Error("plain text must not count as scaffolding")
	}
}
