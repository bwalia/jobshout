package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jobshout/server/internal/model"
)

func TestSpecialistHistoryEvent(t *testing.T) {
	if specialistHistoryEvent(&model.Task{}) != nil {
		t.Fatal("no metadata should yield no specialist event")
	}

	id := uuid.New()
	runID := uuid.New()
	task := &model.Task{
		ID:        id,
		Status:    "in_progress",
		UpdatedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		Metadata: map[string]any{
			model.TaskMetaLaunchKind: "researcher",
			model.TaskMetaRunID:      runID.String(),
		},
	}
	ev := specialistHistoryEvent(task)
	if ev == nil || ev.Kind != "specialist" || ev.LaunchKind == nil || *ev.LaunchKind != "researcher" {
		t.Fatalf("expected researcher specialist event, got %#v", ev)
	}
	if ev.RunID == nil || *ev.RunID != runID {
		t.Fatalf("run id: got %v want %v", ev.RunID, runID)
	}

	task.Metadata[model.TaskMetaLaunchKind] = "task_run"
	if specialistHistoryEvent(task) != nil {
		t.Fatal("generic task_run launches are already in the runs list")
	}
}
