package video

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llmtrace"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// Media is the slice of the workstation client the service uses.
type Media interface {
	Capabilities(ctx context.Context) (*Capabilities, error)
	Submit(ctx context.Context, req SubmitRequest) (*Job, error)
	Job(ctx context.Context, id string) (*Job, error)
	Jobs(ctx context.Context) ([]Job, error)
	Download(ctx context.Context, id string) ([]byte, error)
	Claim(ctx context.Context, id, holder string) error
	Cancel(ctx context.Context, id string) error
}

// ScriptWriter turns a brief into a script.
type ScriptWriter interface {
	Write(ctx context.Context, b Brief) (*Script, error)
}

// Store keeps the finished file and returns the URL it is served from.
type Store interface {
	PutFile(ctx context.Context, orgID uuid.UUID, data []byte, ext, contentType string) (string, error)
}

// Tasks is the slice of the task service that board updates need.
type Tasks interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.Task, error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateTaskRequest) (*model.Task, error)
	Transition(ctx context.Context, id uuid.UUID, status string, changedBy *uuid.UUID) error
}

// Runs records every launch and where it has got to: what the Video Producer
// tab shows. Satisfied by repository.VideoRunRepository.
type Runs = repository.VideoRunRepository

// StartRequest is a launch, resolved.
type StartRequest struct {
	OrgID   uuid.UUID
	UserID  uuid.UUID
	AgentID uuid.UUID
	TaskID  *uuid.UUID
	Brief   Brief
}

// Service runs Video Producer launches: script here, pictures and sound on the
// workstation, the finished file back into object storage.
//
// Each launch is a run row (Runs), updated at every step so the agent's tab
// can show what it is doing. The workstation holds the render itself and hands
// back the task it belongs to (Job.Ref), so a server that restarts mid-render
// picks its runs up again (StartResumer).
type Service struct {
	writer ScriptWriter
	media  Media
	store  Store
	tasks  Tasks
	runs   Runs
	cfg    Config
	logger *zap.Logger

	// holder names this process when it claims a finished render, so of the
	// replicas that may all be following one render, one collects it.
	holder string

	mu        sync.Mutex
	following map[string]bool
}

