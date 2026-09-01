package service

import (
	"context"

	"github.com/google/uuid"
)

// boardTaskSyncer is the slice of TaskService the specialist workers need.
type boardTaskSyncer interface {
	Transition(ctx context.Context, id uuid.UUID, status string, changedBy *uuid.UUID) error
}

// boardStatusForSpecialist maps a specialist run's terminal status onto a
// board column. Success moves the card to Done. Failure leaves it In Progress
// — there is no Failed column, matching generic task runs.
func boardStatusForSpecialist(runStatus string) (string, bool) {
	switch runStatus {
	case "completed":
		return "done", true
	default:
		return "", false
	}
}

func syncSpecialistBoard(ctx context.Context, tasks boardTaskSyncer, taskID *uuid.UUID, runStatus string) {
	if tasks == nil || taskID == nil {
		return
	}
	status, ok := boardStatusForSpecialist(runStatus)
	if !ok {
		return
	}
	_ = tasks.Transition(ctx, *taskID, status, nil)
}
