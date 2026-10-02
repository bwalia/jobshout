package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/course"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/llmtrace"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ErrCourseRunNotFound is returned when a run is missing or belongs to another org.
var ErrCourseRunNotFound = errors.New("course run not found")

// ErrCourseNotCancellable is returned for terminal runs.
var ErrCourseNotCancellable = errors.New("course run cannot be cancelled")

// Vars so tests can shorten them.
var (
	courseHeartbeatInterval = 30 * time.Second
	courseResumeInterval    = 20 * time.Second
)

// CourseService is the launch and HTTP surface for the Course Generator.
type CourseService interface {
	CreateRun(ctx context.Context, req model.CreateCourseRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.CourseRun, error)
	GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.CourseRun, error)
	ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CourseRun], error)
	ListChapters(ctx context.Context, runID, orgID uuid.UUID) ([]model.CourseChapter, error)
	CancelRun(ctx context.Context, runID, orgID uuid.UUID) (*model.CourseRun, error)
	BindTasks(tasks TaskService)
	// ResumeInterrupted picks up runs whose server went away — handed back at
	// shutdown, or left with a stale heartbeat by a killed pod — and carries
	// them on from their saved state. It returns how many it started.
	ResumeInterrupted(ctx context.Context) (int, error)
	// StartResumer runs ResumeInterrupted now and on a ticker until ctx ends.
	StartResumer(ctx context.Context)
	// InterruptAll stops in-flight runs on shutdown and hands them back to be
	// resumed by the next server.
	InterruptAll()
}

// courseGenerator is the slice of *course.Generator the service uses.
type courseGenerator interface {
	Ready() error
	Generate(ctx context.Context, job course.Job, h course.Hooks) error
}

// courseLive is a run generating in this process.
type courseLive struct {
	cancel  context.CancelCauseFunc
	attempt int
}

type courseService struct {
	repo      repository.CourseRepository
	agentRepo repository.AgentRepository
	gen       courseGenerator
	cfg       course.Config
	logger    *zap.Logger

	tasks TaskService

	mu           sync.Mutex
	live         map[uuid.UUID]courseLive
	shuttingDown bool
}

var (
	errCourseCancelled = errors.New("cancelled by user")
	// errCourseInterrupted stops a run at shutdown. It is not a failure: the
	// run is handed back and resumed.
	errCourseInterrupted = errors.New("interrupted: the server is restarting; the course will resume")
	// errCourseLost stops a goroutine whose run is no longer its attempt's.
	errCourseLost     = errors.New("course run was taken over by another attempt")
	errCourseTimedOut = errors.New("timed out: the run exceeded its runtime budget (COURSE_MAX_RUNTIME per chapter)")
	errCourseGaveUp   = errors.New("interrupted too many times: the server restarted during generation on every attempt (COURSE_MAX_RESUMES); launch the course again")
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
		live:      make(map[uuid.UUID]courseLive),
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
	brief = withAgentModel(brief, agent)

	now := time.Now()
	run := &model.CourseRun{
		ID:          uuid.New(),
		AgentID:     agent.ID,
		TaskID:      req.TaskID,
		OrgID:       orgID,
		Status:      model.CourseRunRunning,
		Brief:       brief,
		Steps:       initialCourseSteps(),
		Attempt:     1,
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

	// The goroutine gets its own copy: the caller serialises run while
	// generation is updating outline and steps.
	work := *run
	work.Steps = append([]model.CourseRunStep(nil), run.Steps...)
	s.launch(&work, course.State{})
	return run, nil
}

// launch starts run's current attempt from state. If the server began
// shutting down in the meantime, the run is handed back for the next server.
func (s *courseService) launch(run *model.CourseRun, state course.State) bool {
	runCtx, cancel := context.WithCancelCause(context.Background())
	runCtx, cancelTimeout := context.WithTimeoutCause(runCtx, s.budget(run, state), errCourseTimedOut)

	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		cancelTimeout()
		cancel(errCourseInterrupted)
		if _, err := s.repo.Release(context.Background(), run.ID, run.Attempt); err != nil {
			s.logger.Warn("course: hand back run", zap.String("course_run_id", run.ID.String()), zap.Error(err))
		}
		return false
	}
	s.live[run.ID] = courseLive{cancel: cancel, attempt: run.Attempt}
	s.mu.Unlock()

	go func() {
		defer cancelTimeout()
		s.execute(runCtx, cancel, run, state)
	}()
	return true
}

