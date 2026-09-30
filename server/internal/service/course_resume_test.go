package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/course"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// These tests run the real course.Generator against a stand-in for the world
// outside the process (LLM, research, image service) that outlives a
// "server". A server is one courseService; a restart is a new one on the same
// repository.

var resumeChapters = []string{"Alpha", "Beta", "Gamma"}

const resumeQuizJSON = `{"questions":[
 {"question":"Q1?","options":["a","b","c","d"],"correct_index":0,"explanation":"e1"},
 {"question":"Q2?","options":["a","b","c","d"],"correct_index":1,"explanation":"e2"},
 {"question":"Q3?","options":["a","b","c","d"],"correct_index":2,"explanation":"e3"}]}`

// world counts every stage call by key ("research", "outline", "write:2",
// "review:2", "image:2", "quiz:2", "cover") and can hold one stage until its
// caller is cancelled or the test lets it go.
type world struct {
	mu      sync.Mutex
	calls   map[string]int
	holdKey string
	held    chan struct{} // closed when a call reaches the held stage
	letGo   chan struct{} // closed to let the held call finish
}

func newWorld() *world { return &world{calls: map[string]int{}} }

// hold makes the next call of key wait. The returned channel closes when a
// call is waiting there.
func (w *world) hold(key string) <-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.holdKey, w.held, w.letGo = key, make(chan struct{}), make(chan struct{})
	return w.held
}

// release lets the held call return normally, as if its server were alive.
func (w *world) release() {
	w.mu.Lock()
	defer w.mu.Unlock()
	close(w.letGo)
}

func (w *world) enter(ctx context.Context, key string) error {
	w.mu.Lock()
	w.calls[key]++
	var letGo chan struct{}
	if w.holdKey == key {
		w.holdKey = ""
		close(w.held)
		letGo = w.letGo
	}
	w.mu.Unlock()
	if letGo == nil {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-letGo:
		return nil
	}
}

func (w *world) count(key string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls[key]
}

func (w *world) ProviderName() string { return "stub" }

func chapterOf(prompt string) int {
	for i, title := range resumeChapters {
		if strings.Contains(prompt, "body of "+title) {
			return i + 1
		}
	}
	return 0
}

func (w *world) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	prompt := req.Messages[len(req.Messages)-1].Content
	var key, reply string
	switch {
	case strings.Contains(prompt, "Plan a course."):
		key = "outline"
		var chs []string
		for _, t := range resumeChapters {
			chs = append(chs, fmt.Sprintf(`{"title":%q,"summary":"About %s.","objectives":["Know %s"]}`, t, t, t))
		}
		reply = `{"title":"Resumable","description":"d","category":"general","tags":["t"],"learning_outcomes":["Learn"],"chapters":[` + strings.Join(chs, ",") + `]}`
	case strings.Contains(prompt, "Review this course chapter"):
		key, reply = fmt.Sprintf("review:%d", chapterOf(prompt)), `{"issues":[]}`
	case strings.Contains(prompt, "Write a multiple-choice quiz"):
		key, reply = fmt.Sprintf("quiz:%d", chapterOf(prompt)), resumeQuizJSON
	default:
		n := 0
		if _, err := fmt.Sscanf(prompt, "Write chapter %d of a course.", &n); err != nil {
			return nil, errors.New("stub: unexpected prompt: " + prompt[:min(60, len(prompt))])
		}
		key = fmt.Sprintf("write:%d", n)
		reply = "## Intro\n\nThe body of " + resumeChapters[n-1] + "."
	}
	if err := w.enter(ctx, key); err != nil {
		return nil, err
	}
	return &llm.GenerateResponse{Content: reply}, nil
}

func (w *world) Research(ctx context.Context, _ research.Request, _ research.ProgressFunc) (*research.Brief, error) {
	if err := w.enter(ctx, "research"); err != nil {
		return nil, err
	}
	return &research.Brief{
		Topic:    "Resumable",
		Summary:  "s",
		Findings: []research.Finding{{Claim: "A fact.", SourceURL: "https://example.org/a"}},
		Sources:  []research.Source{{URL: "https://example.org/a", Title: "A"}},
		Warnings: []string{"set aside 1 search result(s)"},
	}, nil
}

func (w *world) Illustrate(ctx context.Context, _ uuid.UUID, _ *uuid.UUID, prompt string, width, _ int) (string, error) {
	key := "cover"
	if width == 1280 {
		for i, title := range resumeChapters {
			if strings.Contains(prompt, title) {
				key = fmt.Sprintf("image:%d", i+1)
			}
		}
	}
	if err := w.enter(ctx, key); err != nil {
		return "", err
	}
	return "/img/" + uuid.NewString() + ".png", nil
}

