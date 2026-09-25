package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/course"
	"github.com/jobshout/server/internal/llmtrace"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ErrCourseRunNotFound is returned when a run is missing or belongs to another org.
var ErrCourseRunNotFound = errors.New("course run not found")

// ErrCourseNotCancellable is returned for terminal runs.
var ErrCourseNotCancellable = errors.New("course run cannot be cancelled")

const (
	courseHeartbeatInterval = 30 * time.Second
	courseReaperInterval    = 60 * time.Second
)

// CourseService is the launch and HTTP surface for the Course Generator.
type CourseService interface {
	CreateRun(ctx context.Context, req model.CreateCourseRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.CourseRun, error)
	GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.CourseRun, error)
	ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CourseRun], error)
	ListChapters(ctx context.Context, runID, orgID uuid.UUID) ([]model.CourseChapter, error)
	CancelRun(ctx context.Context, runID, orgID uuid.UUID) (*model.CourseRun, error)
	BindTasks(tasks TaskService)
	// ReapOrphans fails runs whose heartbeat stopped (pod killed mid-run).
	ReapOrphans(ctx context.Context) (int, error)
	// StartReaper ticks ReapOrphans until ctx ends.
	StartReaper(ctx context.Context)
	// InterruptAll cancels in-flight runs on shutdown.
	InterruptAll()
}

// courseGenerator is the slice of *course.Generator the service uses.
type courseGenerator interface {
	Ready() error
	Generate(ctx context.Context, job course.Job, h course.Hooks) error
}

type courseService struct {
	repo      repository.CourseRepository
	agentRepo repository.AgentRepository
	gen       courseGenerator
	cfg       course.Config
	logger    *zap.Logger

	tasks TaskService

	mu           sync.Mutex
	cancels      map[uuid.UUID]context.CancelCauseFunc
	shuttingDown bool
}

var (
	errCourseCancelled   = errors.New("cancelled by user")
	errCourseInterrupted = errors.New("interrupted: the server restarted during generation; launch the course again")
	errCourseTimedOut    = errors.New("timed out: the run exceeded its runtime budget (COURSE_MAX_RUNTIME per chapter)")
)

// NewCourseService wires the Course Generator.
func NewCourseService(repo repository.CourseRepository, agentRepo repository.AgentRepository, gen courseGenerator, cfg course.Config, logger *zap.Logger) CourseService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &courseService{
		repo:      repo,
		agentRepo: agentRepo,
		gen:       gen,
		cfg:       cfg,
		logger:    logger,
		cancels:   make(map[uuid.UUID]context.CancelCauseFunc),
	}
}

func (s *courseService) BindTasks(tasks TaskService) { s.tasks = tasks }

func (s *courseService) CreateRun(ctx context.Context, req model.CreateCourseRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.CourseRun, error) {
	if s.gen == nil {
		return nil, errors.New("Course Generator is not configured")
	}
	if err := s.gen.Ready(); err != nil {
		return nil, err
	}
	brief, err := course.NormalizeBrief(req.Brief, s.cfg.MaxChapters)
	if err != nil {
		return nil, err
	}
	agent, err := s.agentRepo.FindByID(ctx, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil || agent.OrgID != orgID || agent.SeededBuiltin() != model.BuiltinCourseGenerator {
		return nil, errors.New("agent not found")
	}

	now := time.Now()
	run := &model.CourseRun{
		ID:          uuid.New(),
		AgentID:     agent.ID,
		TaskID:      req.TaskID,
		OrgID:       orgID,
		Status:      model.CourseRunRunning,
		Brief:       brief,
		Steps:       initialCourseSteps(),
		RequestedBy: requestedBy,
		HeartbeatAt: &now,
		StartedAt:   &now,
	}

	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		return nil, errors.New("server is shutting down; try again shortly")
	}
	s.mu.Unlock()

	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	budget := s.cfg.RunBudget(brief.ChapterCount)
	runCtx, cancel := context.WithCancelCause(context.Background())
	runCtx, cancelTimeout := context.WithTimeoutCause(runCtx, budget, errCourseTimedOut)
	s.mu.Lock()
	s.cancels[run.ID] = cancel
	s.mu.Unlock()

	// The goroutine gets its own copy: the caller serialises run while
	// generation is updating outline and steps.
	work := *run
	work.Steps = append([]model.CourseRunStep(nil), run.Steps...)
	go func() {
		defer cancelTimeout()
		s.execute(runCtx, &work, agent.ID)
	}()
	return run, nil
}

func initialCourseSteps() []model.CourseRunStep {
	steps := make([]model.CourseRunStep, len(model.CourseStepOrder))
	for i, k := range model.CourseStepOrder {
		steps[i] = model.CourseRunStep{Key: k, Status: "pending"}
	}
	return steps
}

