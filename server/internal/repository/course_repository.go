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

// ErrCourseRunLost is returned by an attempt's write when the run is no longer
// that attempt's: it was resumed elsewhere, or it reached a terminal status.
var ErrCourseRunLost = errors.New("course run is no longer owned by this attempt")

// CourseRunRef names a run together with the attempt last seen on it.
type CourseRunRef struct {
	ID      uuid.UUID
	Attempt int
}

// CourseRepository persists Course Generator runs and their chapters.
//
// Methods that take an attempt are the generating goroutine's writes. They
// change the row only while it is running on that attempt and return
// ErrCourseRunLost otherwise, so a goroutine left over from an earlier attempt
// (on this pod or another replica) cannot write onto the current one.
type CourseRepository interface {
	CreateRun(ctx context.Context, run *model.CourseRun) error
	GetRun(ctx context.Context, id uuid.UUID) (*model.CourseRun, error)
	ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CourseRun], error)
	UpdateSteps(ctx context.Context, id uuid.UUID, attempt int, steps []model.CourseRunStep) error
	SaveResearch(ctx context.Context, id uuid.UUID, attempt int, notes string, sources []model.CourseSource) error
	SaveOutline(ctx context.Context, id uuid.UUID, attempt int, outline *model.CourseOutline, sources []model.CourseSource) error
	SetCover(ctx context.Context, id uuid.UUID, attempt int, url string) error
	SaveProgress(ctx context.Context, id uuid.UUID, attempt int, progress *model.CourseProgress) error
	AppendWarning(ctx context.Context, id uuid.UUID, warning string) error
	// Finish moves the attempt's run to a terminal status. It reports whether
	// the row changed.
	Finish(ctx context.Context, id uuid.UUID, attempt int, status string, errMsg *string) (bool, error)
	// TouchHeartbeat reports whether the attempt still owns the run.
	TouchHeartbeat(ctx context.Context, id uuid.UUID, attempt int) (bool, error)

	// Transition moves a run to a status whatever its attempt. It only
	// changes rows that are not already terminal, and reports whether a row
	// changed. It also closes the step list when the status is terminal.
	Transition(ctx context.Context, id uuid.UUID, status string, errMsg *string) (bool, error)
	// Release hands the attempt's running run back to be resumed.
	Release(ctx context.Context, id uuid.UUID, attempt int) (bool, error)
	// ListResumable returns runs waiting to be resumed, and running or queued
	// runs whose heartbeat is older than staleBefore.
	ListResumable(ctx context.Context, staleBefore time.Time) ([]CourseRunRef, error)
	// Claim starts the next attempt of a run last seen on ref.Attempt. It
	// returns nil when another server claimed it first or the run has ended.
	Claim(ctx context.Context, ref CourseRunRef) (*model.CourseRun, error)

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
	completed_at, created_at, updated_at, attempt, COALESCE(research_notes, ''), progress`

func scanCourseRun(row pgx.Row) (*model.CourseRun, error) {
	run := &model.CourseRun{}
	var brief, outline, steps, sources, warnings, progress []byte
	if err := row.Scan(
		&run.ID, &run.AgentID, &run.TaskID, &run.OrgID, &run.Status, &brief, &outline,
		&steps, &run.CoverURL, &sources, &warnings, &run.ErrorMessage, &run.RequestedBy,
		&run.HeartbeatAt, &run.StartedAt, &run.CompletedAt, &run.CreatedAt, &run.UpdatedAt,
		&run.Attempt, &run.ResearchNotes, &progress,
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
	if len(progress) > 0 {
		var p model.CourseProgress
		if json.Unmarshal(progress, &p) == nil {
			run.Progress = &p
		}
	}
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
	if run.Attempt < 1 {
		run.Attempt = 1
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO course_runs (id, agent_id, task_id, org_id, status, brief, steps,
			requested_by, heartbeat_at, started_at, attempt, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW(),NOW())
		RETURNING created_at, updated_at`,
		run.ID, run.AgentID, run.TaskID, run.OrgID, run.Status, brief, steps,
		run.RequestedBy, run.HeartbeatAt, run.StartedAt, run.Attempt,
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

// owned is the guard on every write made by a generating attempt.
const courseOwned = ` WHERE id=$1 AND attempt=$2 AND status='running'`

// execOwned runs an attempt-guarded update whose first two arguments are the
// run id and the attempt.
func (r *courseRepository) execOwned(ctx context.Context, set string, args ...any) error {
	tag, err := r.pool.Exec(ctx, `UPDATE course_runs SET `+set+`, updated_at=NOW()`+courseOwned, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCourseRunLost
	}
	return nil
}

func (r *courseRepository) UpdateSteps(ctx context.Context, id uuid.UUID, attempt int, steps []model.CourseRunStep) error {
	b, err := json.Marshal(steps)
	if err != nil {
		return err
	}
	return r.execOwned(ctx, `steps=$3`, id, attempt, b)
}