type resumeEnv struct {
	t     *testing.T
	w     *world
	repo  *memCourseRepo
	org   uuid.UUID
	agent *model.Agent
	cfg   course.Config
}

func newResumeEnv(t *testing.T) *resumeEnv {
	org := uuid.New()
	return &resumeEnv{
		t: t, w: newWorld(), repo: newMemCourseRepo(), org: org, agent: courseAgent(org),
		cfg: course.Config{
			PlanBudget: time.Minute, ChapterBudget: time.Minute, MaxChapters: 8,
			Images: true, OrphanTimeout: time.Hour, MaxResumes: 5,
		},
	}
}

// server boots a new API process on the same database.
func (e *resumeEnv) server() *courseService {
	gen := course.NewGenerator(e.w, e.w, e.w, e.cfg, nil)
	return NewCourseService(e.repo, &courseAgents{agent: e.agent}, gen, e.cfg, nil).(*courseService)
}

func (e *resumeEnv) launch(svc *courseService) uuid.UUID {
	e.t.Helper()
	run, err := svc.CreateRun(context.Background(), model.CreateCourseRunRequest{
		AgentID: e.agent.ID, Brief: model.CourseBrief{Topic: "Resumable", ChapterCount: len(resumeChapters)},
	}, e.org, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return run.ID
}

func waitHeld(t *testing.T, held <-chan struct{}) {
	t.Helper()
	select {
	case <-held:
	case <-time.After(2 * time.Second):
		t.Fatal("the run never reached the stage it should be interrupted in")
	}
}

// kill ends a server the way SIGKILL does: its goroutines stop and nothing is
// written on the way out.
func kill(svc *courseService) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.shuttingDown = true
	for _, l := range svc.live {
		l.cancel(errCourseLost)
	}
}

// makeStale ages the run's heartbeat past the orphan timeout.
func (e *resumeEnv) makeStale(id uuid.UUID) {
	e.repo.mu.Lock()
	defer e.repo.mu.Unlock()
	old := time.Now().Add(-2 * e.cfg.OrphanTimeout)
	e.repo.runs[id].HeartbeatAt = &old
}

func (e *resumeEnv) resume(svc *courseService, want int) {
	e.t.Helper()
	if n, err := svc.ResumeInterrupted(context.Background()); err != nil || n != want {
		e.t.Fatalf("ResumeInterrupted = %d, %v; want %d", n, err, want)
	}
}

// assertComplete checks the finished course: every chapter exactly once, in
// order, with its quiz and image, and an accurate step list.
func (e *resumeEnv) assertComplete(id uuid.UUID, attempt int) {
	e.t.Helper()
	waitStatus(e.t, e.repo, id, model.CourseRunCompleted)
	run, _ := e.repo.GetRun(context.Background(), id)
	if run.Attempt != attempt {
		e.t.Errorf("attempt = %d, want %d", run.Attempt, attempt)
	}
	if run.ErrorMessage != nil {
		e.t.Errorf("a completed run carries no error, got %q", *run.ErrorMessage)
	}
	if run.CoverURL == "" {
		e.t.Error("cover missing")
	}
	chapters, _ := e.repo.ListChapters(context.Background(), id)
	if len(chapters) != len(resumeChapters) {
		e.t.Fatalf("chapters = %d, want %d", len(chapters), len(resumeChapters))
	}
	for i, ch := range chapters {
		if ch.Position != i+1 || ch.Title != resumeChapters[i] {
			e.t.Errorf("chapter %d = position %d %q", i+1, ch.Position, ch.Title)
		}
		if !strings.Contains(ch.Markdown, "body of "+resumeChapters[i]) {
			e.t.Errorf("chapter %d has the wrong text: %q", i+1, ch.Markdown)
		}
		if ch.Quiz == nil || len(ch.Quiz.Questions) != 3 || len(ch.Images) != 1 {
			e.t.Errorf("chapter %d is missing its quiz or image: quiz=%v images=%d", i+1, ch.Quiz != nil, len(ch.Images))
		}
		if n := e.repo.upserts[id][ch.Position]; n != 1 {
			e.t.Errorf("chapter %d was saved %d times, want once", ch.Position, n)
		}
	}
	for _, st := range run.Steps {
		if st.Status != "done" {
			e.t.Errorf("step %s = %s, want done", st.Key, st.Status)
		}
	}
	if run.Progress == nil || run.Progress.Draft != nil || !run.Progress.CoverDone {
		e.t.Errorf("progress should show the cover done and no chapter in flight: %+v", run.Progress)
	}
}