func (s *courseService) execute(ctx context.Context, run *model.CourseRun, agentID uuid.UUID) {
	defer func() {
		s.mu.Lock()
		delete(s.cancels, run.ID)
		s.mu.Unlock()
	}()
	ctx = llmtrace.WithTrace(ctx, llmtrace.TraceInfo{
		TraceName: "go-course-run",
		SessionID: run.ID.String(),
		AgentID:   agentID.String(),
		OrgID:     run.OrgID.String(),
	})
	log := s.logger.With(zap.String("course_run_id", run.ID.String()))

	stopHB := make(chan struct{})
	defer close(stopHB)
	go s.heartbeat(run.ID, stopHB)

	// Persistence uses a background context: a cancelled run must still be
	// able to record that it was cancelled.
	pctx := context.Background()
	tracker := &courseSteps{steps: run.Steps}
	chapters := 0

	err := s.gen.Generate(ctx, course.Job{OrgID: run.OrgID, UserID: run.RequestedBy, Brief: run.Brief}, course.Hooks{
		Step: func(key, detail string) {
			if err := s.repo.UpdateSteps(pctx, run.ID, tracker.advance(key, detail)); err != nil {
				log.Warn("course: persist steps", zap.Error(err))
			}
		},
		Planned: func(o *model.CourseOutline, sources []model.CourseSource) error {
			run.Outline = o
			return s.repo.SaveOutline(pctx, run.ID, o, sources)
		},
		Cover: func(url string) error { return s.repo.SetCover(pctx, run.ID, url) },
		Chapter: func(ch *model.CourseChapter) error {
			ch.RunID = run.ID
			if err := s.repo.UpsertChapter(pctx, ch); err != nil {
				return err
			}
			chapters++
			return nil
		},
		Warn: func(msg string) {
			log.Info("course: warning", zap.String("warning", msg))
			if err := s.repo.AppendWarning(pctx, run.ID, msg); err != nil {
				log.Warn("course: persist warning", zap.Error(err))
			}
		},
	})

	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			err = cause
		}
		msg := err.Error()
		_ = s.repo.UpdateSteps(pctx, run.ID, tracker.fail())
		status := model.CourseRunFailed
		if errors.Is(err, errCourseCancelled) {
			status = model.CourseRunCancelled
		}
		if _, terr := s.repo.Transition(pctx, run.ID, status, &msg); terr != nil {
			log.Error("course: record failure", zap.Error(terr))
		}
		log.Warn("course: run ended", zap.String("status", status), zap.Error(err))
		s.notifyBoard(run.TaskID, fmt.Sprintf("Course run %s: %s", status, msg), "")
		return
	}

	_ = s.repo.UpdateSteps(pctx, run.ID, tracker.finish())
	if _, err := s.repo.Transition(pctx, run.ID, model.CourseRunCompleted, nil); err != nil {
		log.Error("course: record completion", zap.Error(err))
	}
	title := run.Brief.Topic
	if run.Outline != nil && run.Outline.Title != "" {
		title = run.Outline.Title
	}
	log.Info("course: run completed", zap.Int("chapters", chapters))
	s.notifyBoard(run.TaskID, fmt.Sprintf("Course ready for review: %s (%d chapters)", title, chapters), "done")
}

func (s *courseService) heartbeat(runID uuid.UUID, stop <-chan struct{}) {
	t := time.NewTicker(courseHeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if err := s.repo.TouchHeartbeat(context.Background(), runID); err != nil {
				s.logger.Warn("course: heartbeat", zap.String("course_run_id", runID.String()), zap.Error(err))
			}
		}
	}
}

func (s *courseService) notifyBoard(taskID *uuid.UUID, note, status string) {
	if s.tasks == nil || taskID == nil {
		return
	}
	ctx := context.Background()
	n := note
	if task, err := s.tasks.GetByID(ctx, *taskID); err == nil && task != nil && task.Description != nil && *task.Description != "" {
		n = *task.Description + "\n\n" + note
	}
	_, _ = s.tasks.Update(ctx, *taskID, model.UpdateTaskRequest{Description: &n})
	if status != "" {
		_ = s.tasks.Transition(ctx, *taskID, status, nil)
	}
}

func (s *courseService) owned(ctx context.Context, runID, orgID uuid.UUID) (*model.CourseRun, error) {
	run, err := s.repo.GetRun(ctx, runID)
	if errors.Is(err, repository.ErrCourseRunNotFound) {
		return nil, ErrCourseRunNotFound
	}
	if err != nil {
		return nil, err
	}
	if run.OrgID != orgID {
		return nil, ErrCourseRunNotFound
	}
	return run, nil
}

func (s *courseService) GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.CourseRun, error) {
	return s.owned(ctx, runID, orgID)
}

