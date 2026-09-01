package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/reviewbot"
)

type ReviewBotClient interface {
	Start(ctx context.Context, req reviewbot.StartRequest) (*reviewbot.Handle, error)
	Status(ctx context.Context, jobID string) (*reviewbot.Snapshot, error)
	Enabled() bool
}

type ReviewReconciler struct {
	runRepo         repository.ReviewRunRepository
	client          ReviewBotClient
	interval        time.Duration
	maxRuntime      time.Duration
	batchSize       int
	backoff         time.Duration
	maxPollAttempts int
	logger          *zap.Logger
	tasks           TaskService
}

func NewReviewReconciler(
	runRepo repository.ReviewRunRepository,
	client ReviewBotClient,
	cfg reviewbot.Config,
	logger *zap.Logger,
) *ReviewReconciler {
	interval := cfg.PollInterval
	if interval <= 0 {
		interval = reviewbot.DefaultPollInterval
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReviewReconciler{
		runRepo:         runRepo,
		client:          client,
		interval:        interval,
		maxRuntime:      cfg.MaxRuntime,
		batchSize:       10,
		backoff:         30 * time.Second,
		maxPollAttempts: 20,
		logger:          logger,
	}
}

func (rc *ReviewReconciler) BindTasks(tasks TaskService) {
	rc.tasks = tasks
}

func (rc *ReviewReconciler) Start(ctx context.Context) {
	if rc.client == nil || !rc.client.Enabled() {
		rc.logger.Info("review reconciler not started: sidecar not configured")
		return
	}
	rc.logger.Info("review reconciler started", zap.Duration("interval", rc.interval))
	ticker := time.NewTicker(rc.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			rc.logger.Info("review reconciler stopped")
			return
		case <-ticker.C:
			if err := rc.Tick(ctx); err != nil {
				rc.logger.Warn("review reconciler tick failed", zap.Error(err))
			}
		}
	}
}

func (rc *ReviewReconciler) Tick(ctx context.Context) error {
	runs, err := rc.runRepo.ClaimDueRuns(ctx, rc.batchSize, rc.interval)
	if err != nil {
		return fmt.Errorf("claim due review runs: %w", err)
	}
	for i := range runs {
		run := &runs[i]
		if err := rc.advance(ctx, run); err != nil {
			rc.logger.Error("failed to advance review run",
				zap.String("runID", run.ID.String()), zap.Error(err))
		}
	}
	return nil
}

func (rc *ReviewReconciler) advance(ctx context.Context, run *model.ReviewRun) error {
	if rc.exceededRuntime(run) {
		return rc.fail(ctx, run, fmt.Sprintf("review exceeded the %s runtime limit", rc.maxRuntime))
	}
	if run.RemoteJobID == nil || *run.RemoteJobID == "" {
		return rc.startRun(ctx, run)
	}
	return rc.pollRun(ctx, run)
}

func (rc *ReviewReconciler) startRun(ctx context.Context, run *model.ReviewRun) error {
	handle, err := rc.client.Start(ctx, reviewbot.StartRequest{
		Repo:     run.Repo,
		PRNumber: run.PRNumber,
		DryRun:   run.DryRun,
		Force:    run.Force,
		RunRef:   run.ID.String(),
	})
	if err != nil {
		return rc.handleStartError(ctx, run, err)
	}
	id := handle.JobID
	run.RemoteJobID = &id
	run.Status = "running"
	if run.StartedAt == nil {
		now := time.Now()
		run.StartedAt = &now
	}
	run.PollAttempts = 0
	next := time.Now()
	run.NextPollAt = &next
	return rc.runRepo.Update(ctx, run)
}

