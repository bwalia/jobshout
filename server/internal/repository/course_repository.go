package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// ErrCourseRunNotFound is returned when a course run does not exist.
var ErrCourseRunNotFound = errors.New("course run not found")

// CourseRepository persists Course Generator runs and their chapters.
type CourseRepository interface {
	CreateRun(ctx context.Context, run *model.CourseRun) error
	GetRun(ctx context.Context, id uuid.UUID) (*model.CourseRun, error)
	ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CourseRun], error)
	UpdateSteps(ctx context.Context, id uuid.UUID, steps []model.CourseRunStep) error
	SaveOutline(ctx context.Context, id uuid.UUID, outline *model.CourseOutline, sources []model.CourseSource) error
	SetCover(ctx context.Context, id uuid.UUID, url string) error
	AppendWarning(ctx context.Context, id uuid.UUID, warning string) error
	// Transition moves a run to a status. It only changes rows that are not
	// already terminal, and reports whether a row changed.
	Transition(ctx context.Context, id uuid.UUID, status string, errMsg *string) (bool, error)
	TouchHeartbeat(ctx context.Context, id uuid.UUID) error
	ListStaleRunning(ctx context.Context, olderThan time.Time) ([]uuid.UUID, error)

	UpsertChapter(ctx context.Context, ch *model.CourseChapter) error
	ListChapters(ctx context.Context, runID uuid.UUID) ([]model.CourseChapter, error)
}

type courseRepository struct{ pool *pgxpool.Pool }

// NewCourseRepository constructs a Postgres-backed store.
func NewCourseRepository(pool *pgxpool.Pool) CourseRepository {
	return &courseRepository{pool: pool}
}

const courseRunColumns = `
	id, agent_id, task_id, org_id, status, brief, outline, steps, cover_url,
	sources, warnings, error_message, requested_by, heartbeat_at, started_at,
	completed_at, created_at, updated_at`

func scanCourseRun(row pgx.Row) (*model.CourseRun, error) {
	run := &model.CourseRun{}
	var brief, outline, steps, sources, warnings []byte
	if err := row.Scan(
		&run.ID, &run.AgentID, &run.TaskID, &run.OrgID, &run.Status, &brief, &outline,
		&steps, &run.CoverURL, &sources, &warnings, &run.ErrorMessage, &run.RequestedBy,
		&run.HeartbeatAt, &run.StartedAt, &run.CompletedAt, &run.CreatedAt, &run.UpdatedAt,
	); err != nil {
		return nil, err
	}
	// Tolerant decode: a malformed column must not hide the run.
	_ = json.Unmarshal(brief, &run.Brief)
	if len(outline) > 0 {
		var o model.CourseOutline
		if json.Unmarshal(outline, &o) == nil {
			run.Outline = &o
		}
	}
	_ = json.Unmarshal(steps, &run.Steps)
	_ = json.Unmarshal(sources, &run.Sources)
	_ = json.Unmarshal(warnings, &run.Warnings)
	return run, nil
}

func (r *courseRepository) CreateRun(ctx context.Context, run *model.CourseRun) error {
	brief, err := json.Marshal(run.Brief)
	if err != nil {
		return err
	}
	steps, err := json.Marshal(run.Steps)
	if err != nil {
		return err
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO course_runs (id, agent_id, task_id, org_id, status, brief, steps,
			requested_by, heartbeat_at, started_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW(),NOW())
		RETURNING created_at, updated_at`,
		run.ID, run.AgentID, run.TaskID, run.OrgID, run.Status, brief, steps,
		run.RequestedBy, run.HeartbeatAt, run.StartedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *courseRepository) GetRun(ctx context.Context, id uuid.UUID) (*model.CourseRun, error) {
	run, err := scanCourseRun(r.pool.QueryRow(ctx, `SELECT `+courseRunColumns+` FROM course_runs WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCourseRunNotFound
	}
	return run, err
}

func (r *courseRepository) ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CourseRun], error) {
	pagination.Normalize()
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM course_runs WHERE org_id=$1`, orgID).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+courseRunColumns+` FROM course_runs WHERE org_id=$1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, pagination.PerPage, pagination.Offset())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]model.CourseRun, 0, pagination.PerPage)
	for rows.Next() {
		run, err := scanCourseRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	pages := 1
	if pagination.PerPage > 0 {
		pages = (total + pagination.PerPage - 1) / pagination.PerPage
	}
	return &model.PaginatedResponse[model.CourseRun]{
		Data: runs, Total: total, Page: pagination.Page, PerPage: pagination.PerPage, TotalPages: pages,
	}, nil
}

