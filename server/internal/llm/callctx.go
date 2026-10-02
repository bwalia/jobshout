package llm

import (
	"context"
	"sync/atomic"
)

// Per-call labels carried on ctx. They are read by decorators installed with
// Router.WrapClients (the LLM benchmark recorder) and never change what a
// call does: an agent marks which step it is in, and the decorator records it.

type stageKey struct{}
type attemptKey struct{}
type callStatsKey struct{}

// WithStage labels every LLM call made under ctx as belonging to step, e.g.
// "research", "outline" or "write". Labelling only — it changes nothing about
// the call.
func WithStage(ctx context.Context, step string) context.Context {
	return context.WithValue(ctx, stageKey{}, step)
}

// StageFrom returns the step label on ctx, or "" when none was set.
func StageFrom(ctx context.Context) string {
	s, _ := ctx.Value(stageKey{}).(string)
	return s
}

// withAttempt marks the calls under ctx as the n-th try at the same request
// (GenerateJSON's corrective retry is attempt 2).
func withAttempt(ctx context.Context, n int) context.Context {
	return context.WithValue(ctx, attemptKey{}, n)
}

// AttemptFrom returns the attempt number on ctx; 1 when none was set.
func AttemptFrom(ctx context.Context) int {
	if n, ok := ctx.Value(attemptKey{}).(int); ok && n > 0 {
		return n
	}
	return 1
}

// CallStats collects what a provider did inside one Generate call that the
// caller cannot see from the outside — today, transport-level retries.
type CallStats struct {
	retries atomic.Int32
}

// Retries is how many times the request was re-sent after a transient
// failure; 0 when the first try settled it.
func (s *CallStats) Retries() int { return int(s.retries.Load()) }

// WithCallStats attaches a fresh CallStats to ctx for one Generate call.
func WithCallStats(ctx context.Context) (context.Context, *CallStats) {
	s := &CallStats{}
	return context.WithValue(ctx, callStatsKey{}, s), s
}

// noteRetry counts one re-send on the CallStats on ctx, if any.
func noteRetry(ctx context.Context) {
	if s, ok := ctx.Value(callStatsKey{}).(*CallStats); ok {
		s.retries.Add(1)
	}
}
