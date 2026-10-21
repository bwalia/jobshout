package video

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jobshout/server/internal/llm"
)

type fakeLLM struct {
	name  string
	resp  string
	err   error
	calls int
}

func (f *fakeLLM) ProviderName() string { return f.name }

func (f *fakeLLM) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &llm.GenerateResponse{Content: f.resp}, nil
}

const goodScript = `{"title":"Hills","shots":[{"narration":"Hello there, friend.","visual":"a wide shot of a green hill at dawn"}]}`

func blockErr() error {
	return &llm.ProviderError{Provider: "gemini", Kind: llm.ErrProviderBadRequest, Message: "prompt blocked: PROHIBITED_CONTENT"}
}

func TestWriteFallsBackToLocalModelOnContentBlock(t *testing.T) {
	primary := &fakeLLM{name: "gemini", err: blockErr()}
	fallback := &fakeLLM{name: "ollama", resp: goodScript}
	w := &Writer{LLM: primary, Fallback: fallback}

	script, err := w.Write(context.Background(), Brief{Topic: "lighthouses"})
	if err != nil {
		t.Fatalf("expected the fallback to succeed, got %v", err)
	}
	if script.Title != "Hills" {
		t.Fatalf("script did not come from the fallback: %+v", script)
	}
	// The block is not retried on the primary, and the fallback is tried once.
	if primary.calls != 1 {
		t.Errorf("primary called %d times, want 1 (a block must not be retried)", primary.calls)
	}
	if fallback.calls != 1 {
		t.Errorf("fallback called %d times, want 1", fallback.calls)
	}
}

func TestWriteDoesNotFallBackOnOrdinaryFailure(t *testing.T) {
	defer func(d time.Duration) { writeRetryWait = d }(writeRetryWait)
	writeRetryWait = time.Millisecond // don't wait minutes between the primary's retries

	primary := &fakeLLM{name: "gemini", err: errors.New("server busy")}
	fallback := &fakeLLM{name: "ollama", resp: goodScript}
	w := &Writer{LLM: primary, Fallback: fallback}

	if _, err := w.Write(context.Background(), Brief{Topic: "lighthouses"}); err == nil {
		t.Fatal("expected a non-block failure to surface, not silently fall back")
	}
	if fallback.calls != 0 {
		t.Errorf("fallback called %d times on a non-block error, want 0", fallback.calls)
	}
	if primary.calls != 3 {
		t.Errorf("primary called %d times, want 3 (transient retries)", primary.calls)
	}
}

func TestWriteReturnsBlockWhenFallbackAlsoFails(t *testing.T) {
	defer func(d time.Duration) { writeRetryWait = d }(writeRetryWait)
	writeRetryWait = time.Millisecond // the fallback's bad JSON triggers retries

	primary := &fakeLLM{name: "gemini", err: blockErr()}
	fallback := &fakeLLM{name: "ollama", resp: "not json at all"}
	w := &Writer{LLM: primary, Fallback: fallback}

	_, err := w.Write(context.Background(), Brief{Topic: "lighthouses"})
	if !llm.IsContentBlocked(err) {
		t.Fatalf("want the original block surfaced, got %v", err)
	}
}
