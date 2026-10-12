package video

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
)

func TestParseScriptAcceptsDriftedFieldNamesAndDropsHalfShots(t *testing.T) {
	reply := "Here you go:\n```json\n" + `{"title": "Lighthouses", "scenes": [
		{"voiceover": "For two centuries, lighthouses kept sailors alive.", "image_prompt": "A stone lighthouse at dusk"},
		{"narration": ["Then came GPS.", "Or did it?"], "visual_prompt": "A ship's bridge with a glowing screen"},
		{"narration": "A line with nothing to show."},
		{"visual": "A picture with nothing said."}
	]}` + "\n```"
	got, err := ParseScript(reply)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Lighthouses" || len(got.Shots) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got.Shots[1].Narration != "Then came GPS. Or did it?" {
		t.Fatalf("list narration not joined: %q", got.Shots[1].Narration)
	}
	if got.Shots[0].Visual != "A stone lighthouse at dusk" {
		t.Fatalf("visual alias not read: %q", got.Shots[0].Visual)
	}
}

func TestParseScriptKeepsTheTitleWithinTheWorkstationLimit(t *testing.T) {
	got, err := ParseScript(`{"title": "` + strings.Repeat("long ", 100) + `", "shots": [{"narration": "a", "visual": "b"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(got.Title)); n > maxTitle {
		t.Fatalf("title is %d runes", n)
	}
}

func TestParseScriptRejectsUnusableReplies(t *testing.T) {
	for _, reply := range []string{"", "no json here", `{"title": "x", "shots": []}`, `{"shots": [{"narration": "only words"}]}`} {
		if _, err := ParseScript(reply); err == nil {
			t.Errorf("accepted %q", reply)
		}
	}
}

func TestBriefDefaultsAndClamps(t *testing.T) {
	b := BriefFromValues(map[string]string{"topic": "  tides  ", "length": "9999", "voice": "--rm", "motion": "stills", "captions": "no", "format": "portrait"})
	if b.Topic != "tides" || b.Seconds != 180 || b.Voice != defaultVoice || b.Motion || b.Captions || b.Format != "portrait" {
		t.Fatalf("got %+v", b)
	}
	d := BriefFromValues(map[string]string{"topic": "tides"})
	if d.Seconds != 60 || d.Format != "landscape" || !d.Motion || !d.Captions || d.Voice != defaultVoice {
		t.Fatalf("defaults: %+v", d)
	}
}

func TestShotCountAndLanguageFollowTheBrief(t *testing.T) {
	if ShotCount(60) != 10 || ShotCount(5) != 3 || ShotCount(100000) != maxShots {
		t.Fatal("shot count out of bounds")
	}
	if !strings.HasPrefix(Language("hf_alpha"), "Hindi") || Language("bm_george") != "British English" {
		t.Fatal("voice language")
	}
}

func TestTheRunBudgetGrowsWithTheVideo(t *testing.T) {
	s := &Service{cfg: Config{MaxRuntime: 4 * time.Hour}}
	if got := s.budget(3, false); got != 4*time.Hour {
		t.Fatalf("a short stills video should get the floor, got %s", got)
	}
	// A 3-minute animated video is ~30 shots of ~11.5 min: far past a flat 4 h.
	if got := s.budget(30, true); got < 11*time.Hour {
		t.Fatalf("a long animated video would be abandoned mid-render: %s", got)
	}
}

// ─── service flow ────────────────────────────────────────────────────────────

type fakeWriter struct{}

func (fakeWriter) Write(context.Context, Brief) (*Script, error) {
	return &Script{Title: "Tides", Shots: []Shot{{Narration: "The sea breathes twice a day.", Visual: "A tidal flat at dawn"}}}, nil
}

type fakeMedia struct {
	mu        sync.Mutex
	caps      Capabilities
	submitted []SubmitRequest
	polls     int
	jobs      []Job
	// statuses is what successive polls report; the last one repeats.
	// Empty means running once, then done.
	statuses  []string
	claimErr  error
	submitErr []error
}

func (m *fakeMedia) Capabilities(context.Context) (*Capabilities, error) { return &m.caps, nil }

func (m *fakeMedia) Submit(_ context.Context, req SubmitRequest) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submitted = append(m.submitted, req)
	if len(m.submitErr) > 0 {
		err := m.submitErr[0]
		m.submitErr = m.submitErr[1:]
		return nil, err
	}
	return &Job{ID: req.ID, Status: JobQueued}, nil
}

func (m *fakeMedia) Job(_ context.Context, id string) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	statuses := m.statuses
	if len(statuses) == 0 {
		statuses = []string{JobRunning, JobDone}
	}
	status := statuses[min(m.polls, len(statuses)-1)]
	m.polls++
	return &Job{ID: id, Status: status, Shots: 1, DurationSeconds: 65, Notes: []string{"shot 1: animation failed, used a still"}}, nil
}

func (m *fakeMedia) Claim(context.Context, string, string) error { return m.claimErr }

func (m *fakeMedia) Jobs(context.Context) ([]Job, error) { return m.jobs, nil }

func (m *fakeMedia) Download(context.Context, string) ([]byte, error) { return []byte("mp4"), nil }

type fakeStore struct{ ext, contentType string }

func (s *fakeStore) PutFile(_ context.Context, _ uuid.UUID, _ []byte, ext, contentType string) (string, error) {
	s.ext, s.contentType = ext, contentType
	return "/api/v1/images/file/org/2026/10/x.mp4", nil
}

type fakeTasks struct {
	mu     sync.Mutex
	task   model.Task
	status chan string
}

func newFakeTasks(id uuid.UUID) *fakeTasks {
	return &fakeTasks{task: model.Task{ID: id, Status: "in_progress"}, status: make(chan string, 4)}
}

func (f *fakeTasks) GetByID(_ context.Context, id uuid.UUID) (*model.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != f.task.ID {
		return nil, errors.New("not found")
	}
	t := f.task
	return &t, nil
}

func (f *fakeTasks) Update(_ context.Context, _ uuid.UUID, req model.UpdateTaskRequest) (*model.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.task.Description = req.Description
	return &f.task, nil
}

func (f *fakeTasks) Transition(_ context.Context, _ uuid.UUID, status string, _ *uuid.UUID) error {
	f.mu.Lock()
	f.task.Status = status
	f.mu.Unlock()
	f.status <- status
	return nil
}

func (f *fakeTasks) description() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.task.Description == nil {
		return ""
	}
	return *f.task.Description
}

func testConfig() Config {
	return Config{MaxRuntime: time.Minute, PollInterval: 5 * time.Millisecond, PublicBaseURL: "https://int.jobshout.co.uk"}
}

func waitStatus(t *testing.T, tasks *fakeTasks) string {
	t.Helper()
	select {
	case s := <-tasks.status:
		return s
	case <-time.After(5 * time.Second):
		t.Fatalf("task never moved; description so far:\n%s", tasks.description())
		return ""
	}
}

func TestARunWritesRendersStoresAndClosesTheTask(t *testing.T) {
	taskID := uuid.New()
	tasks := newFakeTasks(taskID)
	// Animation not installed: an animated request must render as stills.
	media := &fakeMedia{caps: Capabilities{FFmpeg: true, Narration: true, Motion: false}}
	store := &fakeStore{}
	svc := NewService(fakeWriter{}, media, store, testConfig(), zap.NewNop())
	svc.BindTasks(tasks)

	runID, err := svc.Start(context.Background(), StartRequest{OrgID: uuid.New(), TaskID: &taskID,
		Brief: Brief{Topic: "tides", Seconds: 30, Format: "landscape", Voice: "af_heart", Motion: true, Captions: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got := waitStatus(t, tasks); got != "done" {
		t.Fatalf("status %q", got)
	}

	media.mu.Lock()
	sub := media.submitted[0]
	media.mu.Unlock()
	if sub.ID != runID.String() || sub.Motion || sub.Ref["task_id"] != taskID.String() {
		t.Fatalf("submitted %+v", sub)
	}
	if store.ext != "mp4" || store.contentType != "video/mp4" {
		t.Fatalf("stored as %s %s", store.ext, store.contentType)
	}
	desc := tasks.description()
	for _, want := range []string{"Script written", "stills are used", "Video ready: Tides",
		"https://int.jobshout.co.uk/api/v1/images/file/", "1:05", "animation failed"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description lacks %q:\n%s", want, desc)
		}
	}
}

func TestStartFailsFastOnAHalfInstalledWorkstation(t *testing.T) {
	media := &fakeMedia{caps: Capabilities{FFmpeg: true, Narration: false}}
	svc := NewService(fakeWriter{}, media, &fakeStore{}, testConfig(), nil)
	if _, err := svc.Start(context.Background(), StartRequest{Brief: Brief{Topic: "tides"}}); err == nil ||
		!strings.Contains(err.Error(), "install-video-tools.sh") {
		t.Fatalf("err = %v", err)
	}
	if len(media.submitted) != 0 {
		t.Fatal("submitted anyway")
	}
}

func TestAnUnconfiguredServiceSaysSo(t *testing.T) {
	var nilClient *Client
	svc := NewService(&Writer{}, nilClient, &fakeStore{}, testConfig(), nil)
	if svc.Configured() {
		t.Fatal("typed nils read as configured")
	}
	if _, err := svc.Start(context.Background(), StartRequest{Brief: Brief{Topic: "x"}}); err == nil {
		t.Fatal("started without a workstation")
	}
}

func TestTheResumerCollectsARenderItsServerLost(t *testing.T) {
	old := resumeDelay
	resumeDelay = 0
	defer func() { resumeDelay = old }()

	taskID, orgID := uuid.New(), uuid.New()
	tasks := newFakeTasks(taskID)
	tasks.task.Metadata = map[string]any{model.TaskMetaRunID: "lost-render"}
	ref := map[string]string{"task_id": taskID.String(), "org_id": orgID.String()}
	media := &fakeMedia{jobs: []Job{
		// An earlier run of the same task, finished and still on disk: not
		// this task's current video, so it must not close the task.
		{ID: "previous-run", Title: "Old", Status: JobDone, Ref: ref},
		{ID: "lost-render", Title: "Tides", Status: JobRunning, Ref: ref},
		// Another ring's render: its task is not in this database.
		{ID: "other-ring", Status: JobDone, Ref: map[string]string{"task_id": uuid.NewString(), "org_id": orgID.String()}},
	}}
	store := &fakeStore{}
	svc := NewService(fakeWriter{}, media, store, testConfig(), nil)
	svc.BindTasks(tasks)

	go svc.StartResumer(context.Background())
	if got := waitStatus(t, tasks); got != "done" {
		t.Fatalf("status %q", got)
	}
	if d := tasks.description(); !strings.Contains(d, "Video ready: Tides") || strings.Contains(d, "Old") {
		t.Fatalf("description: %s", d)
	}
}

func TestAReplicaThatLosesTheCollectClaimStandsDown(t *testing.T) {
	taskID := uuid.New()
	tasks := newFakeTasks(taskID)
	media := &fakeMedia{statuses: []string{JobDone}, claimErr: ErrClaimed}
	store := &fakeStore{}
	svc := NewService(fakeWriter{}, media, store, testConfig(), nil)
	svc.BindTasks(tasks)
	svc.follow(context.Background(), "render", uuid.New(), &taskID, "Tides", time.Hour)
	if store.ext != "" || tasks.description() != "" {
		t.Fatalf("collected anyway: stored %q, note %q", store.ext, tasks.description())
	}
}

func TestTimeQueuedIsNotCountedAgainstTheRender(t *testing.T) {
	taskID := uuid.New()
	tasks := newFakeTasks(taskID)
	media := &fakeMedia{statuses: []string{JobQueued, JobQueued, JobQueued, JobRunning}}
	svc := NewService(fakeWriter{}, media, &fakeStore{}, testConfig(), nil)
	svc.BindTasks(tasks)
	// A budget shorter than the time spent queued: only rendering counts.
	svc.follow(context.Background(), "render", uuid.New(), &taskID, "Tides", 12*time.Millisecond)
	media.mu.Lock()
	polls := media.polls
	media.mu.Unlock()
	if polls < 5 {
		t.Fatalf("gave up after %d polls, while still queued", polls)
	}
	if !strings.Contains(tasks.description(), "rendering took longer") {
		t.Fatalf("description: %s", tasks.description())
	}
}

func TestASubmitLostInTransitIsRetried(t *testing.T) {
	old := submitRetryDelay
	submitRetryDelay = 0
	defer func() { submitRetryDelay = old }()

	taskID := uuid.New()
	tasks := newFakeTasks(taskID)
	media := &fakeMedia{caps: Capabilities{FFmpeg: true, Narration: true}, submitErr: []error{errors.New("connection reset")}}
	svc := NewService(fakeWriter{}, media, &fakeStore{}, testConfig(), nil)
	svc.BindTasks(tasks)
	if _, err := svc.Start(context.Background(), StartRequest{OrgID: uuid.New(), TaskID: &taskID,
		Brief: Brief{Topic: "tides", Seconds: 30}}); err != nil {
		t.Fatal(err)
	}
	if got := waitStatus(t, tasks); got != "done" {
		t.Fatalf("status %q", got)
	}
	if len(media.submitted) != 2 || media.submitted[0].ID != media.submitted[1].ID {
		t.Fatalf("expected one retry with the same id, got %+v", media.submitted)
	}
}
