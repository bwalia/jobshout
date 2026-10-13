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

// videoPG runs migration 068 in a throwaway schema of a real Postgres:
//
//	VIDEO_TEST_DATABASE_URL=postgres://postgres:pw@localhost:5432/postgres go test ./internal/repository -run VideoPG
func videoPG(t *testing.T) (VideoRunRepository, *pgxpool.Pool, uuid.UUID) {
	t.Helper()
	dsn := os.Getenv("VIDEO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VIDEO_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "video_test_" + uuid.NewString()[:8]
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		pool.Close()
	})
	migration, err := os.ReadFile("../../migrations/000068_video_runs.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE SCHEMA ` + schema,
		`CREATE TABLE organizations (id UUID PRIMARY KEY DEFAULT gen_random_uuid())`,
		string(migration),
		string(migration), // replayed on every boot: must be a no-op
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("%v\n%s", err, sql[:min(200, len(sql))])
		}
	}
	var org uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO organizations DEFAULT VALUES RETURNING id`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	return NewVideoRunRepository(pool), pool, org
}

func TestVideoPGRunLifecycle(t *testing.T) {
	repo, pool, org := videoPG(t)
	ctx := context.Background()
	task := uuid.New()

	run := &model.VideoRun{ID: uuid.New(), OrgID: org, TaskID: &task, Topic: "tides",
		Brief: model.VideoBrief{Seconds: 30, Format: "portrait", Motion: true}, Status: model.VideoRunScripting,
		Stage: "script", Message: "Writing the script", EstimateMinutes: 58}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	if run.CreatedAt.IsZero() {
		t.Fatal("created_at not returned")
	}

	got, err := repo.Get(ctx, org, run.ID)
	if err != nil || got.Brief.Format != "portrait" || !got.Brief.Motion || got.Shots == nil || got.Notes == nil {
		t.Fatalf("get after create: %+v %v", got, err)
	}
	if _, err := repo.Get(ctx, uuid.New(), run.ID); !errors.Is(err, ErrVideoRunNotFound) {
		t.Fatalf("another org read it: %v", err)
	}
	if open, _ := repo.UnfinishedForTask(ctx, task); len(open) != 1 {
		t.Fatalf("unfinished for task: %d", len(open))
	}

	now := time.Now()
	run.Title, run.Shots = "Tides", []model.VideoShot{{Narration: "The sea breathes.", Visual: "A tidal flat"}}
	run.Status, run.Stage, run.Progress, run.Message, run.StartedAt = model.VideoRunRendering, "motion", 0.42, "Animating shot 3 of 5", &now
	if err := repo.Save(ctx, run); err != nil {
		t.Fatal(err)
	}
	if rendering, _ := repo.Rendering(ctx); len(rendering) != 1 || rendering[0].Stage != "motion" {
		t.Fatalf("rendering: %+v", rendering)
	}
	got, _ = repo.Get(ctx, org, run.ID)
	if got.Title != "Tides" || len(got.Shots) != 1 || got.Progress < 0.41 || got.StartedAt == nil {
		t.Fatalf("after save: %+v", got)
	}

	run.Status, run.VideoURL, run.Notes, run.FinishedAt = model.VideoRunDone, "/api/v1/images/file/x.mp4", []string{"shot 2 used a still"}, &now
	if err := repo.Save(ctx, run); err != nil {
		t.Fatal(err)
	}
	if rendering, _ := repo.Rendering(ctx); len(rendering) != 0 {
		t.Fatal("a finished run is still listed as rendering")
	}
	list, err := repo.List(ctx, org, 10)
	if err != nil || len(list) != 1 || list[0].VideoURL == "" || len(list[0].Notes) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}

	// A run cut off while writing its script is failed only once stale.
	stale := &model.VideoRun{ID: uuid.New(), OrgID: org, Status: model.VideoRunScripting, Stage: "script"}
	if err := repo.Create(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.FailStaleScripting(ctx, time.Now().Add(-time.Hour), "interrupted"); n != 0 {
		t.Fatalf("failed a fresh scripting run")
	}
	if _, err := pool.Exec(ctx, `UPDATE video_runs SET updated_at = NOW() - INTERVAL '2 hours' WHERE id = $1`, stale.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.FailStaleScripting(ctx, time.Now().Add(-time.Hour), "interrupted"); n != 1 {
		t.Fatalf("stale scripting run not failed: %d", n)
	}
	got, _ = repo.Get(ctx, org, stale.ID)
	if got.Status != model.VideoRunFailed || got.Error != "interrupted" || got.FinishedAt == nil {
		t.Fatalf("stale run: %+v", got)
	}
}
