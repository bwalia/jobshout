package llm

import (
	"strings"
	"testing"
)

func TestRecoverLeakedToolCalls_FunctionMarkup(t *testing.T) {
	text := "I'll create that image for you.\n\n" +
		"<function=image_generate>\n<parameter=prompt>\nA realistic tiger doing Bhangra\n</parameter>\n</function>"
	calls, cleaned, ok := recoverLeakedToolCalls(text)
	if !ok || len(calls) != 1 {
		t.Fatalf("expected 1 recovered call, got ok=%v calls=%v", ok, calls)
	}
	if calls[0].Name != "image_generate" || calls[0].ID != "call_0" {
		t.Fatalf("unexpected call: %+v", calls[0])
	}
	if got := calls[0].Arguments["prompt"]; got != "A realistic tiger doing Bhangra" {
		t.Fatalf("unexpected prompt arg: %q", got)
	}
	if cleaned != "I'll create that image for you." {
		t.Fatalf("markup not stripped: %q", cleaned)
	}
}

func TestRecoverLeakedToolCalls_ToolCallJSON(t *testing.T) {
	text := "<tool_call>\n{\"name\": \"task_create\", \"arguments\": {\"title\": \"Tiger article\", \"priority\": 2}}\n</tool_call>"
	calls, cleaned, ok := recoverLeakedToolCalls(text)
	if !ok || len(calls) != 1 || calls[0].Name != "task_create" {
		t.Fatalf("expected recovered task_create, got ok=%v calls=%v", ok, calls)
	}
	if calls[0].Arguments["priority"] != float64(2) {
		t.Fatalf("expected numeric arg preserved, got %#v", calls[0].Arguments["priority"])
	}
	if cleaned != "" {
		t.Fatalf("expected empty cleaned text, got %q", cleaned)
	}
}

func TestRecoverLeakedToolCalls_TypedParamsAndUnclosedTail(t *testing.T) {
	text := "Working on it.\n" +
		"<function=schedule_create>\n" +
		"<parameter=name>\nTiger Articles\n</parameter>\n" +
		"<parameter=dry_run>\ntrue\n</parameter>\n" +
		"</function>\n" +
		"<function=task_create>\n<parameter=title>\ncut off mid-stream"
	calls, cleaned, ok := recoverLeakedToolCalls(text)
	if !ok || len(calls) != 1 || calls[0].Name != "schedule_create" {
		t.Fatalf("expected only the well-formed call, got ok=%v calls=%v", ok, calls)
	}
	if calls[0].Arguments["dry_run"] != true {
		t.Fatalf("expected boolean coercion, got %#v", calls[0].Arguments["dry_run"])
	}
	if strings.Contains(cleaned, "<function=") || strings.Contains(cleaned, "cut off") {
		t.Fatalf("unclosed trailing markup not stripped: %q", cleaned)
	}
	if cleaned != "Working on it." {
		t.Fatalf("unexpected cleaned text: %q", cleaned)
	}
}

func TestRecoverLeakedToolCalls_PlainTextUntouched(t *testing.T) {
	text := "Here is your answer: a < b and b > c."
	calls, cleaned, ok := recoverLeakedToolCalls(text)
	if ok || calls != nil || cleaned != text {
		t.Fatalf("plain text must pass through, got ok=%v cleaned=%q", ok, cleaned)
	}
}

func TestLeakStreamGuard_SuppressesFromMarker(t *testing.T) {
	var got strings.Builder
	g := &leakStreamGuard{onToken: func(s string) { got.WriteString(s) }}
	for _, chunk := range []string{"I'll create", " that image.\n\n", "<func", "tion=image_generate>", "\n<parameter=prompt>", "tiger</parameter>"} {
		g.feed(chunk)
	}
	if s := got.String(); s != "I'll create that image.\n\n" {
		t.Fatalf("markup leaked into stream: %q", s)
	}
}

func TestLeakStreamGuard_ReleasesFalsePrefix(t *testing.T) {
	var got strings.Builder
	g := &leakStreamGuard{onToken: func(s string) { got.WriteString(s) }}
	g.feed("use a <function")
	g.feed("al style here")
	if s := got.String(); s != "use a <functional style here" {
		t.Fatalf("false marker prefix mangled the stream: %q", s)
	}
}
