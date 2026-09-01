// Package scheduler provides a background cron dispatcher that executes
// due model.ScheduledTask rows. Tasks of type "blog" trigger the article
// pipeline — including runs that discover their own trending topic — and
// everything else routes to the existing WorkflowService/ExecutionService
// paths.
//
// The dispatcher ticks every 30s. That cadence matches typical "run every
// hour / run every Monday" schedules with negligible overhead, without
// requiring a full-cron-engine design.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
)

// TickInterval is how often the dispatcher sweeps for due tasks.
const TickInterval = 30 * time.Second

// Runner is the cron dispatcher. It is started from main.go and runs
// until its context is cancelled.
type Runner struct {
	repo        repository.SchedulerRepository
	blogSvc     service.BlogService
	workflows   service.WorkflowService
	execs       service.ExecutionService
	multiAgents service.MultiAgentService
	career      service.CareerService
	parser      cron.Parser
	logger      *zap.Logger
}

// NewRunner wires the Runner. blogSvc / multiAgents may be nil — tasks of
// those types will be skipped with a clear warning rather than crashing.
func NewRunner(
	repo repository.SchedulerRepository,
	blogSvc service.BlogService,
	workflows service.WorkflowService,
	execs service.ExecutionService,
	multiAgents service.MultiAgentService,
	logger *zap.Logger,
) *Runner {
	return &Runner{
		repo:        repo,
		blogSvc:     blogSvc,
		workflows:   workflows,
		execs:       execs,
		multiAgents: multiAgents,
		// Standard 5-field cron spec ("0 9 * * 1") is what users know from
		// crontabs; the library also supports descriptors like @daily.
		parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
		logger: logger,
	}
}

// WithCareer enables scheduled portal scans (task_type career_scan).
func (r *Runner) WithCareer(svc service.CareerService) *Runner {
	r.career = svc
	return r
}

// Start blocks until ctx is cancelled. Usually launched with `go runner.Start(ctx)`.
func (r *Runner) Start(ctx context.Context) {
	r.logger.Info("scheduler: runner started", zap.Duration("tick", TickInterval))
	t := time.NewTicker(TickInterval)
	defer t.Stop()

	// Fire immediately once so freshly-due tasks don't wait a full tick.
	r.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("scheduler: runner stopping", zap.Error(ctx.Err()))
			return
		case <-t.C:
			r.tick(ctx)
		}
	}
}

func (r *Runner) tick(ctx context.Context) {
	tasks, err := r.repo.ListDueTasks(ctx)
	if err != nil {
		r.logger.Error("scheduler: list due tasks failed", zap.Error(err))
		return
	}
	if len(tasks) == 0 {
		return
	}

	r.logger.Info("scheduler: dispatching due tasks", zap.Int("count", len(tasks)))
	for i := range tasks {
		// Spawn per-task so a slow LLM call can't block the tick loop.
		go r.runOne(ctx, tasks[i])
	}
}

func (r *Runner) runOne(ctx context.Context, t model.ScheduledTask) {
	log := r.logger.With(
		zap.String("task_id", t.ID.String()),
		zap.String("name", t.Name),
	)
	log.Info("scheduler: running scheduled task")

	runRec := &model.ScheduledTaskRun{
		ID:              uuid.New(),
		ScheduledTaskID: t.ID,
		Status:          "running",
	}
	started := time.Now()
	runRec.StartedAt = &started

	var err error
	switch {
	case isBlogTask(t):
		err = r.dispatchBlog(ctx, t, runRec)
	case t.TaskType == "career_scan":
		err = r.dispatchCareerScan(ctx, t)
	case t.TaskType == "workflow" && t.WorkflowID != nil:
		err = r.dispatchWorkflow(ctx, t, runRec)
	case t.TaskType == "multi_agent":
		err = r.dispatchMultiAgent(ctx, t, runRec)
	case t.TaskType == "agent" && t.AgentID != nil:
		err = r.dispatchAgent(ctx, t, runRec)
	default:
		err = fmt.Errorf("scheduler: task %q has no dispatchable target", t.Name)
	}

	completed := time.Now()
	runRec.CompletedAt = &completed
	if err != nil {
		msg := err.Error()
		runRec.Status = "failed"
		runRec.ErrorMessage = &msg
		log.Error("scheduler: task failed", zap.Error(err))
	} else {
		runRec.Status = "completed"
	}
	if err := r.repo.CreateRun(ctx, runRec); err != nil {
		log.Error("scheduler: persist run failed", zap.Error(err))
	}

	// Advance the task: increment count + compute next_run_at.
	if err := r.repo.IncrementRunCount(ctx, t.ID); err != nil {
		log.Error("scheduler: increment run count failed", zap.Error(err))
	}
	r.scheduleNext(ctx, t, log)
}

