package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

type TaskRepository interface {
	Create(ctx context.Context, task *model.Task) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Task, error)
	ListByProject(ctx context.Context, projectID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Task], error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Task], error)
	ListComments(ctx context.Context, taskID uuid.UUID) ([]model.TaskComment, error)
	AddComment(ctx context.Context, comment *model.TaskComment) error
	ListHistory(ctx context.Context, taskID uuid.UUID) (*model.TaskHistory, error)
	Update(ctx context.Context, task *model.Task) error
	Delete(ctx context.Context, id uuid.UUID) error
	TransitionStatus(ctx context.Context, id uuid.UUID, status string, changedBy *uuid.UUID) error
	Reorder(ctx context.Context, id uuid.UUID, status string, position int, changedBy *uuid.UUID) error
	FindByLaunchRunID(ctx context.Context, runID uuid.UUID) (*model.Task, error)
}

type taskRepository struct {
	pool *pgxpool.Pool
}

func NewTaskRepository(pool *pgxpool.Pool) TaskRepository {
	return &taskRepository{pool: pool}
}

// taskSelectColumns + taskFrom join the latest task_runs row so list/get can
// render a progress chip without an N+1.
const taskSelectColumns = `
	t.id, t.project_id, t.parent_id, t.title, t.description, t.status, t.priority,
	t.assigned_agent_id, t.assigned_user_id, t.story_points, t.due_date, t.position,
	t.created_by, t.metadata, t.created_at, t.updated_at, t.completed_at,
	lr.id, lr.status, COALESCE(lr.completed_at, lr.created_at)`

const taskFrom = `
	tasks t
	LEFT JOIN LATERAL (
		SELECT id, status, completed_at, created_at
		FROM task_runs
		WHERE task_id = t.id
		ORDER BY created_at DESC
		LIMIT 1
	) lr ON TRUE`

func (r *taskRepository) Create(ctx context.Context, task *model.Task) error {
	var maxPos int
	posQuery := `SELECT COALESCE(MAX(position), -1) + 1 FROM tasks WHERE project_id = $1 AND status = $2`
	if err := r.pool.QueryRow(ctx, posQuery, task.ProjectID, task.Status).Scan(&maxPos); err != nil {
		return fmt.Errorf("getting max position: %w", err)
	}
	task.Position = maxPos

	meta, err := json.Marshal(task.Metadata)
	if err != nil || task.Metadata == nil {
		meta = []byte("{}")
	}

	if task.Status == "done" && task.CompletedAt == nil {
		now := time.Now()
		task.CompletedAt = &now
	}

	query := `
		INSERT INTO tasks (id, project_id, parent_id, title, description, status, priority,
			assigned_agent_id, assigned_user_id, story_points, due_date, position, created_by, metadata,
			completed_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW())
		RETURNING created_at, updated_at, completed_at`

	return r.pool.QueryRow(ctx, query,
		task.ID, task.ProjectID, task.ParentID, task.Title, task.Description,
		task.Status, task.Priority, task.AssignedAgentID, task.AssignedUserID,
		task.StoryPoints, task.DueDate, task.Position, task.CreatedBy, meta,
		task.CompletedAt,
	).Scan(&task.CreatedAt, &task.UpdatedAt, &task.CompletedAt)
}

func (r *taskRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Task, error) {
	query := `SELECT ` + taskSelectColumns + ` FROM ` + taskFrom + ` WHERE t.id = $1`
	t, err := scanTask(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding task by id: %w", err)
	}
	return t, nil
}

func (r *taskRepository) FindByLaunchRunID(ctx context.Context, runID uuid.UUID) (*model.Task, error) {
	query := `SELECT ` + taskSelectColumns + ` FROM ` + taskFrom + ` WHERE t.metadata->>'run_id' = $1 LIMIT 1`
	t, err := scanTask(r.pool.QueryRow(ctx, query, runID.String()))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding task by launch run: %w", err)
	}
	return t, nil
}

func (r *taskRepository) ListByProject(ctx context.Context, projectID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	params.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE project_id = $1`, projectID).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting tasks: %w", err)
	}

	query := `
		SELECT ` + taskSelectColumns + `
		FROM ` + taskFrom + `
		WHERE t.project_id = $1
		ORDER BY t.status, t.position ASC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, projectID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}
	defer rows.Close()
	return pageTasks(rows, total, params)
}

func (r *taskRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	params.Normalize()

	where := `t.project_id IN (SELECT id FROM projects WHERE org_id = $1)`
	args := []any{orgID}
	if params.Status != "" {
		args = append(args, params.Status)
		where += fmt.Sprintf(` AND t.status = $%d`, len(args))
	}
	if params.AssignedAgentID != "" {
		aid, err := uuid.Parse(params.AssignedAgentID)
		if err != nil {
			return nil, fmt.Errorf("invalid assigned_agent_id: %w", err)
		}
		args = append(args, aid)
		where += fmt.Sprintf(` AND t.assigned_agent_id = $%d`, len(args))
	}

	var total int
	countQuery := `SELECT COUNT(*) FROM tasks t WHERE ` + where
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting org tasks: %w", err)
	}

	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2
	query := fmt.Sprintf(`
		SELECT `+taskSelectColumns+`
		FROM `+taskFrom+`
		WHERE %s
		ORDER BY t.updated_at DESC
		LIMIT $%d OFFSET $%d`, where, limitIdx, offsetIdx)

	rows, err := r.pool.Query(ctx, query, append(args, params.PerPage, params.Offset())...)
	if err != nil {
		return nil, fmt.Errorf("listing org tasks: %w", err)
	}
	defer rows.Close()
	return pageTasks(rows, total, params)
}

