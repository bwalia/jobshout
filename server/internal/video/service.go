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
// It keeps no table of its own. The workstation holds each render's state and
// hands back the task it belongs to (Job.Ref), and the board task holds the
// rest, so a server that restarts mid-render finds its work again by asking
// the workstation (StartResumer).
type Service struct {
	writer ScriptWriter
	media  Media
	store  Store
	tasks  Tasks
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

// Configured reports whether a launch can run at all.
func (s *Service) Configured() bool {
	return s != nil && s.writer != nil && s.media != nil && s.store != nil
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

	runID := uuid.New()
	s.claim(runID.String())
	go s.run(runID, req, caps.Motion)
	return runID, nil
}

// cancelSuperseded cancels the render of a task's previous launch. A relaunch
// replaces it — the resumer only collects the task's current run — so left
// alone it would hold the shared GPU for hours for a video nobody collects,
// and queue ahead of the one the user is waiting for. The task still carries
// the previous run's id here: the new one is written after Start returns.
func (s *Service) cancelSuperseded(ctx context.Context, taskID *uuid.UUID) {
	if taskID == nil || s.tasks == nil {
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

func (s *Service) run(runID uuid.UUID, req StartRequest, motionInstalled bool) {
	defer s.release(runID.String())
	animated := req.Brief.Motion && motionInstalled
	ctx, cancel := context.WithTimeout(context.Background(), maxFollow)
	defer cancel()
	trace := llmtrace.TraceInfo{
		TraceName: "go-video-script",
		SessionID: runID.String(),
		AgentID:   req.AgentID.String(),
		OrgID:     req.OrgID.String(),
	}
	if req.TaskID != nil {
		trace.TaskID = req.TaskID.String()
	}
	ctx = llmtrace.WithTrace(ctx, trace)
	log := s.logger.With(zap.String("video_run", runID.String()))

	sctx, scancel := context.WithTimeout(ctx, scriptTimeout)
	script, err := s.writer.Write(sctx, req.Brief)
	scancel()
	if err != nil {
		log.Warn("video: script failed", zap.Error(err))
		s.note(req.TaskID, "Video failed while writing the script: "+err.Error(), "")
		return
	}
	s.note(req.TaskID, scriptNote(script, animated, req.Brief.Motion && !motionInstalled), "")

	ref := map[string]string{"org_id": req.OrgID.String()}
	if req.TaskID != nil {
		ref["task_id"] = req.TaskID.String()
	}
	sub := SubmitRequest{
		ID:       runID.String(),
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
		s.note(req.TaskID, "Video failed: the workstation did not accept the render: "+err.Error(), "")
		return
	}
	log.Info("video: rendering", zap.Int("shots", len(script.Shots)), zap.Bool("animated", animated))
	s.follow(ctx, runID.String(), req.OrgID, req.TaskID, script.Title, s.budget(len(script.Shots), animated))
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

// follow polls a render until it ends, then collects it. A workstation that
// is briefly unreachable (restarting, network) is waited out. The render
// budget runs from when rendering starts, not from submission: time spent
// queued behind other videos is not this video running late.
func (s *Service) follow(ctx context.Context, jobID string, orgID uuid.UUID, taskID *uuid.UUID, title string, budget time.Duration) {
	log := s.logger.With(zap.String("video_run", jobID))
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	var rendering time.Time
	for {
		job, err := s.media.Job(ctx, jobID)
		switch {
		case errors.Is(err, ErrNotFound):
			s.note(taskID, "Video failed: the workstation no longer has this render.", "")
			return
		case err != nil:
			log.Warn("video: progress check failed; retrying", zap.Error(err))
		case job.Status == JobDone:
			// Several replicas may be following this render (each resumes
			// it after a deploy); the workstation lets one collect it.
			cerr := s.media.Claim(ctx, job.ID, s.holder)
			if cerr == nil {
				s.collect(ctx, job, orgID, taskID, firstNonEmpty(title, job.Title))
				return
			}
			if errors.Is(cerr, ErrClaimed) || errors.Is(cerr, ErrNotFound) {
				return
			}
			log.Warn("video: claim failed; retrying", zap.Error(cerr))
		case job.Status == JobFailed:
			s.note(taskID, "Video failed while rendering: "+job.Error, "")
			return
		case job.Status == JobCancelled:
			// Superseded by a relaunch of the task, which reports for itself.
			return
		case job.Status == JobRunning && rendering.IsZero():
			rendering = time.Now()
		}
		if !rendering.IsZero() && time.Since(rendering) > budget {
			s.note(taskID, fmt.Sprintf("Video failed: rendering took longer than %s. It may still finish on the workstation; launch again to retry.", budget.Round(time.Minute)), "")
			return
		}
		select {
		case <-ctx.Done():
			s.note(taskID, "Video failed: the workstation did not finish the render in time.", "")
			return
		case <-ticker.C:
		}
	}
}

// collect copies the finished file into object storage and closes the task.
// A failure here leaves the task in progress, so the next server start
// collects it again (the render is still on the workstation).
func (s *Service) collect(ctx context.Context, job *Job, orgID uuid.UUID, taskID *uuid.UUID, title string) {
	data, err := s.media.Download(ctx, job.ID)
	if err == nil && len(data) == 0 {
		err = errors.New("the file was empty")
	}
	if err != nil {
		s.note(taskID, "The video rendered but could not be fetched from the workstation: "+err.Error(), "")
		return
	}
	url, err := s.store.PutFile(ctx, orgID, data, "mp4", "video/mp4")
	if err != nil {
		s.note(taskID, "The video rendered but could not be stored: "+err.Error(), "")
		return
	}
	if s.cfg.PublicBaseURL != "" {
		url = strings.TrimRight(s.cfg.PublicBaseURL, "/") + url
	}
	s.logger.Info("video: ready", zap.String("video_run", job.ID), zap.Float64("seconds", job.DurationSeconds))
	s.note(taskID, doneNote(title, url, job), "done")
}

// resumeDelay lets the database and the workstation route settle after boot
// before the resumer asks about lost renders. A variable for tests.
var resumeDelay = 30 * time.Second

// StartResumer follows renders whose server restarted while they ran. Rings
// share the workstation, so it sees every ring's renders; a task this ring's
// database does not hold, or that is no longer in progress, is skipped.
func (s *Service) StartResumer(ctx context.Context) {
	if s == nil || s.media == nil || s.store == nil || s.tasks == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(resumeDelay):
	}
	jobs, err := s.media.Jobs(ctx)
	if err != nil {
		s.logger.Info("video: resume check skipped — workstation unreachable", zap.Error(err))
		return
	}
	for _, job := range jobs {
		taskID, terr := uuid.Parse(job.Ref["task_id"])
		orgID, oerr := uuid.Parse(job.Ref["org_id"])
		if terr != nil || oerr != nil || job.Status == JobFailed || job.Status == JobCancelled {
			continue
		}
		task, err := s.tasks.GetByID(ctx, taskID)
		if err != nil || task == nil || task.Status != "in_progress" {
			continue
		}
		// A task launched again carries the new run's id. An older render of
		// it — still kept on the workstation — is not this task's video.
		if runID, _ := task.Metadata[model.TaskMetaRunID].(string); runID != job.ID || !s.claim(job.ID) {
			continue
		}
		s.logger.Info("video: resuming", zap.String("video_run", job.ID), zap.String("status", job.Status))
		go func(job Job) {
			defer s.release(job.ID)
			rctx, cancel := context.WithTimeout(context.Background(), maxFollow)
			defer cancel()
			// The workstation does not say whether a resumed render is
			// animated; budget as if it is.
			s.follow(rctx, job.ID, orgID, &taskID, job.Title, s.budget(job.Shots, true))
		}(job)
	}
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
	per := minutesPerStill
	if animated {
		per += minutesPerAnimated
	}
	expected := time.Duration(per * float64(shots) * float64(time.Minute))
	return max(s.cfg.MaxRuntime, 15*time.Minute+2*expected)
}

// Minutes of workstation time per shot, measured on the M4 Max: a keyframe is
// about 1.5 min; animating it (LTX-2.3 distilled, 5 s at 1024x576) about 10 more.
const (
	minutesPerStill    = 1.5
	minutesPerAnimated = 10.0
)

func scriptNote(script *Script, animated, motionMissing bool) string {
	per := minutesPerStill
	how := "still pictures with slow camera moves"
	if animated {
		per += minutesPerAnimated
		how = "animated shots"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Script written: %q, %d shots. Rendering %s on the workstation — about %d minutes.",
		script.Title, len(script.Shots), how, int(math.Ceil(per*float64(len(script.Shots)))))
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