// NewService wires the Video Producer. A nil writer, media or store leaves it
// "not configured" — Start says so rather than failing later.
func NewService(writer ScriptWriter, media Media, store Store, cfg Config, logger *zap.Logger) *Service {
	// A typed nil in an interface reads as configured; normalise it.
	if w, ok := writer.(*Writer); ok && (w == nil || w.LLM == nil) {
		writer = nil
	}
	if c, ok := media.(*Client); ok && c == nil {
		media = nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{writer: writer, media: media, store: store, cfg: cfg, logger: logger,
		holder: uuid.NewString(), following: map[string]bool{}}
}

// BindTasks lets runs report to the board.
func (s *Service) BindTasks(tasks Tasks) { s.tasks = tasks }

// BindRuns lets runs be recorded for the agent's tab.
func (s *Service) BindRuns(runs Runs) { s.runs = runs }

// Configured reports whether a launch can run at all.
func (s *Service) Configured() bool {
	return s != nil && s.writer != nil && s.media != nil && s.store != nil
}

// ErrNoRuns: the server keeps no run records (no database wired).
var ErrNoRuns = errors.New("video runs are not recorded on this server")

// List returns an org's runs, newest first.
func (s *Service) List(ctx context.Context, orgID uuid.UUID, limit int) ([]model.VideoRun, error) {
	if s == nil || s.runs == nil {
		return nil, ErrNoRuns
	}
	return s.runs.List(ctx, orgID, limit)
}

// Get returns one of an org's runs.
func (s *Service) Get(ctx context.Context, orgID, id uuid.UUID) (*model.VideoRun, error) {
	if s == nil || s.runs == nil {
		return nil, ErrNoRuns
	}
	return s.runs.Get(ctx, orgID, id)
}

// Cancel stops a run: the workstation drops or stops the render, and the run
// and its task say so. A run that has already finished is returned as is.
func (s *Service) Cancel(ctx context.Context, orgID, id uuid.UUID) (*model.VideoRun, error) {
	run, err := s.Get(ctx, orgID, id)
	if err != nil || !run.Unfinished() {
		return run, err
	}
	s.cancelRun(ctx, run, "Cancelled.")
	s.note(run.TaskID, "Video cancelled.", "")
	return run, nil
}

// cancelRun cancels the render (if it reached the workstation) and records it.
func (s *Service) cancelRun(ctx context.Context, run *model.VideoRun, message string) {
	if s.media != nil && run.Status != model.VideoRunScripting {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		if err := s.media.Cancel(cctx, run.ID.String()); err != nil && !errors.Is(err, ErrNotFound) {
			s.logger.Warn("video: could not cancel the render", zap.String("video_run", run.ID.String()), zap.Error(err))
		}
		cancel()
	}
	s.finish(run, model.VideoRunCancelled, message, "")
}

// Start checks the workstation can render, then makes the video in the
// background. The check is up front so an unreachable or half-installed
// workstation is an immediate error on launch, not a failure an hour later.
func (s *Service) Start(ctx context.Context, req StartRequest) (uuid.UUID, error) {
	switch {
	case s == nil || s.media == nil:
		return uuid.Nil, errors.New("the Video Producer is not configured: IMAGE_BASE_URL does not point at the workstation image service")
	case s.writer == nil:
		return uuid.Nil, errors.New("the Video Producer has no LLM to write scripts with")
	case s.store == nil:
		return uuid.Nil, errors.New("the Video Producer has nowhere to store videos (MinIO)")
	case strings.TrimSpace(req.Brief.Topic) == "":
		return uuid.Nil, errors.New("tell the Video Producer what the video is about")
	}

	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	caps, err := s.media.Capabilities(checkCtx)
	cancel()
	if errors.Is(err, ErrNotFound) {
		return uuid.Nil, errors.New("the workstation image service has no video support yet — deploy image-service from this release there")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("the video workstation is not reachable: %w", err)
	}
	if !caps.FFmpeg || !caps.Narration {
		return uuid.Nil, errors.New("the video workstation is missing ffmpeg or narration — run image-service/install-video-tools.sh on it")
	}

	s.cancelSuperseded(ctx, req.TaskID)

	animated := req.Brief.Motion && caps.Motion
	userID := req.UserID
	run := &model.VideoRun{
		ID:              uuid.New(),
		OrgID:           req.OrgID,
		TaskID:          req.TaskID,
		RequestedBy:     &userID,
		Topic:           req.Brief.Topic,
		Brief:           req.Brief.model(),
		Status:          model.VideoRunScripting,
		Stage:           "script",
		Message:         "Writing the script",
		EstimateMinutes: estimateMinutes(ShotCount(req.Brief.Seconds), animated),
		Shots:           []model.VideoShot{},
		Notes:           []string{},
	}
	if req.AgentID != uuid.Nil {
		agentID := req.AgentID
		run.AgentID = &agentID
	}
	if s.runs != nil {
		if err := s.runs.Create(ctx, run); err != nil {
			return uuid.Nil, fmt.Errorf("video: record the run: %w", err)
		}
	}
	s.claim(run.ID.String())
	go s.run(run, req, caps.Motion)
	return run.ID, nil
}

// cancelSuperseded cancels the renders of a task's earlier launches. A
// relaunch replaces them — only a task's latest run is collected — so left
// alone they would hold the shared GPU for hours for videos nobody collects,
// and queue ahead of the one the user is waiting for.
func (s *Service) cancelSuperseded(ctx context.Context, taskID *uuid.UUID) {
	if taskID == nil {
		return
	}
	if s.runs != nil {
		if older, err := s.runs.UnfinishedForTask(ctx, *taskID); err == nil {
			for i := range older {
				s.cancelRun(ctx, &older[i], "Replaced by a newer launch of this task.")
			}
		}
	}
	// Runs launched before runs were recorded are known only by the task's
	// run_id (the new one is written after Start returns).
	if s.tasks == nil {
		return
	}
	task, err := s.tasks.GetByID(ctx, *taskID)
	if err != nil || task == nil {
		return
	}
	prev, _ := task.Metadata[model.TaskMetaRunID].(string)
	if prev == "" {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// Not found is normal: the previous run may have been another agent's,
	// or already pruned.
	if err := s.media.Cancel(cctx, prev); err != nil && !errors.Is(err, ErrNotFound) {
		s.logger.Warn("video: could not cancel the superseded render", zap.String("video_run", prev), zap.Error(err))
	}
}

func (s *Service) run(run *model.VideoRun, req StartRequest, motionInstalled bool) {
	defer s.release(run.ID.String())
	animated := req.Brief.Motion && motionInstalled
	ctx, cancel := context.WithTimeout(context.Background(), maxFollow)
	defer cancel()
	trace := llmtrace.TraceInfo{
		TraceName: "go-video-script",
		SessionID: run.ID.String(),
		AgentID:   req.AgentID.String(),
		OrgID:     req.OrgID.String(),
	}
	if req.TaskID != nil {
		trace.TaskID = req.TaskID.String()
	}
	ctx = llmtrace.WithTrace(ctx, trace)
	log := s.logger.With(zap.String("video_run", run.ID.String()))

	sctx, scancel := context.WithTimeout(ctx, scriptTimeout)
	script, err := s.writer.Write(sctx, req.Brief)
	scancel()
	if err != nil {
		log.Warn("video: script failed", zap.Error(err))
		s.fail(run, "Writing the script failed: "+err.Error())
		return
	}
	// Cancelled while the script was being written (from the tab, or by a
	// relaunch): stop before the workstation spends hours on it.
	if s.cancelledMeanwhile(ctx, run) {
		return
	}
	run.Title = script.Title
	run.Shots = script.Shots
	run.Status = model.VideoRunQueued
	run.Stage = "queued"
	run.Message = "Script written; sending it to the workstation"
	run.EstimateMinutes = estimateMinutes(len(script.Shots), animated)
	s.save(run)
	s.note(req.TaskID, scriptNote(script, animated, req.Brief.Motion && !motionInstalled), "")

	ref := map[string]string{"org_id": req.OrgID.String()}
	if req.TaskID != nil {
		ref["task_id"] = req.TaskID.String()
	}
	sub := SubmitRequest{
		ID:       run.ID.String(),
		Title:    script.Title,
		Shots:    script.Shots,
		Format:   req.Brief.Format,
		Voice:    req.Brief.Voice,
		Style:    req.Brief.Style,
		Motion:   animated,
		Captions: req.Brief.Captions,
		Ref:      ref,
	}
	// The id is ours, so a resubmission finds the job a lost response already
	// created: retrying is safe. A full queue is not worth retrying for.
	for attempt := 1; ; attempt++ {
		_, err = s.media.Submit(ctx, sub)
		if err == nil || errors.Is(err, ErrBusy) || attempt == 3 {
			break
		}
		log.Warn("video: submit failed; retrying", zap.Error(err))
		time.Sleep(submitRetryDelay)
	}
	if err != nil {
		s.fail(run, "The workstation did not accept the render: "+err.Error())
		return
	}
	log.Info("video: rendering", zap.Int("shots", len(script.Shots)), zap.Bool("animated", animated))
	s.follow(ctx, run, s.budget(len(script.Shots), animated))
}

// cancelledMeanwhile reports whether the run's record says it was cancelled
// by someone else while this process was working on it.
func (s *Service) cancelledMeanwhile(ctx context.Context, run *model.VideoRun) bool {
	if s.runs == nil {
		return false
	}
	latest, err := s.runs.Get(ctx, run.OrgID, run.ID)
	return err == nil && latest.Status == model.VideoRunCancelled
}

const (
	// scriptTimeout bounds writing the script.
	scriptTimeout = 20 * time.Minute
	// maxFollow is the most time spent on one video, queue included. The
	// render budget (budget) starts only once rendering does; this cap is for
	// a workstation that never answers again.
	maxFollow = 48 * time.Hour
)

// submitRetryDelay spaces submission retries. A variable for tests.
var submitRetryDelay = 15 * time.Second

// follow polls a render until it ends, recording its progress, then collects
// it. A workstation that is briefly unreachable (restarting, network) is
// waited out. The render budget runs from when rendering starts, not from
// submission: time spent queued behind other videos is not this video running
// late.
func (s *Service) follow(ctx context.Context, run *model.VideoRun, budget time.Duration) {
	log := s.logger.With(zap.String("video_run", run.ID.String()))
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	var rendering time.Time
	for {
		job, err := s.media.Job(ctx, run.ID.String())
		switch {
		case errors.Is(err, ErrNotFound):
			s.fail(run, "The workstation no longer has this render.")
			return
		case err != nil:
			log.Warn("video: progress check failed; retrying", zap.Error(err))
		case job.Status == JobDone:
			// Several replicas may be following this render (each resumes
			// it after a deploy); the workstation lets one collect it.
			cerr := s.media.Claim(ctx, job.ID, s.holder)
			if cerr == nil {
				if s.collect(ctx, run, job) {
					return
				}
				break // not saved yet: try again next tick
			}
			if errors.Is(cerr, ErrClaimed) || errors.Is(cerr, ErrNotFound) {
				return
			}
			log.Warn("video: claim failed; retrying", zap.Error(cerr))
		case job.Status == JobFailed:
			s.fail(run, "Rendering failed: "+job.Error)
			return
		case job.Status == JobCancelled:
			// Superseded or cancelled from the tab; whoever cancelled it
			// recorded why.
			if run.Status != model.VideoRunCancelled {
				s.finish(run, model.VideoRunCancelled, "Cancelled.", "")
			}
			return
		default:
			if job.Status == JobRunning && rendering.IsZero() {
				rendering = time.Now()
			}
			s.track(run, job)
		}
		if !rendering.IsZero() && time.Since(rendering) > budget {
			s.fail(run, fmt.Sprintf("Rendering took longer than %s. It may still finish on the workstation; launch again to retry.", budget.Round(time.Minute)))
			return
		}
		select {
		case <-ctx.Done():
			s.fail(run, "The workstation did not finish the render in time.")
			return
		case <-ticker.C:
		}
	}
}

// track records what the workstation says it is doing, when it changed.
func (s *Service) track(run *model.VideoRun, job *Job) {
	status, stage, message := model.VideoRunQueued, "queued", "Waiting for the workstation — another video is rendering"
	if job.Status == JobRunning {
		status, stage, message = model.VideoRunRendering, job.Stage, job.Message
	}
	progress := math.Round(job.Progress*1000) / 1000
	if run.Status == status && run.Stage == stage && run.Message == message && run.Progress == progress {
		return
	}
	if status == model.VideoRunRendering && run.StartedAt == nil {
		now := time.Now()
		run.StartedAt = &now
	}
	run.Status, run.Stage, run.Message, run.Progress = status, stage, message, progress
	if job.Title != "" && run.Title == "" {
		run.Title = job.Title
	}
	s.save(run)
}

// collect copies the finished file into object storage and closes the run and
// its task. False means it could not yet (the caller retries; the render stays
// on the workstation).
func (s *Service) collect(ctx context.Context, run *model.VideoRun, job *Job) bool {
	run.Status, run.Stage, run.Progress, run.Message = model.VideoRunRendering, "saving", 1, "Saving the video"
	s.save(run)
	data, err := s.media.Download(ctx, job.ID)
	if err == nil && len(data) == 0 {
		err = errors.New("the file was empty")
	}
	if err != nil {
		run.Message = "Rendered; fetching it from the workstation failed, retrying: " + err.Error()
		s.save(run)
		return false
	}
	url, err := s.store.PutFile(ctx, run.OrgID, data, "mp4", "video/mp4")
	if err != nil {
		run.Message = "Rendered; storing it failed, retrying: " + err.Error()
		s.save(run)
		return false
	}
	if s.cfg.PublicBaseURL != "" {
		url = strings.TrimRight(s.cfg.PublicBaseURL, "/") + url
	}
	run.VideoURL = url
	run.DurationSeconds = job.DurationSeconds
	run.MotionModel = job.MotionModel
	run.Notes = append([]string{}, job.Notes...)
	if run.Title == "" {
		run.Title = job.Title
	}
	s.logger.Info("video: ready", zap.String("video_run", job.ID), zap.Float64("seconds", job.DurationSeconds))
	s.finish(run, model.VideoRunDone, "Ready", "")
	s.note(run.TaskID, doneNote(run.Title, url, job), "done")
	return true
}

// fail records a run's failure and tells its task.
func (s *Service) fail(run *model.VideoRun, why string) {
	s.finish(run, model.VideoRunFailed, why, why)
	s.note(run.TaskID, "Video failed: "+why, "")
}

func (s *Service) finish(run *model.VideoRun, status, message, errMsg string) {
	now := time.Now()
	run.Status, run.Message, run.Error, run.FinishedAt = status, message, errMsg, &now
	// A failed or cancelled run keeps the stage it stopped at, so the tab can
	// show where.
	if status == model.VideoRunDone {
		run.Stage, run.Progress = "done", 1
	}
	s.save(run)
}

func (s *Service) save(run *model.VideoRun) {
	if s.runs == nil {
		return
	}
	if err := s.runs.Save(context.Background(), run); err != nil {
		s.logger.Warn("video: could not record run progress", zap.String("video_run", run.ID.String()), zap.Error(err))
	}
}

// resumeDelay lets the database and the workstation route settle after boot
// before the resumer asks about lost renders. A variable for tests.
var resumeDelay = 30 * time.Second

// StartResumer goes back to the runs a restarted server was following.
//
// The workstation is asked for every render it holds — rings share it, so it
// holds every ring's — and each is matched to this ring's runs. A render
// launched before runs were recorded is adopted into a run when its task is
// still waiting for it. Runs this server never got to hand over (cut off while
// writing the script) are failed: nothing else would ever finish them.
func (s *Service) StartResumer(ctx context.Context) {
	if s == nil || s.media == nil || s.store == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(resumeDelay):
	}
	if s.runs != nil {
		if n, err := s.runs.FailStaleScripting(ctx, time.Now().Add(-scriptTimeout-time.Minute),
			"Interrupted by a server restart while writing the script — launch it again."); err == nil && n > 0 {
			s.logger.Info("video: failed runs cut off while scripting", zap.Int("runs", n))
		}
	}
	jobs, err := s.media.Jobs(ctx)
	if err != nil {
		s.logger.Info("video: resume check skipped — workstation unreachable", zap.Error(err))
		return
	}
	held := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		held[job.ID] = true
		if run := s.resumable(ctx, job); run != nil {
			s.resume(run)
		}
	}
	// A run the workstation no longer holds (pruned, or lost with its disk)
	// is followed once more so it is recorded as failed, not left spinning.
	if s.runs != nil {
		if open, err := s.runs.Rendering(ctx); err == nil {
			for i := range open {
				if !held[open[i].ID.String()] {
					s.resume(&open[i])
				}
			}
		}
	}
}