// budget is the deadline for this attempt: the whole run for a fresh one, the
// chapters still to write for a resumed one.
func (s *courseService) budget(run *model.CourseRun, state course.State) time.Duration {
	if state.Outline == nil {
		return s.cfg.RunBudget(run.Brief.ChapterCount)
	}
	remaining := max(len(state.Outline.Chapters)-len(state.Done), 1)
	return time.Duration(remaining) * s.cfg.ChapterBudget
}

func initialCourseSteps() []model.CourseRunStep {
	steps := make([]model.CourseRunStep, len(model.CourseStepOrder))
	for i, k := range model.CourseStepOrder {
		steps[i] = model.CourseRunStep{Key: k, Status: "pending"}
	}
	return steps
}

func (s *courseService) execute(ctx context.Context, cancel context.CancelCauseFunc, run *model.CourseRun, state course.State) {
	defer func() {
		s.mu.Lock()
		if l, ok := s.live[run.ID]; ok && l.attempt == run.Attempt {
			delete(s.live, run.ID)
		}
		s.mu.Unlock()
	}()
	trace := llmtrace.TraceInfo{
		TraceName: "go-course-run",
		SessionID: run.ID.String(),
		AgentID:   run.AgentID.String(),
		OrgID:     run.OrgID.String(),
	})
	log := s.logger.With(zap.String("course_run_id", run.ID.String()), zap.Int("attempt", run.Attempt))
	lost := func() { cancel(errCourseLost) }

	stopHB := make(chan struct{})
	defer close(stopHB)
	go s.heartbeat(run.ID, run.Attempt, stopHB, lost)

	// Persistence uses a background context: a cancelled run must still be
	// able to record that it was cancelled. Every write is guarded by the
	// attempt; one that finds the run is no longer ours stops the goroutine.
	pctx := context.Background()
	owned := func(err error) error {
		if errors.Is(err, repository.ErrCourseRunLost) {
			lost()
		}
		return err
	}
	tracker := newCourseSteps(run.Steps)

	err := s.gen.Generate(ctx, course.Job{RunID: run.ID, OrgID: run.OrgID, UserID: run.RequestedBy, Brief: run.Brief, Resume: state}, course.Hooks{
		Step: func(key, detail string) {
			err := owned(s.repo.UpdateSteps(pctx, run.ID, run.Attempt, tracker.advance(key, detail)))
			if err != nil && !errors.Is(err, repository.ErrCourseRunLost) {
				log.Warn("course: persist steps", zap.Error(err))
			}
		},
		Researched: func(notes string, sources []model.CourseSource) error {
			return owned(s.repo.SaveResearch(pctx, run.ID, run.Attempt, notes, sources))
		},
		Planned: func(o *model.CourseOutline, sources []model.CourseSource) error {
			run.Outline = o
			return owned(s.repo.SaveOutline(pctx, run.ID, run.Attempt, o, sources))
		},
		Cover: func(url string) error { return owned(s.repo.SetCover(pctx, run.ID, run.Attempt, url)) },
		Progress: func(p model.CourseProgress) error {
			return owned(s.repo.SaveProgress(pctx, run.ID, run.Attempt, &p))
		},
		Chapter: func(ch *model.CourseChapter) error {
			// Chapters live in their own table, so check ownership first.
			if ok, err := s.repo.TouchHeartbeat(pctx, run.ID, run.Attempt); err != nil {
				return err
			} else if !ok {
				return owned(repository.ErrCourseRunLost)
			}
			ch.RunID = run.ID
			return s.repo.UpsertChapter(pctx, ch)
		},
		Warn: func(msg string) {
			log.Info("course: warning", zap.String("warning", msg))
			if err := s.repo.AppendWarning(pctx, run.ID, msg); err != nil {
				log.Warn("course: persist warning", zap.Error(err))
			}
		},
	})

	if err != nil {
		cause := context.Cause(ctx)
		switch {
		case errors.Is(cause, errCourseLost):
			// Resumed elsewhere, or ended from another replica. Not ours to record.
			log.Info("course: attempt stopped, the run is no longer this attempt's")
			return
		case errors.Is(cause, errCourseInterrupted):
			// InterruptAll has normally handed it back already; this covers
			// the goroutine getting here first.
			if _, rerr := s.repo.Release(pctx, run.ID, run.Attempt); rerr != nil {
				log.Warn("course: hand back run", zap.Error(rerr))
			}
			log.Info("course: run interrupted by shutdown, it will resume")
			return
		}
		if cause != nil {
			err = cause
		}
		msg := err.Error()
		status := model.CourseRunFailed
		if errors.Is(err, errCourseCancelled) {
			status = model.CourseRunCancelled
		}
		ended, terr := s.repo.Finish(pctx, run.ID, run.Attempt, status, &msg)
		if terr != nil {
			log.Error("course: record failure", zap.Error(terr))
		}
		log.Warn("course: run ended", zap.String("status", status), zap.Error(err))
		if ended {
			s.notifyBoard(run.TaskID, fmt.Sprintf("Course run %s: %s", status, msg), "")
		}
		return
	}

	if err := s.repo.UpdateSteps(pctx, run.ID, run.Attempt, tracker.finish()); err != nil {
		log.Warn("course: persist steps", zap.Error(err))
	}
	ended, err := s.repo.Finish(pctx, run.ID, run.Attempt, model.CourseRunCompleted, nil)
	if err != nil {
		log.Error("course: record completion", zap.Error(err))
	}
	if !ended {
		return
	}
	title := run.Brief.Topic
	if run.Outline != nil && run.Outline.Title != "" {
		title = run.Outline.Title
	}
	chapters := 0
	if saved, err := s.repo.ListChapters(pctx, run.ID); err == nil {
		chapters = len(saved)
	}
	log.Info("course: run completed", zap.Int("chapters", chapters))
	s.notifyBoard(run.TaskID, fmt.Sprintf("Course ready for review: %s (%d chapters)", title, chapters), "done")
}