func (rc *ReviewReconciler) handleStartError(ctx context.Context, run *model.ReviewRun, err error) error {
	switch {
	case errors.Is(err, reviewbot.ErrNotAllowed):
		return rc.fail(ctx, run, err.Error())
	case errors.Is(err, reviewbot.ErrUnauthorized):
		return rc.fail(ctx, run, err.Error())
	default:
		run.PollAttempts++
		if run.PollAttempts >= rc.maxPollAttempts {
			return rc.fail(ctx, run, fmt.Sprintf("review sidecar unreachable after %d attempts: %v", run.PollAttempts, err))
		}
		next := time.Now().Add(rc.backoff)
		run.NextPollAt = &next
		return rc.runRepo.Update(ctx, run)
	}
}

func (rc *ReviewReconciler) pollRun(ctx context.Context, run *model.ReviewRun) error {
	snap, err := rc.client.Status(ctx, *run.RemoteJobID)
	if err != nil {
		if errors.Is(err, reviewbot.ErrJobNotFound) {
			return rc.fail(ctx, run, "sidecar forgot this job (it may have restarted). Start the review again.")
		}
		run.PollAttempts++
		next := time.Now().Add(rc.backoff)
		run.NextPollAt = &next
		return rc.runRepo.Update(ctx, run)
	}
	run.PollAttempts = 0
	run.StageLog = snap.StageLog
	if !snap.Terminal() {
		next := time.Now().Add(rc.interval)
		run.NextPollAt = &next
		return rc.runRepo.Update(ctx, run)
	}
	return rc.finalize(ctx, run, snap)
}

func (rc *ReviewReconciler) finalize(ctx context.Context, run *model.ReviewRun, snap *reviewbot.Snapshot) error {
	now := time.Now()
	run.CompletedAt = &now
	run.NextPollAt = nil
	run.StageLog = snap.StageLog
	if snap.State == "failed" {
		msg := snap.Error
		if msg == "" {
			msg = "review failed"
		}
		return rc.fail(ctx, run, msg)
	}
	run.Status = "completed"
	if len(snap.Result) > 0 {
		run.Result = snap.Result
		applyReviewResult(run, snap.Result)
	}
	if snap.Error != "" {
		msg := snap.Error
		run.ErrorMessage = &msg
	}
	rc.logger.Info("review run completed",
		zap.String("runID", run.ID.String()),
		zap.Stringp("decision", run.Decision))
	if err := rc.runRepo.Update(ctx, run); err != nil {
		return err
	}
	syncSpecialistBoard(ctx, rc.tasks, run.TaskID, run.Status)
	return nil
}

func applyReviewResult(run *model.ReviewRun, raw json.RawMessage) {
	var parsed struct {
		HeadSHA  string `json:"head_sha"`
		Decision string `json:"decision"`
		Verdict  string `json:"verdict"`
		Summary  string `json:"summary"`
		URL      string `json:"url"`
	}
	if json.Unmarshal(raw, &parsed) != nil {
		return
	}
	if parsed.HeadSHA != "" {
		run.HeadSHA = &parsed.HeadSHA
	}
	if parsed.Decision != "" {
		run.Decision = &parsed.Decision
	}
	if parsed.Verdict != "" {
		run.Verdict = &parsed.Verdict
	}
	if parsed.Summary != "" {
		run.Summary = &parsed.Summary
	}
	if parsed.URL != "" {
		run.GitHubURL = &parsed.URL
	}
}

func (rc *ReviewReconciler) fail(ctx context.Context, run *model.ReviewRun, msg string) error {
	now := time.Now()
	run.Status = "failed"
	run.ErrorMessage = &msg
	run.CompletedAt = &now
	run.NextPollAt = nil
	if err := rc.runRepo.Update(ctx, run); err != nil {
		return err
	}
	syncSpecialistBoard(ctx, rc.tasks, run.TaskID, run.Status)
	return nil
}

func (rc *ReviewReconciler) exceededRuntime(run *model.ReviewRun) bool {
	if rc.maxRuntime <= 0 {
		return false
	}
	origin := run.CreatedAt
	if run.StartedAt != nil {
		origin = *run.StartedAt
	}
	return time.Since(origin) > rc.maxRuntime
}

var _ ReviewBotClient = (*reviewbot.Client)(nil)
