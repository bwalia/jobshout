package repository

import (
	"testing"
	"time"
)

func TestNextCompletedAt(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	prior := now.Add(-time.Hour)

	if got := nextCompletedAt("in_progress", &prior, now); got != nil {
		t.Fatalf("leaving done (or any non-done) must clear completed_at, got %v", got)
	}
	if got := nextCompletedAt("done", nil, now); got == nil || !got.Equal(now) {
		t.Fatalf("first completion should stamp now, got %v", got)
	}
	if got := nextCompletedAt("done", &prior, now); got == nil || !got.Equal(prior) {
		t.Fatalf("already completed must keep the original time, got %v", got)
	}
}

func TestShouldRecordStatusHistory(t *testing.T) {
	if shouldRecordStatusHistory("todo", "todo") {
		t.Fatal("same status is a no-op")
	}
	if !shouldRecordStatusHistory("todo", "in_progress") {
		t.Fatal("a real move must be recorded")
	}
}