func pageTasks(rows pgx.Rows, total int, params model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	tasks := make([]model.Task, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning task: %w", err)
		}
		tasks = append(tasks, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	totalPages := total / params.PerPage
	if total%params.PerPage != 0 {
		totalPages++
	}
	return &model.PaginatedResponse[model.Task]{
		Data: tasks, Total: total, Page: params.Page,
		PerPage: params.PerPage, TotalPages: totalPages,
	}, nil
}

func (r *taskRepository) ListComments(ctx context.Context, taskID uuid.UUID) ([]model.TaskComment, error) {
	query := `SELECT id, task_id, author_id, agent_id, body, created_at
		FROM task_comments WHERE task_id = $1 ORDER BY created_at ASC`

	rows, err := r.pool.Query(ctx, query, taskID)
	if err != nil {
		return nil, fmt.Errorf("listing task comments: %w", err)
	}
	defer rows.Close()

	comments := make([]model.TaskComment, 0)
	for rows.Next() {
		var c model.TaskComment
		if err := rows.Scan(&c.ID, &c.TaskID, &c.AuthorID, &c.AgentID, &c.Body, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning task comment: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, nil
}

func (r *taskRepository) AddComment(ctx context.Context, comment *model.TaskComment) error {
	query := `INSERT INTO task_comments (id, task_id, author_id, agent_id, body, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW()) RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		comment.ID, comment.TaskID, comment.AuthorID, comment.AgentID, comment.Body,
	).Scan(&comment.CreatedAt)
}

func (r *taskRepository) ListHistory(ctx context.Context, taskID uuid.UUID) (*model.TaskHistory, error) {
	task, err := r.FindByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, nil
	}

	events := make([]model.TaskHistoryEvent, 0)

	statusRows, err := r.pool.Query(ctx, `
		SELECT id, old_status, new_status, changed_at
		FROM task_status_history
		WHERE task_id = $1`, taskID)
	if err != nil {
		return nil, fmt.Errorf("listing task status history: %w", err)
	}
	defer statusRows.Close()
	for statusRows.Next() {
		var e model.TaskHistoryEvent
		var hid uuid.UUID
		var changedAt time.Time
		if err := statusRows.Scan(&hid, &e.OldStatus, &e.NewStatus, &changedAt); err != nil {
			return nil, fmt.Errorf("scanning status history: %w", err)
		}
		e.ID = hid.String()
		e.Kind = "status"
		e.ChangedAt = &changedAt
		e.CreatedAt = changedAt
		events = append(events, e)
	}
	if err := statusRows.Err(); err != nil {
		return nil, err
	}

	runRows, err := r.pool.Query(ctx, `
		SELECT id, agent_id, status, created_at, completed_at
		FROM task_runs WHERE task_id = $1`, taskID)
	if err != nil {
		return nil, fmt.Errorf("listing task runs for history: %w", err)
	}
	defer runRows.Close()
	for runRows.Next() {
		var e model.TaskHistoryEvent
		var runID uuid.UUID
		if err := runRows.Scan(&runID, &e.AgentID, &e.Status, &e.CreatedAt, &e.CompletedAt); err != nil {
			return nil, fmt.Errorf("scanning run history: %w", err)
		}
		e.Kind = "run"
		e.ID = runID.String()
		e.RunID = &runID
		events = append(events, e)
	}
	if err := runRows.Err(); err != nil {
		return nil, err
	}

	hasRun := false
	for _, e := range events {
		if e.Kind == "run" {
			hasRun = true
			break
		}
	}
	if !hasRun {
		if ev := specialistHistoryEvent(task); ev != nil {
			events = append(events, *ev)
		}
	}

	sort.SliceStable(events, func(i, j int) bool {
		return historyEventTime(events[i]).After(historyEventTime(events[j]))
	})

	return &model.TaskHistory{CompletedAt: task.CompletedAt, Events: events}, nil
}

func specialistHistoryEvent(task *model.Task) *model.TaskHistoryEvent {
	if task == nil || task.Metadata == nil {
		return nil
	}
	kind, _ := task.Metadata[model.TaskMetaLaunchKind].(string)
	kind = strings.TrimSpace(kind)
	if kind == "" || kind == "task_run" {
		return nil
	}
	e := &model.TaskHistoryEvent{
		ID:         "specialist:" + task.ID.String(),
		Kind:       "specialist",
		LaunchKind: &kind,
		CreatedAt:  task.UpdatedAt,
		Status:     &task.Status,
	}
	if raw, ok := task.Metadata[model.TaskMetaRunID].(string); ok {
		if rid, err := uuid.Parse(raw); err == nil {
			e.RunID = &rid
		}
	}
	return e
}

func historyEventTime(e model.TaskHistoryEvent) time.Time {
	if e.ChangedAt != nil {
		return *e.ChangedAt
	}
	if e.CompletedAt != nil {
		return *e.CompletedAt
	}
	return e.CreatedAt
}

func (r *taskRepository) Update(ctx context.Context, task *model.Task) error {
	meta, err := json.Marshal(task.Metadata)
	if err != nil || task.Metadata == nil {
		meta = []byte("{}")
	}
	query := `
		UPDATE tasks SET title = $1, description = $2, priority = $3,
			assigned_agent_id = $4, assigned_user_id = $5, story_points = $6,
			due_date = $7, metadata = $8, updated_at = NOW()
		WHERE id = $9
		RETURNING updated_at`

	return r.pool.QueryRow(ctx, query,
		task.Title, task.Description, task.Priority,
		task.AssignedAgentID, task.AssignedUserID, task.StoryPoints,
		task.DueDate, meta, task.ID,
	).Scan(&task.UpdatedAt)
}

func scanTask(row pgx.Row) (*model.Task, error) {
	t := &model.Task{}
	var metaRaw []byte
	if err := row.Scan(
		&t.ID, &t.ProjectID, &t.ParentID, &t.Title, &t.Description,
		&t.Status, &t.Priority, &t.AssignedAgentID, &t.AssignedUserID,
		&t.StoryPoints, &t.DueDate, &t.Position, &t.CreatedBy, &metaRaw,
		&t.CreatedAt, &t.UpdatedAt, &t.CompletedAt,
		&t.LastRunID, &t.LastRunStatus, &t.LastRunAt,
	); err != nil {
		return nil, err
	}
	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &t.Metadata)
	}
	if t.Metadata == nil {
		t.Metadata = map[string]any{}
	}
	return t, nil
}

func (r *taskRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	return err
}

func (r *taskRepository) TransitionStatus(ctx context.Context, id uuid.UUID, status string, changedBy *uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var projectID uuid.UUID
	var oldStatus string
	var completedAt *time.Time
	if err := tx.QueryRow(ctx,
		`SELECT project_id, status, completed_at FROM tasks WHERE id = $1 FOR UPDATE`, id,
	).Scan(&projectID, &oldStatus, &completedAt); err != nil {
		return fmt.Errorf("finding task: %w", err)
	}

	if !shouldRecordStatusHistory(oldStatus, status) {
		return tx.Commit(ctx)
	}

	var maxPos int
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM tasks WHERE project_id = $1 AND status = $2`,
		projectID, status,
	).Scan(&maxPos); err != nil {
		return fmt.Errorf("getting max position: %w", err)
	}

	next := nextCompletedAt(status, completedAt, time.Now())
	if _, err = tx.Exec(ctx,
		`UPDATE tasks SET status = $1, position = $2, completed_at = $3, updated_at = NOW() WHERE id = $4`,
		status, maxPos, next, id,
	); err != nil {
		return fmt.Errorf("updating task status: %w", err)
	}

	if err := insertStatusHistory(ctx, tx, id, oldStatus, status, changedBy); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *taskRepository) Reorder(ctx context.Context, id uuid.UUID, status string, position int, changedBy *uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var projectID uuid.UUID
	var oldStatus string
	var completedAt *time.Time
	if err := tx.QueryRow(ctx,
		`SELECT project_id, status, completed_at FROM tasks WHERE id = $1 FOR UPDATE`, id,
	).Scan(&projectID, &oldStatus, &completedAt); err != nil {
		return fmt.Errorf("finding task for reorder: %w", err)
	}

	if _, err = tx.Exec(ctx,
		`UPDATE tasks SET position = position + 1 WHERE project_id = $1 AND status = $2 AND position >= $3 AND id != $4`,
		projectID, status, position, id,
	); err != nil {
		return fmt.Errorf("shifting tasks: %w", err)
	}

	next := completedAt
	if shouldRecordStatusHistory(oldStatus, status) {
		next = nextCompletedAt(status, completedAt, time.Now())
	}

	if _, err = tx.Exec(ctx,
		`UPDATE tasks SET status = $1, position = $2, completed_at = $3, updated_at = NOW() WHERE id = $4`,
		status, position, next, id,
	); err != nil {
		return fmt.Errorf("placing task: %w", err)
	}

	if shouldRecordStatusHistory(oldStatus, status) {
		if err := insertStatusHistory(ctx, tx, id, oldStatus, status, changedBy); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func insertStatusHistory(ctx context.Context, tx pgx.Tx, taskID uuid.UUID, oldStatus, newStatus string, changedBy *uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO task_status_history (id, task_id, old_status, new_status, changed_by, changed_at)
		VALUES ($1, $2, $3, $4, $5, NOW())`,
		uuid.New(), taskID, oldStatus, newStatus, changedBy,
	)
	if err != nil {
		return fmt.Errorf("recording task status history: %w", err)
	}
	return nil
}