// assertCalls checks how often each stage ran. Stages not listed must have
// run exactly once.
func (e *resumeEnv) assertCalls(twice ...string) {
	e.t.Helper()
	want := map[string]int{"research": 1, "outline": 1, "cover": 1}
	for i := range resumeChapters {
		for _, stage := range []string{"write", "review", "image", "quiz"} {
			want[fmt.Sprintf("%s:%d", stage, i+1)] = 1
		}
	}
	for _, k := range twice {
		want[k]++
	}
	for k, n := range want {
		if got := e.w.count(k); got != n {
			e.t.Errorf("stage %s ran %d times, want %d", k, got, n)
		}
	}
}

func TestCourseResume_RestartDuringResearch(t *testing.T) {
	e := newResumeEnv(t)
	s1 := e.server()
	held := e.w.hold("research")
	id := e.launch(s1)
	waitHeld(t, held)

	s1.InterruptAll()
	// The process exits right after InterruptAll returns, so the hand-back
	// must already be recorded — and as resuming, not failed.
	if got := e.repo.status(id); got != model.CourseRunResuming {
		t.Fatalf("status after shutdown = %q, want resuming", got)
	}

	e.resume(e.server(), 1)
	e.assertComplete(id, 2)
	e.assertCalls("research")
}

func TestCourseResume_RestartDuringOutline(t *testing.T) {
	e := newResumeEnv(t)
	s1 := e.server()
	held := e.w.hold("outline")
	id := e.launch(s1)
	waitHeld(t, held)
	s1.InterruptAll()

	run, _ := e.repo.GetRun(context.Background(), id)
	if run.ResearchNotes == "" || len(run.Sources) != 1 {
		t.Fatalf("research must be saved before outlining starts: notes=%q sources=%d", run.ResearchNotes, len(run.Sources))
	}

	e.resume(e.server(), 1)
	e.assertComplete(id, 2)
	// Research is not repeated; only the outline is.
	e.assertCalls("outline")

	run, _ = e.repo.GetRun(context.Background(), id)
	research := 0
	for _, w := range run.Warnings {
		if strings.HasPrefix(w, "research:") {
			research++
		}
	}
	if research != 1 {
		t.Errorf("research warnings must not repeat on resume: %v", run.Warnings)
	}
}

func TestCourseResume_RestartDuringChapter(t *testing.T) {
	for _, stage := range []string{"write:2", "review:2", "image:2", "quiz:2"} {
		t.Run(stage, func(t *testing.T) {
			e := newResumeEnv(t)
			s1 := e.server()
			held := e.w.hold(stage)
			id := e.launch(s1)
			waitHeld(t, held)
			e.finishAfterShutdown(s1, id, stage)
		})
	}
}

// finishAfterShutdown restarts the server with chapter 2 in flight and checks
// that chapter 1 is left alone and only the interrupted stage is redone.
func (e *resumeEnv) finishAfterShutdown(s1 *courseService, id uuid.UUID, stage string) {
	e.t.Helper()
	s1.InterruptAll()
	if got := e.repo.status(id); got != model.CourseRunResuming {
		e.t.Fatalf("status after shutdown = %q, want resuming", got)
	}
	before, _ := e.repo.ListChapters(context.Background(), id)
	if len(before) != 1 || before[0].Position != 1 {
		e.t.Fatalf("only chapter 1 should be saved at shutdown, got %d chapters", len(before))
	}

	e.resume(e.server(), 1)
	e.assertComplete(id, 2)
	e.assertCalls(stage)

	after, _ := e.repo.ListChapters(context.Background(), id)
	if after[0].Markdown != before[0].Markdown || after[0].Images[0].URL != before[0].Images[0].URL {
		e.t.Error("a chapter finished before the restart must not be regenerated")
	}
}

func TestCourseResume_MultipleRestarts(t *testing.T) {
	e := newResumeEnv(t)
	e.cfg.MaxResumes = 10
	stages := []string{"research", "outline", "cover", "write:1", "review:2", "image:2", "quiz:3"}
	svc := e.server()
	held := e.w.hold(stages[0])
	id := e.launch(svc)
	for i := range stages {
		waitHeld(t, held)
		if i+1 < len(stages) {
			held = e.w.hold(stages[i+1])
		}
		// Alternate a clean shutdown with a killed pod.
		if i%2 == 0 {
			svc.InterruptAll()
		} else {
			kill(svc)
			if got := e.repo.status(id); got != model.CourseRunRunning {
				t.Fatalf("a killed server writes nothing; status = %q", got)
			}
			e.makeStale(id)
		}
		svc = e.server()
		e.resume(svc, 1)
	}
	e.assertComplete(id, len(stages)+1)
	e.assertCalls(stages...)
}