func (r *courseRepository) UpdateSteps(ctx context.Context, id uuid.UUID, steps []model.CourseRunStep) error {
	b, err := json.Marshal(steps)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `UPDATE course_runs SET steps=$2, updated_at=NOW() WHERE id=$1`, id, b)
	return err
}

func (r *courseRepository) SaveOutline(ctx context.Context, id uuid.UUID, outline *model.CourseOutline, sources []model.CourseSource) error {
	o, err := json.Marshal(outline)
	if err != nil {
		return err
	}
	if sources == nil {
		sources = []model.CourseSource{}
	}
	s, err := json.Marshal(sources)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `UPDATE course_runs SET outline=$2, sources=$3, updated_at=NOW() WHERE id=$1`, id, o, s)
	return err
}

func (r *courseRepository) SetCover(ctx context.Context, id uuid.UUID, url string) error {
	_, err := r.pool.Exec(ctx, `UPDATE course_runs SET cover_url=$2, updated_at=NOW() WHERE id=$1`, id, url)
	return err
}

func (r *courseRepository) AppendWarning(ctx context.Context, id uuid.UUID, warning string) error {
	b, err := json.Marshal([]string{warning})
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `UPDATE course_runs SET warnings = warnings || $2::jsonb, updated_at=NOW() WHERE id=$1`, id, b)
	return err
}

func (r *courseRepository) Transition(ctx context.Context, id uuid.UUID, status string, errMsg *string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE course_runs SET
			status=$2,
			error_message=COALESCE($3, error_message),
			completed_at=CASE WHEN $2 IN ('completed','failed','cancelled') THEN NOW() ELSE completed_at END,
			updated_at=NOW()
		WHERE id=$1 AND status NOT IN ('completed','failed','cancelled')`,
		id, status, errMsg)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *courseRepository) TouchHeartbeat(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE course_runs SET heartbeat_at=NOW() WHERE id=$1 AND status='running'`, id)
	return err
}

func (r *courseRepository) ListStaleRunning(ctx context.Context, olderThan time.Time) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id FROM course_runs
		WHERE status IN ('queued','running') AND COALESCE(heartbeat_at, started_at, created_at) < $1`, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *courseRepository) UpsertChapter(ctx context.Context, ch *model.CourseChapter) error {
	if ch.ID == uuid.Nil {
		ch.ID = uuid.New()
	}
	objectives, err := json.Marshal(nonNil(ch.Objectives))
	if err != nil {
		return err
	}
	images, err := json.Marshal(nonNilImages(ch.Images))
	if err != nil {
		return err
	}
	var quiz []byte
	if ch.Quiz != nil {
		if quiz, err = json.Marshal(ch.Quiz); err != nil {
			return err
		}
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO course_chapters (id, run_id, locale, position, title, summary, objectives,
			markdown, html, images, quiz, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW(),NOW())
		ON CONFLICT (run_id, locale, position) DO UPDATE SET
			title=EXCLUDED.title, summary=EXCLUDED.summary, objectives=EXCLUDED.objectives,
			markdown=EXCLUDED.markdown, html=EXCLUDED.html, images=EXCLUDED.images,
			quiz=EXCLUDED.quiz, updated_at=NOW()
		RETURNING id, created_at, updated_at`,
		ch.ID, ch.RunID, ch.Locale, ch.Position, ch.Title, ch.Summary, objectives,
		ch.Markdown, ch.HTML, images, quiz,
	).Scan(&ch.ID, &ch.CreatedAt, &ch.UpdatedAt)
}

func (r *courseRepository) ListChapters(ctx context.Context, runID uuid.UUID) ([]model.CourseChapter, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, run_id, locale, position, title, summary, objectives, markdown, html,
			images, quiz, created_at, updated_at
		FROM course_chapters WHERE run_id=$1 ORDER BY locale, position`, runID)
	if err != nil {
		return nil, fmt.Errorf("list chapters: %w", err)
	}
	defer rows.Close()
	out := []model.CourseChapter{}
	for rows.Next() {
		var ch model.CourseChapter
		var objectives, images, quiz []byte
		if err := rows.Scan(&ch.ID, &ch.RunID, &ch.Locale, &ch.Position, &ch.Title, &ch.Summary,
			&objectives, &ch.Markdown, &ch.HTML, &images, &quiz, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(objectives, &ch.Objectives)
		_ = json.Unmarshal(images, &ch.Images)
		if len(quiz) > 0 {
			var q model.CourseQuiz
			if json.Unmarshal(quiz, &q) == nil {
				ch.Quiz = &q
			}
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilImages(s []model.CourseImage) []model.CourseImage {
	if s == nil {
		return []model.CourseImage{}
	}
	return s
}
