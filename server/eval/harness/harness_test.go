package harness

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/llm"
)

func req(content string) llm.GenerateRequest {
	return llm.GenerateRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: content}}}
}

func TestFakeLLMMatchesFirstScript(t *testing.T) {
	f := NewFakeLLM(
		Script{Match: "classify", Reply: "IGNORE"},
		Script{Match: "", Reply: "catch-all"},
	)
	got, err := f.Generate(context.Background(), req("please classify this"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "IGNORE" {
		t.Fatalf("content = %q; want IGNORE", got.Content)
	}
	got, _ = f.Generate(context.Background(), req("something else"))
	if got.Content != "catch-all" {
		t.Fatalf("content = %q; want catch-all", got.Content)
	}
	if f.Calls() != 2 {
		t.Fatalf("calls = %d; want 2", f.Calls())
	}
}

func TestFakeLLMUnmatchedCallIsAnError(t *testing.T) {
	f := NewFakeLLM(Script{Match: "nope", Reply: "x"})
	if _, err := f.Generate(context.Background(), req("hello")); err == nil {
		t.Fatal("expected an error for an unanticipated call")
	}
}

func TestFakeLLMOnceScriptRetires(t *testing.T) {
	f := NewFakeLLM(
		Script{Match: "go", Reply: "first", Once: true},
		Script{Match: "go", Reply: "second"},
	)
	first, _ := f.Generate(context.Background(), req("go"))
	second, _ := f.Generate(context.Background(), req("go"))
	if first.Content != "first" || second.Content != "second" {
		t.Fatalf("got %q then %q", first.Content, second.Content)
	}
}

func TestFakeLLMPropagatesScriptedError(t *testing.T) {
	boom := errors.New("model down")
	f := NewFakeLLM(Script{Match: "", Err: boom})
	if _, err := f.Generate(context.Background(), req("x")); !errors.Is(err, boom) {
		t.Fatalf("err = %v; want %v", err, boom)
	}
}

func TestRequireSubsetCatchesInventedURL(t *testing.T) {
	allowed := []string{"https://example.com/lathe"}
	err := RequireSubset("draft", []string{"https://evil.example/invented"}, allowed)
	if err == nil {
		t.Fatal("expected an invented URL to fail the subset check")
	}
	if err := RequireSubset("draft", nil, allowed); err != nil {
		t.Fatalf("citing nothing should pass: %v", err)
	}
	// Trailing slash and case must not read as a different source.
	if err := RequireSubset("draft", []string{"https://Example.com/lathe/"}, allowed); err != nil {
		t.Fatalf("normalised URL should pass: %v", err)
	}
}

func TestURLsInStripsTrailingPunctuation(t *testing.T) {
	got := URLsIn("see https://example.com/lathe-9000. also [x](https://example.com/b)")
	want := []string{"https://example.com/lathe-9000", "https://example.com/b"}
	if len(got) != len(want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v; want %v", got, want)
		}
	}
}

func TestReportFatalGating(t *testing.T) {
	r := &Report{Suite: "x", Outcomes: []Outcome{
		{Name: "a", Passed: true},
		{Name: "b", Passed: false, Fatal: false},
	}}
	if !r.Passed() {
		t.Fatal("a non-fatal failure must not fail the suite")
	}
	r.Outcomes = append(r.Outcomes, Outcome{Name: "c", Passed: false, Fatal: true})
	if r.Passed() {
		t.Fatal("a fatal failure must fail the suite")
	}
}

func TestSuiteMarkdownRendersVerdict(t *testing.T) {
	s := &Suite{Name: "mail", Tier: "1"}
	s.Add(&Report{Case: "price_with_link", Outcomes: []Outcome{
		{Name: "never_sends", Fatal: true, Passed: true},
	}})
	md := s.Markdown()
	if !strings.Contains(md, "**PASS**") || !strings.Contains(md, "price_with_link") {
		t.Fatalf("markdown missing verdict or case:\n%s", md)
	}
}