// resumable returns the run a workstation render belongs to in this ring, or
// nil when it is not this ring's, is finished, or was replaced.
func (s *Service) resumable(ctx context.Context, job Job) *model.VideoRun {
	orgID, oerr := uuid.Parse(job.Ref["org_id"])
	jobID, jerr := uuid.Parse(job.ID)
	if oerr != nil || jerr != nil || job.Status == JobFailed || job.Status == JobCancelled {
		return nil
	}
	if s.runs != nil {
		run, err := s.runs.Get(ctx, orgID, jobID)
		if err == nil {
			if run.Unfinished() {
				return run
			}
			return nil
		}
		if !errors.Is(err, repository.ErrVideoRunNotFound) {
			return nil
		}
	}
	// Launched before runs were recorded: follow it only while its task is
	// still in progress and still on this launch (a relaunch moved it on).
	taskID, terr := uuid.Parse(job.Ref["task_id"])
	if terr != nil || s.tasks == nil {
		return nil
	}
	task, err := s.tasks.GetByID(ctx, taskID)
	if err != nil || task == nil || task.Status != "in_progress" {
		return nil
	}
	if current, _ := task.Metadata[model.TaskMetaRunID].(string); current != job.ID {
		return nil
	}
	run := &model.VideoRun{
		ID: jobID, OrgID: orgID, TaskID: &taskID, Topic: job.Title, Title: job.Title,
		Status: model.VideoRunQueued, Stage: "queued", Message: "Picked up after a server restart",
		Brief: model.VideoBrief{Motion: true}, EstimateMinutes: estimateMinutes(job.Shots, true),
		Shots: []model.VideoShot{}, Notes: []string{},
	}
	if s.runs != nil {
		if err := s.runs.Create(ctx, run); err != nil {
			s.logger.Warn("video: could not record an adopted render", zap.String("video_run", job.ID), zap.Error(err))
		}
	}
	return run
}

