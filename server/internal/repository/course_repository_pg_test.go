package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// coursePG runs the course migrations in a throwaway schema of a real
// Postgres. The statements here are guarded updates and JSON rewrites that a
// fake cannot vouch for, so they are exercised against the real thing:
//
//	COURSE_TEST_DATABASE_URL=postgres://postgres:pw@localhost:5432/postgres go test ./internal/repository -run CoursePG
func coursePG(t *testing.T) (*pgxpool.Pool, uuid.UUID, uuid.UUID) {
	t.Helper()
	dsn := os.Getenv("COURSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("COURSE_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "course_test_" + uuid.NewString()[:8]
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		pool.Close()
	})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("%v\n%s", err, sql[:min(200, len(sql))])
		}
	}
	// Just enough of the tables migration 053 refers to.
	exec(`CREATE SCHEMA ` + schema)
	exec(`CREATE TABLE organizations (id UUID PRIMARY KEY DEFAULT gen_random_uuid());
		CREATE TABLE tasks (id UUID PRIMARY KEY DEFAULT gen_random_uuid());
		CREATE TABLE agents (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), org_id UUID, name TEXT, role TEXT,
			description TEXT, status TEXT, engine_type TEXT, system_prompt TEXT, metadata JSONB);
		INSERT INTO organizations DEFAULT VALUES`)
	migrate := func(name string) {
		t.Helper()
		b, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		exec(string(b))
	}
	migrate("000053_course_generator.up.sql")
	migrate("000057_course_run_resume.up.sql")

	var org, agent uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT org_id, id FROM agents LIMIT 1`).Scan(&org, &agent); err != nil {
		t.Fatal(err)
	}
	// Migrations replay on every boot, with live rows in every status.
	exec(`INSERT INTO course_runs (agent_id, org_id, status) VALUES ('` + agent.String() + `','` + org.String() + `','resuming')`)
	migrate("000053_course_generator.up.sql")
	migrate("000057_course_run_resume.up.sql")
	exec(`DELETE FROM course_runs`)
	return pool, org, agent
}

func TestCoursePG_AttemptGuardedLifecycle(t *testing.T) {
	pool, org, agent := coursePG(t)
	repo := NewCourseRepository(pool)
	ctx := context.Background()

	now := time.Now()
	run := &model.CourseRun{
		ID: uuid.New(), AgentID: agent, OrgID: org, Status: model.CourseRunRunning,
		Brief: model.CourseBrief{Topic: "Go", ChapterCount: 2, Locale: "en"},
		Steps: []model.CourseRunStep{
			{Key: "researching", Status: "done"}, {Key: "outlining", Status: "running", Detail: "d"}, {Key: "writing", Status: "pending"},
		},
		HeartbeatAt: &now, StartedAt: &now,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	// Everything an attempt saves comes back.
	outline := &model.CourseOutline{Title: "Go", Chapters: []model.CourseOutlineChapter{{Title: "A"}, {Title: "B"}}}
	progress := &model.CourseProgress{CoverDone: true, Draft: &model.CourseChapterDraft{Position: 2, Markdown: "## x", Reviewed: true}}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(repo.SaveResearch(ctx, run.ID, 1, "notes", []model.CourseSource{{URL: "https://a"}}))
	must(repo.SaveOutline(ctx, run.ID, 1, outline, nil))
	must(repo.SetCover(ctx, run.ID, 1, "/c.png"))
	must(repo.SaveProgress(ctx, run.ID, 1, progress))
	must(repo.UpdateSteps(ctx, run.ID, 1, run.Steps))
	got, err := repo.GetRun(ctx, run.ID)
	must(err)
	if got.Attempt != 1 || got.ResearchNotes != "notes" || got.Outline == nil || len(got.Outline.Chapters) != 2 ||
		got.CoverURL != "/c.png" || got.Progress == nil || !got.Progress.CoverDone ||
		got.Progress.Draft == nil || got.Progress.Draft.Position != 2 || !got.Progress.Draft.Reviewed {
		t.Fatalf("saved state did not round-trip: %+v progress=%+v", got, got.Progress)
	}
	// A fresh heartbeat is not resumable; a run handed back is.
	if refs, err := repo.ListResumable(ctx, time.Now().Add(-time.Hour)); err != nil || len(refs) != 0 {
		t.Fatalf("fresh running run listed as resumable: %v %v", refs, err)
	}
	if ok, err := repo.Release(ctx, run.ID, 1); err != nil || !ok {
		t.Fatalf("Release = %v, %v", ok, err)
	}
	if err := repo.SaveProgress(ctx, run.ID, 1, progress); !errors.Is(err, ErrCourseRunLost) {
		t.Fatalf("a write after hand-back = %v, want ErrCourseRunLost", err)
	}
	if ok, _ := repo.TouchHeartbeat(ctx, run.ID, 1); ok {
		t.Fatal("heartbeat after hand-back must report the run lost")
	}
	refs, err := repo.ListResumable(ctx, time.Now().Add(-time.Hour))
	if err != nil || len(refs) != 1 || refs[0] != (CourseRunRef{ID: run.ID, Attempt: 1}) {
		t.Fatalf("ListResumable = %v, %v", refs, err)
	}

	// One claim wins; the state travels with it.
	claimed, err := repo.Claim(ctx, refs[0])
	must(err)
	if claimed == nil || claimed.Attempt != 2 || claimed.Status != model.CourseRunRunning || claimed.ResearchNotes != "notes" || claimed.Progress == nil {
		t.Fatalf("Claim = %+v", claimed)
	}
	if again, err := repo.Claim(ctx, refs[0]); err != nil || again != nil {
		t.Fatalf("second claim of the same ref = %+v, %v; want nil", again, err)
	}
	// The superseded attempt cannot write or finish; the new one can.
	if err := repo.SaveOutline(ctx, run.ID, 1, outline, nil); !errors.Is(err, ErrCourseRunLost) {
		t.Fatalf("superseded attempt wrote: %v", err)
	}
	if ok, _ := repo.Finish(ctx, run.ID, 1, model.CourseRunFailed, nil); ok {
		t.Fatal("superseded attempt finished the run")
	}
	must(repo.SaveProgress(ctx, run.ID, 2, progress))

	// A killed pod: still running, heartbeat gone stale.
	if _, err := pool.Exec(ctx, `UPDATE course_runs SET heartbeat_at = NOW() - INTERVAL '1 hour' WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	if refs, _ := repo.ListResumable(ctx, time.Now().Add(-time.Minute)); len(refs) != 1 || refs[0].Attempt != 2 {
		t.Fatalf("stale running run not listed: %v", refs)
	}

	// Chapters are keyed by position: saving one twice leaves one row.
	for range 2 {
		must(repo.UpsertChapter(ctx, &model.CourseChapter{RunID: run.ID, Locale: "en", Position: 1, Title: "A"}))
	}
	if chs, _ := repo.ListChapters(ctx, run.ID); len(chs) != 1 {
		t.Fatalf("chapters = %d, want 1", len(chs))
	}

	// Failing closes the step list, in order.
	msg := "boom"
	if ok, err := repo.Finish(ctx, run.ID, 2, model.CourseRunFailed, &msg); err != nil || !ok {
		t.Fatalf("Finish = %v, %v", ok, err)
	}
	got, _ = repo.GetRun(ctx, run.ID)
	want := []model.CourseRunStep{{Key: "researching", Status: "done"}, {Key: "outlining", Status: "failed", Detail: "d"}, {Key: "writing", Status: "skipped"}}
	if got.Status != model.CourseRunFailed || got.CompletedAt == nil || got.ErrorMessage == nil || *got.ErrorMessage != "boom" || len(got.Steps) != 3 {
		t.Fatalf("failed run = %+v", got)
	}
	for i := range want {
		if got.Steps[i] != want[i] {
			t.Errorf("step %d = %+v, want %+v", i, got.Steps[i], want[i])
		}
	}
	if c, _ := repo.Claim(ctx, CourseRunRef{ID: run.ID, Attempt: 2}); c != nil {
		t.Fatal("a failed run must not be claimable")
	}
	if refs, _ := repo.ListResumable(ctx, time.Now()); len(refs) != 0 {
		t.Fatalf("a failed run must not be resumable: %v", refs)
	}
}