// heartbeat keeps the attempt's heartbeat fresh, and calls lost once if the
// run stops being this attempt's.
func (s *courseService) heartbeat(runID uuid.UUID, attempt int, stop <-chan struct{}, lost func()) {
	t := time.NewTicker(courseHeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			ok, err := s.repo.TouchHeartbeat(context.Background(), runID, attempt)
			if err != nil {
				// A database blip is not evidence the run was taken away.
				s.logger.Warn("course: heartbeat", zap.String("course_run_id", runID.String()), zap.Error(err))
				continue
			}
			if !ok {
				lost()
				return
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
	switch run.Status {
	case model.CourseRunQueued, model.CourseRunRunning, model.CourseRunResuming:
	default:
		return nil, ErrCourseNotCancellable
	}
	s.mu.Lock()
	l, live := s.live[runID]
	s.mu.Unlock()
	if live {
		// The run goroutine records the cancellation itself.
		l.cancel(errCourseCancelled)
	} else {
		// No goroutine in this process (another replica, or waiting to be
		// resumed): record it directly. A goroutine elsewhere stops at its
		// next write or heartbeat, and a cancelled run is never resumed.
		msg := errCourseCancelled.Error()
		if _, err := s.repo.Transition(ctx, runID, model.CourseRunCancelled, &msg); err != nil {
			return nil, err
		}
	}
	run.Status = model.CourseRunCancelled
	return run, nil
}

func (s *courseService) ResumeInterrupted(ctx context.Context) (int, error) {
	s.mu.Lock()
	down := s.shuttingDown
	s.mu.Unlock()
	if down || s.gen == nil {
		return 0, nil
	}
	if err := s.gen.Ready(); err != nil {
		// Leave the runs for a server that can generate.
		return 0, nil
	}
	timeout := s.cfg.OrphanTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	refs, err := s.repo.ListResumable(ctx, time.Now().Add(-timeout))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, ref := range refs {
		s.mu.Lock()
		_, live := s.live[ref.ID]
		s.mu.Unlock()
		if live {
			continue
		}
		log := s.logger.With(zap.String("course_run_id", ref.ID.String()))
		// Attempt 1 is the launch, so ref.Attempt-1 resumes have been used.
		if ref.Attempt-1 >= s.cfg.MaxResumes {
			msg := errCourseGaveUp.Error()
			if ok, err := s.repo.Transition(ctx, ref.ID, model.CourseRunFailed, &msg); err != nil {
				log.Warn("course: fail run out of resumes", zap.Error(err))
			} else if ok {
				log.Warn("course: run failed, out of resumes", zap.Int("attempt", ref.Attempt))
			}
			continue
		}
		run, err := s.repo.Claim(ctx, ref)
		if err != nil {
			log.Warn("course: claim run", zap.Error(err))
			continue
		}
		if run == nil {
			continue // another server has it, or it ended
		}
		state, err := s.savedState(ctx, run)
		if err != nil {
			log.Warn("course: load saved state, will retry", zap.Error(err))
			_, _ = s.repo.Release(context.Background(), run.ID, run.Attempt)
			continue
		}
		log.Info("course: resuming run",
			zap.Int("attempt", run.Attempt),
			zap.Bool("researched", state.Notes != ""),
			zap.Bool("outlined", state.Outline != nil),
			zap.Int("chapters_saved", len(state.Done)))
		if s.launch(run, state) {
			n++
			note := fmt.Sprintf("resumed after a server restart (attempt %d)", run.Attempt)
			if err := s.repo.AppendWarning(context.Background(), run.ID, note); err != nil {
				log.Warn("course: persist warning", zap.Error(err))
			}
		}
	}
	return n, nil
}

// savedState gathers what earlier attempts of run saved.
func (s *courseService) savedState(ctx context.Context, run *model.CourseRun) (course.State, error) {
	state := course.State{
		Notes:   run.ResearchNotes,
		Sources: run.Sources,
		Done:    map[int]bool{},
	}
	if run.Outline != nil && len(run.Outline.Chapters) > 0 {
		state.Outline = run.Outline
	}
	if run.Progress != nil {
		state.Progress = *run.Progress
	}
	chapters, err := s.repo.ListChapters(ctx, run.ID)
	if err != nil {
		return state, err
	}
	for _, ch := range chapters {
		if ch.Locale == run.Brief.Locale {
			state.Done[ch.Position] = true
		}
	}
	return state, nil
}

func (s *courseService) StartResumer(ctx context.Context) {
	tick := func() {
		if n, err := s.ResumeInterrupted(ctx); err != nil {
			s.logger.Warn("course resumer", zap.Error(err))
		} else if n > 0 {
			s.logger.Info("course resumer picked up interrupted runs", zap.Int("count", n))
		}
	}
	tick()
	t := time.NewTicker(courseResumeInterval)
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
	live := make(map[uuid.UUID]courseLive, len(s.live))
	for id, l := range s.live {
		live[id] = l
	}
	s.mu.Unlock()
	for id, l := range live {
		l.cancel(errCourseInterrupted)
		// Hand the run back here, not only in the run goroutine: the process
		// exits before that goroutine has unwound from its LLM call, and the
		// run would then wait out the heartbeat timeout before resuming.
		if _, err := s.repo.Release(context.Background(), id, l.attempt); err != nil {
			s.logger.Warn("course: hand back run", zap.String("course_run_id", id.String()), zap.Error(err))
		}
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

// newCourseSteps starts from a run's saved step list. For a resumed run, the
// steps an earlier attempt entered count as visited, and the one it was
// interrupted in goes back to pending until the pipeline re-enters it.
func newCourseSteps(saved []model.CourseRunStep) *courseSteps {
	t := &courseSteps{visited: map[string]bool{}}
	if len(saved) == 0 {
		saved = initialCourseSteps()
	}
	t.steps = make([]model.CourseRunStep, len(saved))
	copy(t.steps, saved)
	for i, st := range t.steps {
		switch st.Status {
		case "done":
			t.visited[st.Key] = true
		case "running", "failed":
			t.visited[st.Key] = true
			t.steps[i].Status = "pending"
		}
	}
	return t
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