func (s *Service) resume(run *model.VideoRun) {
	if !s.claim(run.ID.String()) {
		return
	}
	s.logger.Info("video: resuming", zap.String("video_run", run.ID.String()), zap.String("status", run.Status))
	go func() {
		defer s.release(run.ID.String())
		ctx, cancel := context.WithTimeout(context.Background(), maxFollow)
		defer cancel()
		shots := max(len(run.Shots), 1)
		s.follow(ctx, run, s.budget(shots, run.Brief.Motion))
	}()
}

// claim marks a render as followed by this process; false if it already is.
// Per process only: across replicas, the workstation's collect claim (Media.
// Claim) is what keeps a render from being stored twice.
func (s *Service) claim(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.following[id] {
		return false
	}
	s.following[id] = true
	return true
}

func (s *Service) release(id string) {
	s.mu.Lock()
	delete(s.following, id)
	s.mu.Unlock()
}

// note appends a line to the task's description and optionally moves it.
func (s *Service) note(taskID *uuid.UUID, note, status string) {
	if s.tasks == nil || taskID == nil {
		return
	}
	ctx := context.Background()
	text := note
	if task, err := s.tasks.GetByID(ctx, *taskID); err == nil && task != nil && task.Description != nil && *task.Description != "" {
		text = *task.Description + "\n\n" + note
	}
	_, _ = s.tasks.Update(ctx, *taskID, model.UpdateTaskRequest{Description: &text})
	if status != "" {
		_ = s.tasks.Transition(ctx, *taskID, status, nil)
	}
}