func TestCourseResume_KilledPodIsResumedOnlyOnceHeartbeatIsStale(t *testing.T) {
	e := newResumeEnv(t)
	s1 := e.server()
	held := e.w.hold("write:2")
	id := e.launch(s1)
	waitHeld(t, held)
	kill(s1)

	s2 := e.server()
	// The heartbeat is still fresh: the writer may be alive on another replica.
	e.resume(s2, 0)
	if got := e.repo.status(id); got != model.CourseRunRunning {
		t.Fatalf("status = %q, want running", got)
	}
	e.makeStale(id)
	e.resume(s2, 1)
	e.assertComplete(id, 2)
	e.assertCalls("write:2")
}

func TestCourseResume_TwoServersResumeARunOnce(t *testing.T) {
	e := newResumeEnv(t)
	s1 := e.server()
	held := e.w.hold("write:1")
	id := e.launch(s1)
	waitHeld(t, held)
	s1.InterruptAll()

	a, b := e.server(), e.server()
	var wg sync.WaitGroup
	started := make([]int, 2)
	for i, svc := range []*courseService{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started[i], _ = svc.ResumeInterrupted(context.Background())
		}()
	}
	wg.Wait()
	if started[0]+started[1] != 1 {
		t.Fatalf("the run must be resumed by exactly one server, got %v", started)
	}
	e.assertComplete(id, 2)
	e.assertCalls("write:1")
}

// A server that was only slow, not dead, comes back to find its run resumed
// elsewhere. Its late writes must not land.
func TestCourseResume_SupersededAttemptCannotWrite(t *testing.T) {
	e := newResumeEnv(t)
	s1 := e.server()
	held := e.w.hold("write:2")
	id := e.launch(s1)
	waitHeld(t, held)

	// s1 is still alive and blocked in its LLM call; its heartbeat went stale.
	e.makeStale(id)
	s2 := e.server()
	e.resume(s2, 1)
	e.assertComplete(id, 2)

	// Now the old call returns and s1 tries to carry on.
	e.w.release()
	deadline := time.Now().Add(2 * time.Second)
	for {
		s1.mu.Lock()
		n := len(s1.live)
		s1.mu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the superseded attempt never stopped")
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.assertComplete(id, 2)
	if e.w.count("review:2") != 1 || e.w.count("write:3") != 1 {
		t.Errorf("the superseded attempt carried on generating: review:2=%d write:3=%d", e.w.count("review:2"), e.w.count("write:3"))
	}
}

func TestCourseResume_GivesUpAfterMaxResumes(t *testing.T) {
	e := newResumeEnv(t)
	e.cfg.MaxResumes = 2
	svc := e.server()
	held := e.w.hold("research")
	id := e.launch(svc)
	for range 2 {
		waitHeld(t, held)
		held = e.w.hold("research")
		svc.InterruptAll()
		svc = e.server()
		e.resume(svc, 1)
	}
	waitHeld(t, held)
	svc.InterruptAll()

	e.resume(e.server(), 0)
	run, _ := e.repo.GetRun(context.Background(), id)
	if run.Status != model.CourseRunFailed || run.ErrorMessage == nil || !strings.Contains(*run.ErrorMessage, "too many times") {
		t.Fatalf("run = %s %v, want failed after running out of resumes", run.Status, run.ErrorMessage)
	}
	for _, st := range run.Steps {
		if st.Status == "running" || st.Status == "pending" {
			t.Errorf("a failed run must not leave step %s as %s", st.Key, st.Status)
		}
	}
}

func TestCourseResume_CancelledWhileWaitingIsNotResumed(t *testing.T) {
	e := newResumeEnv(t)
	s1 := e.server()
	held := e.w.hold("write:1")
	id := e.launch(s1)
	waitHeld(t, held)
	s1.InterruptAll()

	s2 := e.server()
	if _, err := s2.CancelRun(context.Background(), id, e.org); err != nil {
		t.Fatalf("a run waiting to resume must be cancellable: %v", err)
	}
	e.resume(s2, 0)
	if got := e.repo.status(id); got != model.CourseRunCancelled {
		t.Fatalf("status = %q, want cancelled", got)
	}
	if e.w.count("write:1") != 1 {
		t.Error("a cancelled run must not generate again")
	}
}

// A failure that is not a restart still fails the run; resume is not a retry
// loop for broken runs.
func TestCourseResume_RealFailureIsNotResumed(t *testing.T) {
	org := uuid.New()
	agent := courseAgent(org)
	svc, repo := newCourseSvc(&fakeGen{err: errors.New("model offline")}, agent)
	run, _ := svc.CreateRun(context.Background(), model.CreateCourseRunRequest{AgentID: agent.ID, Brief: model.CourseBrief{Topic: "Go"}}, org, nil)
	waitStatus(t, repo, run.ID, model.CourseRunFailed)
	if n, _ := svc.ResumeInterrupted(context.Background()); n != 0 {
		t.Errorf("a failed run must not be resumed, resumed %d", n)
	}
}