func (r *Runner) scheduleNext(ctx context.Context, t model.ScheduledTask, log *zap.Logger) {
	// If the task exceeded max_runs, mark it completed so we stop picking it up.
	if t.MaxRuns != nil && t.RunCount+1 >= *t.MaxRuns {
		status := "completed"
		_, err := r.repo.UpdateTask(ctx, t.ID, model.UpdateScheduledTaskRequest{Status: &status})
		if err != nil {
			log.Error("scheduler: mark completed failed", zap.Error(err))
		}
		return
	}

	next, err := r.computeNextRun(t)
	if err != nil {
		log.Warn("scheduler: could not compute next run — pausing task",
			zap.Error(err))
		status := "paused"
		_, _ = r.repo.UpdateTask(ctx, t.ID, model.UpdateScheduledTaskRequest{Status: &status})
		return
	}
	if next == nil {
		// One-shot task — stop picking it up.
		status := "completed"
		_, _ = r.repo.UpdateTask(ctx, t.ID, model.UpdateScheduledTaskRequest{Status: &status})
		return
	}
	if err := r.repo.SetNextRunAt(ctx, t.ID, *next); err != nil {
		log.Error("scheduler: set next_run_at failed", zap.Error(err))
	}
}

func (r *Runner) computeNextRun(t model.ScheduledTask) (*time.Time, error) {
	now := time.Now()
	switch t.ScheduleType {
	case "cron":
		if t.CronExpression == nil || *t.CronExpression == "" {
			return nil, fmt.Errorf("cron schedule with empty expression")
		}
		sched, err := r.parser.Parse(*t.CronExpression)
		if err != nil {
			return nil, fmt.Errorf("parse cron %q: %w", *t.CronExpression, err)
		}
		next := sched.Next(now)
		return &next, nil
	case "interval":
		if t.IntervalSeconds == nil || *t.IntervalSeconds <= 0 {
			return nil, fmt.Errorf("interval schedule with non-positive interval")
		}
		next := now.Add(time.Duration(*t.IntervalSeconds) * time.Second)
		return &next, nil
	case "once":
		// One-shot — no next run.
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown schedule_type %q", t.ScheduleType)
	}
}

// dispatch helpers.

func (r *Runner) dispatchBlog(ctx context.Context, t model.ScheduledTask, runRec *model.ScheduledTaskRun) error {
	req, err := blogRequestFromInput(t.InputJSON)
	if err != nil {
		return err
	}
	run, err := r.blogSvc.Generate(ctx, t.OrgID, t.CreatedBy, "schedule", req)
	if err != nil {
		return err
	}
	runRec.WorkflowRunID = nil
	// Stash the blog run ID in Output so the ScheduledTaskRun is traceable.
	// Only the ID: Generate returns as soon as the run is queued, so anything
	// the pipeline produces later is not knowable here.
	out := fmt.Sprintf(`{"blog_run_id":%q}`, run.ID.String())
	runRec.Output = &out
	return nil
}

func (r *Runner) dispatchWorkflow(ctx context.Context, t model.ScheduledTask, runRec *model.ScheduledTaskRun) error {
	triggered := uuid.Nil
	if t.CreatedBy != nil {
		triggered = *t.CreatedBy
	}
	wfRun, err := r.workflows.Execute(ctx, *t.WorkflowID, t.OrgID, triggered, model.ExecuteWorkflowRequest{
		Input: t.InputJSON,
	})
	if err != nil {
		return err
	}
	runRec.WorkflowRunID = &wfRun.ID
	return nil
}