// budget is how long a render may run: twice the expected time plus slack,
// never less than VIDEO_MAX_RUNTIME. A flat cap sized for a short
// video would abandon a long animated one that is still rendering.
func (s *Service) budget(shots int, animated bool) time.Duration {
	expected := time.Duration(estimateMinutes(shots, animated)) * time.Minute
	return max(s.cfg.MaxRuntime, 15*time.Minute+2*expected)
}

// Minutes of workstation time per shot, measured on the M4 Max: a keyframe is
// about 1.5 min; animating it (LTX-2.3 distilled, 5 s at 1024x576) about 10 more.
const (
	minutesPerStill    = 1.5
	minutesPerAnimated = 10.0
)

// estimateMinutes is the expected workstation time for a render.
func estimateMinutes(shots int, animated bool) int {
	per := minutesPerStill
	if animated {
		per += minutesPerAnimated
	}
	return int(math.Ceil(per * float64(shots)))
}

func scriptNote(script *Script, animated, motionMissing bool) string {
	how := "still pictures with slow camera moves"
	if animated {
		how = "animated shots"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Script written: %q, %d shots. Rendering %s on the workstation — about %d minutes. Follow it on the Video Producer's Workspace tab.",
		script.Title, len(script.Shots), how, estimateMinutes(len(script.Shots), animated))
	if motionMissing {
		b.WriteString(" (Animation is not installed on the workstation, so stills are used.)")
	}
	b.WriteString("\n")
	for i, shot := range script.Shots {
		fmt.Fprintf(&b, "\n%d. %s", i+1, shot.Narration)
	}
	return b.String()
}

func doneNote(title, url string, job *Job) string {
	secs := int(math.Round(job.DurationSeconds))
	how := "still pictures with camera moves"
	if job.MotionModel != "" {
		how = "animated with " + job.MotionModel
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Video ready: %s\n%s\n%d:%02d · %d shots · %s", title, url, secs/60, secs%60, job.Shots, how)
	for _, n := range job.Notes {
		b.WriteString("\nNote: " + n)
	}
	return b.String()
}
