// Package harness is the shared scaffolding for the agent evaluation suites
// under server/eval. It provides a scripted LLM, a check runner and a report
// writer, so each suite states what it expects rather than how to run it.
//
// Tier 1 suites (the default) are hermetic: no network, no GPU, no API keys.
// They gate merges. Tier 2 suites live behind the `evallive` build tag, talk to
// the real workstation, and answer "is the output any good" — a question Tier 1
// cannot reach. See docs/plans/README.md.
package harness

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jobshout/server/internal/llm"
)

// Script is one scripted reply. The first script whose Match appears in the
// request text is used, so order the specific before the general.
//
// Match is a substring rather than a regexp because the prompts these suites
// drive are built by the code under test: an exact anchor ("Classify the
// following email") is both available and more legible than a pattern.
type Script struct {
	// Match is looked for across every message in the request. Empty matches
	// anything, which makes it a catch-all when placed last.
	Match string
	// Reply is the assistant content returned when Match hits.
	Reply string
	// ToolCalls, when set, are returned instead of content, exercising the
	// native tool-calling path.
	ToolCalls []llm.ToolCall
	// Err, when set, makes the call fail — for testing degradation paths.
	Err error
	// Once limits this script to a single use, so a suite can script a first
	// turn differently from later ones.
	Once bool

	used bool
}

// FakeLLM is a scripted llm.Client that records every request it receives.
//
// It reports tool capability by default: the agents under evaluation take their
// native tool-calling path unless a suite says otherwise, and that is the path
// most worth testing.
type FakeLLM struct {
	mu       sync.Mutex
	scripts  []Script
	requests []llm.GenerateRequest

	// Fallback is returned when no script matches. A suite that leaves this
	// empty is asserting that every call it makes was anticipated: an
	// unmatched call fails loudly rather than returning a plausible blank.
	Fallback string
	// NoTools makes the client report no native tool support, forcing the
	// ReAct path.
	NoTools bool
}

// NewFakeLLM builds a scripted client.
func NewFakeLLM(scripts ...Script) *FakeLLM {
	return &FakeLLM{scripts: scripts}
}

// Script appends a scripted reply and returns the client for chaining.
func (f *FakeLLM) Script(s Script) *FakeLLM {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scripts = append(f.scripts, s)
	return f
}

func (f *FakeLLM) ProviderName() string { return "fake" }

// SupportsTools satisfies llm.ToolCapableClient.
func (f *FakeLLM) SupportsTools() bool { return !f.NoTools }

// Generate returns the first matching script, recording the request.
func (f *FakeLLM) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)

	text := RequestText(req)
	for i := range f.scripts {
		s := &f.scripts[i]
		if s.Once && s.used {
			continue
		}
		if s.Match != "" && !strings.Contains(text, s.Match) {
			continue
		}
		s.used = true
		if s.Err != nil {
			return nil, s.Err
		}
		return &llm.GenerateResponse{
			Content:      s.Reply,
			ToolCalls:    s.ToolCalls,
			FinishReason: "stop",
			Model:        "fake",
		}, nil
	}
	if f.Fallback != "" {
		return &llm.GenerateResponse{Content: f.Fallback, FinishReason: "stop", Model: "fake"}, nil
	}
	return nil, fmt.Errorf("harness: no script matched request: %s", truncate(text, 400))
}

// Requests returns a copy of every request seen, in order.
func (f *FakeLLM) Requests() []llm.GenerateRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]llm.GenerateRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

// Calls is the number of Generate calls made.
func (f *FakeLLM) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// LastPrompt is the full text of the most recent request, for assertions about
// what the code under test actually asked the model.
func (f *FakeLLM) LastPrompt() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return ""
	}
	return RequestText(f.requests[len(f.requests)-1])
}

// AllPrompts is every request's text, joined — for "was this ever mentioned"
// assertions that should not care which call carried it.
func (f *FakeLLM) AllPrompts() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var b strings.Builder
	for _, r := range f.requests {
		b.WriteString(RequestText(r))
		b.WriteString("\n")
	}
	return b.String()
}

// RequestText flattens every message in a request into one searchable string.
func RequestText(req llm.GenerateRequest) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