func (r *Runner) dispatchAgent(ctx context.Context, t model.ScheduledTask, runRec *model.ScheduledTaskRun) error {
	exec, err := r.execs.Execute(ctx, t.OrgID, *t.AgentID, model.ExecuteAgentRequest{
		Prompt: t.InputPrompt,
	})
	if err != nil {
		return err
	}
	runRec.ExecutionID = &exec.ID
	return nil
}

func (r *Runner) dispatchMultiAgent(ctx context.Context, t model.ScheduledTask, runRec *model.ScheduledTaskRun) error {
	if r.multiAgents == nil {
		return fmt.Errorf("scheduler: multi-agent service not configured")
	}
	if t.PlannerID == nil || t.ExecutorID == nil || t.ReviewerID == nil {
		return fmt.Errorf("scheduler: multi-agent task missing planner/executor/reviewer")
	}

	job, err := r.multiAgents.RunJobSync(ctx, t.OrgID, model.RunMultiAgentRequest{
		TaskPrompt: t.InputPrompt,
		PlannerID:  *t.PlannerID,
		ExecutorID: *t.ExecutorID,
		ReviewerID: *t.ReviewerID,
		MaxReview:  t.MaxReview,
	})
	if err != nil {
		return err
	}
	runRec.MultiAgentJobID = &job.ID
	if job.Status == model.MultiAgentStatusFailed && job.ErrorMsg != nil {
		// Surface the underlying failure to the run record so operators don't
		// have to chase the job ID to see why a scheduled run failed.
		return fmt.Errorf("multi-agent job failed: %s", *job.ErrorMsg)
	}
	return nil
}

func (r *Runner) dispatchCareerScan(ctx context.Context, t model.ScheduledTask) error {
	if r.career == nil {
		return fmt.Errorf("scheduler: career scan is not configured")
	}
	if t.CreatedBy == nil || *t.CreatedBy == uuid.Nil {
		return fmt.Errorf("scheduler: career scan needs the user who created the schedule")
	}
	req := model.ScanCareerRequest{}
	if t.InputJSON != nil {
		if s, ok := t.InputJSON["board"].(string); ok {
			req.Board = s
		}
		if s, ok := t.InputJSON["slug"].(string); ok {
			req.Slug = s
		}
		if s, ok := t.InputJSON["company"].(string); ok {
			req.Company = s
		}
		if s, ok := t.InputJSON["query"].(string); ok {
			req.Query = s
		}
	}
	_, err := r.career.Scan(ctx, t.OrgID, *t.CreatedBy, req)
	return err
}

func isBlogTask(t model.ScheduledTask) bool {
	// The task type is the current way to say this. The tag and the input
	// marker below predate it and are still honoured, because tasks created
	// that way are sitting in the database and must keep firing.
	if t.TaskType == "blog" {
		return true
	}
	if len(t.Tags) > 0 {
		for _, tag := range t.Tags {
			if tag == "blog" {
				return true
			}
		}
	}
	// Also accept a "kind":"blog" marker in input_json for users who don't use tags.
	if kind, ok := t.InputJSON["kind"].(string); ok && kind == "blog" {
		return true
	}
	return false
}

func blogRequestFromInput(in map[string]any) (model.GenerateBlogRequest, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return model.GenerateBlogRequest{}, fmt.Errorf("marshal input: %w", err)
	}
	var req model.GenerateBlogRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return model.GenerateBlogRequest{}, fmt.Errorf("decode blog input: %w", err)
	}
	// Tasks stored before briefs existed carry a bare topics array; Normalize
	// folds either shape into briefs so a schedule created months ago keeps
	// firing without being rewritten.
	req.Normalize()
	if err := req.Validate(); err != nil {
		return model.GenerateBlogRequest{}, fmt.Errorf("scheduled blog task: %w", err)
	}
	return req, nil
}