func marshalCourseSources(sources []model.CourseSource) ([]byte, error) {
	if sources == nil {
		sources = []model.CourseSource{}
	}
	return json.Marshal(sources)
}

func (r *courseRepository) SaveResearch(ctx context.Context, id uuid.UUID, attempt int, notes string, sources []model.CourseSource) error {
	s, err := marshalCourseSources(sources)
	if err != nil {
		return err
	}
	return r.execOwned(ctx, `research_notes=$3, sources=$4`, id, attempt, notes, s)
}

func (r *courseRepository) SaveOutline(ctx context.Context, id uuid.UUID, attempt int, outline *model.CourseOutline, sources []model.CourseSource) error {
	o, err := json.Marshal(outline)
	if err != nil {
		return err
	}
	s, err := marshalCourseSources(sources)
	if err != nil {
		return err
	}
	return r.execOwned(ctx, `outline=$3, sources=$4`, id, attempt, o, s)
}

func (r *courseRepository) SetCover(ctx context.Context, id uuid.UUID, attempt int, url string) error {
	return r.execOwned(ctx, `cover_url=$3`, id, attempt, url)
}

func (r *courseRepository) SaveProgress(ctx context.Context, id uuid.UUID, attempt int, progress *model.CourseProgress) error {
	b, err := json.Marshal(progress)
	if err != nil {
		return err
	}
	return r.execOwned(ctx, `progress=$3`, id, attempt, b)
}

func (r *courseRepository) AppendWarning(ctx context.Context, id uuid.UUID, warning string) error {
	b, err := json.Marshal([]string{warning})
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `UPDATE course_runs SET warnings = warnings || $2::jsonb, updated_at=NOW() WHERE id=$1`, id, b)
	return err
}

// courseCloseSteps is the step list of a run that ended early: the running
// step failed and the ones after it never ran.
const courseCloseSteps = `COALESCE((
	SELECT jsonb_agg(CASE s->>'status'
		WHEN 'running' THEN jsonb_set(s, '{status}', '"failed"')
		WHEN 'pending' THEN jsonb_set(s, '{status}', '"skipped"')
		ELSE s END ORDER BY ord)
	FROM jsonb_array_elements(steps) WITH ORDINALITY AS t(s, ord)), '[]'::jsonb)`

// Finish ends the attempt's run. $3 is cast explicitly: it appears both as
// the varchar column value and in a comparison, and without the cast Postgres
// deduces two types for it and rejects the statement (SQLSTATE 42P08) — which
// left every finished run stuck "running".
func (r *courseRepository) Finish(ctx context.Context, id uuid.UUID, attempt int, status string, errMsg *string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE course_runs SET
			status=$3::varchar,
			error_message=$4,
			steps=CASE WHEN $3::varchar = 'completed' THEN steps ELSE `+courseCloseSteps+` END,
			completed_at=NOW(),
			updated_at=NOW()`+courseOwned,
		id, attempt, status, errMsg)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *courseRepository) TouchHeartbeat(ctx context.Context, id uuid.UUID, attempt int) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE course_runs SET heartbeat_at=NOW()`+courseOwned, id, attempt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// Transition moves a run to status unless it is already terminal, closing the
// step list when status is terminal. See Finish for the casts.
func (r *courseRepository) Transition(ctx context.Context, id uuid.UUID, status string, errMsg *string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE course_runs SET
			status=$2::varchar,
			error_message=COALESCE($3, error_message),
			steps=CASE WHEN $2::varchar IN ('failed','cancelled') THEN `+courseCloseSteps+` ELSE steps END,
			completed_at=CASE WHEN $2::varchar IN ('completed','failed','cancelled') THEN NOW() ELSE completed_at END,
			updated_at=NOW()
		WHERE id=$1 AND status NOT IN ('completed','failed','cancelled')`,
		id, status, errMsg)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *courseRepository) Release(ctx context.Context, id uuid.UUID, attempt int) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE course_runs SET status='resuming', updated_at=NOW()`+courseOwned, id, attempt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *courseRepository) ListResumable(ctx context.Context, staleBefore time.Time) ([]CourseRunRef, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, attempt FROM course_runs
		WHERE status = 'resuming'
		   OR (status IN ('queued','running') AND COALESCE(heartbeat_at, started_at, created_at) < $1)
		ORDER BY created_at`, staleBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []CourseRunRef
	for rows.Next() {
		var ref CourseRunRef
		if err := rows.Scan(&ref.ID, &ref.Attempt); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// Claim is atomic on attempt: of several servers that saw the same ref, one
// update matches and the rest get no row.
func (r *courseRepository) Claim(ctx context.Context, ref CourseRunRef) (*model.CourseRun, error) {
	run, err := scanCourseRun(r.pool.QueryRow(ctx, `
		UPDATE course_runs SET
			status='running', attempt=attempt+1, heartbeat_at=NOW(),
			error_message=NULL, updated_at=NOW()
		WHERE id=$1 AND attempt=$2 AND status IN ('queued','running','resuming')
		RETURNING `+courseRunColumns, ref.ID, ref.Attempt))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return run, err
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