func (s *courseService) ListRuns(ctx context.Context, orgID uuid.UUID, p model.PaginationParams) (*model.PaginatedResponse[model.CourseRun], error) {
	return s.repo.ListRuns(ctx, orgID, p)
}

func (s *courseService) ListChapters(ctx context.Context, runID, orgID uuid.UUID) ([]model.CourseChapter, error) {
	if _, err := s.owned(ctx, runID, orgID); err != nil {
		return nil, err
	}
	return s.repo.ListChapters(ctx, runID)
}

func (s *courseService) CancelRun(ctx context.Context, runID, orgID uuid.UUID) (*model.CourseRun, error) {
	run, err := s.owned(ctx, runID, orgID)
	if err != nil {
		return nil, err
	}
	if run.Status != model.CourseRunQueued && run.Status != model.CourseRunRunning {
		return nil, ErrCourseNotCancellable
	}
	s.mu.Lock()
	cancel, live := s.cancels[runID]
	s.mu.Unlock()
	if live {
		// The run goroutine records the cancellation itself.
		cancel(errCourseCancelled)
	} else {
		// No goroutine in this process (another replica, or a restart):
		// record it directly so the row does not stay running.
		msg := errCourseCancelled.Error()
		if _, err := s.repo.Transition(ctx, runID, model.CourseRunCancelled, &msg); err != nil {
			return nil, err
		}
	}
	run.Status = model.CourseRunCancelled
	return run, nil
}

func (s *courseService) ReapOrphans(ctx context.Context) (int, error) {
	timeout := s.cfg.OrphanTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ids, err := s.repo.ListStaleRunning(ctx, time.Now().Add(-timeout))
	if err != nil {
		return 0, err
	}
	n := 0
	msg := errCourseInterrupted.Error()
	for _, id := range ids {
		s.mu.Lock()
		_, live := s.cancels[id]
		s.mu.Unlock()
		if live {
			continue
		}
		if ok, err := s.repo.Transition(ctx, id, model.CourseRunFailed, &msg); err == nil && ok {
			n++
		}
	}
	return n, nil
}

func (s *courseService) StartReaper(ctx context.Context) {
	tick := func() {
		if n, err := s.ReapOrphans(ctx); err != nil {
			s.logger.Warn("course reaper", zap.Error(err))
		} else if n > 0 {
			s.logger.Info("course reaper failed orphaned runs", zap.Int("count", n))
		}
	}
	tick()
	t := time.NewTicker(courseReaperInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}

func (s *courseService) InterruptAll() {
	s.mu.Lock()
	s.shuttingDown = true
	cancels := make([]context.CancelCauseFunc, 0, len(s.cancels))
	for _, c := range s.cancels {
		cancels = append(cancels, c)
	}
	s.mu.Unlock()
	for _, c := range cancels {
		c(errCourseInterrupted)
	}
}

// courseSteps tracks the fixed step list. Chapters loop through writing →
// quizzing repeatedly, so advancing marks every earlier step done and every
// later one pending; the detail names the chapter.
type courseSteps struct {
	mu      sync.Mutex
	steps   []model.CourseRunStep
	visited map[string]bool
}

func (t *courseSteps) snapshot() []model.CourseRunStep {
	out := make([]model.CourseRunStep, len(t.steps))
	copy(out, t.steps)
	return out
}

func (t *courseSteps) advance(key, detail string) []model.CourseRunStep {
	t.mu.Lock()
	defer t.mu.Unlock()
	idx := -1
	for i, st := range t.steps {
		if st.Key == key {
			idx = i
		}
	}
	if idx < 0 {
		return t.snapshot()
	}
	if t.visited == nil {
		t.visited = map[string]bool{}
	}
	t.visited[key] = true
	for i := range t.steps {
		switch {
		case i < idx:
			t.steps[i].Status = t.doneOrSkipped(t.steps[i].Key)
		case i == idx:
			t.steps[i].Status = "running"
			t.steps[i].Detail = detail
		default:
			t.steps[i].Status = "pending"
		}
	}
	return t.snapshot()
}

func (t *courseSteps) finish() []model.CourseRunStep {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range t.steps {
		t.steps[i].Status = t.doneOrSkipped(t.steps[i].Key)
	}
	return t.snapshot()
}

// doneOrSkipped: a step the pipeline never entered (e.g. illustrating with
// images off) is shown as skipped, not done.
func (t *courseSteps) doneOrSkipped(key string) string {
	if t.visited[key] {
		return "done"
	}
	return "skipped"
}

func (t *courseSteps) fail() []model.CourseRunStep {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range t.steps {
		switch t.steps[i].Status {
		case "running":
			t.steps[i].Status = "failed"
		case "pending":
			t.steps[i].Status = "skipped"
		}
	}
	return t.snapshot()
}