func TestCoursePG_TransitionAndCompletion(t *testing.T) {
	pool, org, agent := coursePG(t)
	repo := NewCourseRepository(pool)
	ctx := context.Background()
	steps := []model.CourseRunStep{{Key: "researching", Status: "running"}, {Key: "outlining", Status: "pending"}}
	newRun := func() uuid.UUID {
		now := time.Now()
		run := &model.CourseRun{ID: uuid.New(), AgentID: agent, OrgID: org, Status: model.CourseRunRunning, Steps: steps, HeartbeatAt: &now}
		if err := repo.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		return run.ID
	}

	// Cancelling a run that is waiting to resume ends it and closes its steps.
	id := newRun()
	_, _ = repo.Release(ctx, id, 1)
	msg := "cancelled by user"
	if ok, err := repo.Transition(ctx, id, model.CourseRunCancelled, &msg); err != nil || !ok {
		t.Fatalf("Transition = %v, %v", ok, err)
	}
	got, _ := repo.GetRun(ctx, id)
	if got.Status != model.CourseRunCancelled || got.Steps[0].Status != "failed" || got.Steps[1].Status != "skipped" {
		t.Fatalf("cancelled run = %s %+v", got.Status, got.Steps)
	}
	if ok, _ := repo.Transition(ctx, id, model.CourseRunFailed, nil); ok {
		t.Fatal("a terminal run must not transition again")
	}

	// Completing keeps the step list the run wrote.
	id = newRun()
	done := []model.CourseRunStep{{Key: "researching", Status: "done"}, {Key: "outlining", Status: "done"}}
	if err := repo.UpdateSteps(ctx, id, 1, done); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.Finish(ctx, id, 1, model.CourseRunCompleted, nil); err != nil || !ok {
		t.Fatalf("Finish = %v, %v", ok, err)
	}
	got, _ = repo.GetRun(ctx, id)
	if got.Status != model.CourseRunCompleted || got.ErrorMessage != nil || got.Steps[0].Status != "done" || got.Steps[1].Status != "done" {
		t.Fatalf("completed run = %s %v %+v", got.Status, got.ErrorMessage, got.Steps)
	}

	// A run with no steps recorded still ends cleanly.
	if _, err := pool.Exec(ctx, `INSERT INTO course_runs (id, agent_id, org_id, status) VALUES ($1,$2,$3,'running')`, uuid.New(), agent, org); err != nil {
		t.Fatal(err)
	}
	refs, _ := repo.ListResumable(ctx, time.Now().Add(time.Minute))
	for _, ref := range refs {
		if ok, err := repo.Transition(ctx, ref.ID, model.CourseRunFailed, &msg); err != nil || !ok {
			t.Fatalf("Transition of a step-less run = %v, %v", ok, err)
		}
	}
}
