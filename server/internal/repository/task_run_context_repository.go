package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// TaskRunContexts decorates a TaskRunRepository with the run's role context
// (task_runs.run_context). It is kept beside the base repository rather than
// in its column lists so the run lifecycle code is untouched:
//
//   - Create stores run.Context, or, when unset, the context the request
//     carried (model.WithRunContext) — set on the run itself too, so the
//     caller's in-memory run (and later webhooks) see it.
//   - GetByID / ListByTask / ListByOrg fill Context back in.
//   - Update never writes the column, so the context survives every update.
type TaskRunContexts struct {
	TaskRunRepository
	Pool *pgxpool.Pool
}

func (t *TaskRunContexts) Create(ctx context.Context, run *model.TaskRun) error {
	if run.Context == nil {
		run.Context = model.RunContextFrom(ctx).Normalize()
	}
	if err := t.TaskRunRepository.Create(ctx, run); err != nil {
		return err
	}
	if run.Context == nil {
		return nil
	}
	raw, err := json.Marshal(run.Context)
	if err != nil {
		return fmt.Errorf("task_run_context: encode: %w", err)
	}
	if _, err := t.Pool.Exec(ctx, `UPDATE task_runs SET run_context = $2 WHERE id = $1`, run.ID, raw); err != nil {
		return fmt.Errorf("task_run_context: store: %w", err)
	}
	return nil
}

func (t *TaskRunContexts) GetByID(ctx context.Context, id uuid.UUID) (*model.TaskRun, error) {
	run, err := t.TaskRunRepository.GetByID(ctx, id)
	if err != nil || run == nil {
		return run, err
	}
	t.fill(ctx, []*model.TaskRun{run})
	return run, nil
}

func (t *TaskRunContexts) ListByTask(ctx context.Context, taskID uuid.UUID, p model.PaginationParams) (*model.PaginatedResponse[model.TaskRun], error) {
	out, err := t.TaskRunRepository.ListByTask(ctx, taskID, p)
	t.fillPage(ctx, out, err)
	return out, err
}

func (t *TaskRunContexts) ListByOrg(ctx context.Context, orgID uuid.UUID, p model.PaginationParams, filter TaskRunListFilter) (*model.PaginatedResponse[model.TaskRun], error) {
	out, err := t.TaskRunRepository.ListByOrg(ctx, orgID, p, filter)
	t.fillPage(ctx, out, err)
	return out, err
}

func (t *TaskRunContexts) fillPage(ctx context.Context, page *model.PaginatedResponse[model.TaskRun], err error) {
	if err != nil || page == nil || len(page.Data) == 0 {
		return
	}
	runs := make([]*model.TaskRun, len(page.Data))
	for i := range page.Data {
		runs[i] = &page.Data[i]
	}
	t.fill(ctx, runs)
}

// fill loads run_context for the runs in one query. Best-effort: a failure
// leaves Context nil rather than failing the read.
func (t *TaskRunContexts) fill(ctx context.Context, runs []*model.TaskRun) {
	ids := make([]uuid.UUID, len(runs))
	byID := make(map[uuid.UUID]*model.TaskRun, len(runs))
	for i, r := range runs {
		ids[i], byID[r.ID] = r.ID, r
	}
	rows, err := t.Pool.Query(ctx,
		`SELECT id, run_context FROM task_runs WHERE id = ANY($1) AND run_context IS NOT NULL`, ids)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var raw []byte
		if rows.Scan(&id, &raw) != nil {
			continue
		}
		var c model.RunContext
		if json.Unmarshal(raw, &c) == nil {
			byID[id].Context = &c
		}
	}
}
