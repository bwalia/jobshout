package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/course"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type memCourseRepo struct {
	mu       sync.Mutex
	runs     map[uuid.UUID]*model.CourseRun
	chapters map[uuid.UUID][]model.CourseChapter
	// upserts counts chapter writes per run and position.
	upserts map[uuid.UUID]map[int]int
}

func newMemCourseRepo() *memCourseRepo {
	return &memCourseRepo{
		runs:     map[uuid.UUID]*model.CourseRun{},
		chapters: map[uuid.UUID][]model.CourseChapter{},
		upserts:  map[uuid.UUID]map[int]int{},
	}
}

func (m *memCourseRepo) CreateRun(_ context.Context, r *model.CourseRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *r
	if cp.Attempt < 1 {
		cp.Attempt = 1
	}
	m.runs[r.ID] = &cp
	return nil
}

func (m *memCourseRepo) GetRun(_ context.Context, id uuid.UUID) (*model.CourseRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return nil, repository.ErrCourseRunNotFound
	}
	return cloneCourseRun(r), nil
}

// cloneCourseRun copies what the generator mutates after a save (the draft),
// the way a database row would.
func cloneCourseRun(r *model.CourseRun) *model.CourseRun {
	cp := *r
	cp.Steps = append([]model.CourseRunStep(nil), r.Steps...)
	if r.Progress != nil {
		p := *r.Progress
		if p.Draft != nil {
			d := *p.Draft
			d.Images = append([]model.CourseImage(nil), d.Images...)
			p.Draft = &d
		}
		cp.Progress = &p
	}
	return &cp
}

func (m *memCourseRepo) ListRuns(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.CourseRun], error) {
	return &model.PaginatedResponse[model.CourseRun]{}, nil
}

// ownedRun mirrors the SQL guard: the row changes only while it is running on
// this attempt. Callers hold m.mu.
func (m *memCourseRepo) ownedRun(id uuid.UUID, attempt int) (*model.CourseRun, error) {
	r := m.runs[id]
	if r == nil || r.Attempt != attempt || r.Status != model.CourseRunRunning {
		return nil, repository.ErrCourseRunLost
	}
	return r, nil
}

func (m *memCourseRepo) UpdateSteps(_ context.Context, id uuid.UUID, attempt int, s []model.CourseRunStep) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.ownedRun(id, attempt)
	if err != nil {
		return err
	}
	r.Steps = s
	return nil
}

func (m *memCourseRepo) SaveResearch(_ context.Context, id uuid.UUID, attempt int, notes string, s []model.CourseSource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.ownedRun(id, attempt)
	if err != nil {
		return err
	}
	r.ResearchNotes, r.Sources = notes, s
	return nil
}

func (m *memCourseRepo) SaveOutline(_ context.Context, id uuid.UUID, attempt int, o *model.CourseOutline, s []model.CourseSource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.ownedRun(id, attempt)
	if err != nil {
		return err
	}
	r.Outline, r.Sources = o, s
	return nil
}

func (m *memCourseRepo) SetCover(_ context.Context, id uuid.UUID, attempt int, u string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.ownedRun(id, attempt)
	if err != nil {
		return err
	}
	r.CoverURL = u
	return nil
}

func (m *memCourseRepo) SaveProgress(_ context.Context, id uuid.UUID, attempt int, p *model.CourseProgress) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.ownedRun(id, attempt)
	if err != nil {
		return err
	}
	r.Progress = cloneCourseRun(&model.CourseRun{Progress: p}).Progress
	return nil
}

func (m *memCourseRepo) AppendWarning(_ context.Context, id uuid.UUID, w string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[id].Warnings = append(m.runs[id].Warnings, w)
	return nil
}

func closeSteps(steps []model.CourseRunStep) {
	for i := range steps {
		switch steps[i].Status {
		case "running":
			steps[i].Status = "failed"
		case "pending":
			steps[i].Status = "skipped"
		}
	}
}

func (m *memCourseRepo) Finish(_ context.Context, id uuid.UUID, attempt int, status string, msg *string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.ownedRun(id, attempt)
	if err != nil {
		return false, nil
	}
	r.Status, r.ErrorMessage = status, msg
	if status != model.CourseRunCompleted {
		closeSteps(r.Steps)
	}
	return true, nil
}

func (m *memCourseRepo) Transition(_ context.Context, id uuid.UUID, status string, msg *string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.runs[id]
	switch r.Status {
	case model.CourseRunCompleted, model.CourseRunFailed, model.CourseRunCancelled:
		return false, nil
	}
	r.Status = status
	if msg != nil {
		r.ErrorMessage = msg
	}
	if status == model.CourseRunFailed || status == model.CourseRunCancelled {
		closeSteps(r.Steps)
	}
	return true, nil
}

func (m *memCourseRepo) TouchHeartbeat(_ context.Context, id uuid.UUID, attempt int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.ownedRun(id, attempt)
	if err != nil {
		return false, nil
	}
	now := time.Now()
	r.HeartbeatAt = &now
	return true, nil
}

func (m *memCourseRepo) Release(_ context.Context, id uuid.UUID, attempt int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.ownedRun(id, attempt)
	if err != nil {
		return false, nil
	}
	r.Status = model.CourseRunResuming
	return true, nil
}

func (m *memCourseRepo) ListResumable(_ context.Context, staleBefore time.Time) ([]repository.CourseRunRef, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var refs []repository.CourseRunRef
	for _, r := range m.runs {
		stale := r.HeartbeatAt == nil || r.HeartbeatAt.Before(staleBefore)
		if r.Status == model.CourseRunResuming || ((r.Status == model.CourseRunRunning || r.Status == model.CourseRunQueued) && stale) {
			refs = append(refs, repository.CourseRunRef{ID: r.ID, Attempt: r.Attempt})
		}
	}
	return refs, nil
}

func (m *memCourseRepo) Claim(_ context.Context, ref repository.CourseRunRef) (*model.CourseRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.runs[ref.ID]
	if r == nil || r.Attempt != ref.Attempt {
		return nil, nil
	}
	switch r.Status {
	case model.CourseRunQueued, model.CourseRunRunning, model.CourseRunResuming:
	default:
		return nil, nil
	}
	now := time.Now()
	r.Status, r.Attempt, r.HeartbeatAt, r.ErrorMessage = model.CourseRunRunning, r.Attempt+1, &now, nil
	return cloneCourseRun(r), nil
}

func (m *memCourseRepo) UpsertChapter(_ context.Context, ch *model.CourseChapter) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.upserts[ch.RunID] == nil {
		m.upserts[ch.RunID] = map[int]int{}
	}
	m.upserts[ch.RunID][ch.Position]++
	for i, have := range m.chapters[ch.RunID] {
		if have.Locale == ch.Locale && have.Position == ch.Position {
			m.chapters[ch.RunID][i] = *ch
			return nil
		}
	}
	m.chapters[ch.RunID] = append(m.chapters[ch.RunID], *ch)
	return nil
}

func (m *memCourseRepo) ListChapters(_ context.Context, id uuid.UUID) ([]model.CourseChapter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]model.CourseChapter(nil), m.chapters[id]...), nil
}

func (m *memCourseRepo) status(id uuid.UUID) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[id].Status
}

type courseAgents struct {
	repository.AgentRepository
	agent *model.Agent
}

func (a *courseAgents) FindByID(context.Context, uuid.UUID) (*model.Agent, error) {
	return a.agent, nil
}

// fakeGen writes one chapter, or blocks until cancelled when block is set.
type fakeGen struct {
	block bool
	err   error
}

func (f *fakeGen) Ready() error { return nil }

func (f *fakeGen) Generate(ctx context.Context, job course.Job, h course.Hooks) error {
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if f.err != nil {
		return f.err
	}
	h.Step(model.CourseStepResearching, "")
	if err := h.Planned(&model.CourseOutline{Title: "Go"}, nil); err != nil {
		return err
	}
	h.Step(model.CourseStepWriting, "1/1")
	if err := h.Chapter(&model.CourseChapter{Position: 1, Locale: job.Brief.Locale, Title: "Intro"}); err != nil {
		return err
	}
	h.Step(model.CourseStepSaved, "")
	return nil
}

func courseAgent(org uuid.UUID) *model.Agent {
	return &model.Agent{ID: uuid.New(), OrgID: org, Metadata: map[string]any{model.MetadataKeyBuiltin: model.BuiltinCourseGenerator}}
}

func waitStatus(t *testing.T, repo *memCourseRepo, id uuid.UUID, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if repo.status(id) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("run status = %q, want %q", repo.status(id), want)
}

func newCourseSvc(gen courseGenerator, agent *model.Agent) (*courseService, *memCourseRepo) {
	repo := newMemCourseRepo()
	cfg := course.Config{PlanBudget: time.Minute, ChapterBudget: time.Minute, MaxChapters: 8}
	svc := NewCourseService(repo, &courseAgents{agent: agent}, gen, cfg, nil).(*courseService)
	return svc, repo
}

func TestCourseRun_CompletesAndSavesChapters(t *testing.T) {
	org := uuid.New()
	agent := courseAgent(org)
	svc, repo := newCourseSvc(&fakeGen{}, agent)

	run, err := svc.CreateRun(context.Background(), model.CreateCourseRunRequest{
		AgentID: agent.ID, Brief: model.CourseBrief{Topic: "Go", ChapterCount: 1},
	}, org, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, repo, run.ID, model.CourseRunCompleted)

	chapters, err := svc.ListChapters(context.Background(), run.ID, org)
	if err != nil || len(chapters) != 1 || chapters[0].RunID != run.ID {
		t.Fatalf("chapters = %+v, %v", chapters, err)
	}
	got, _ := repo.GetRun(context.Background(), run.ID)
	for _, st := range got.Steps {
		want := "done"
		if st.Key == model.CourseStepReviewing || st.Key == model.CourseStepIllustrating || st.Key == model.CourseStepQuizzing || st.Key == model.CourseStepOutlining {
			want = "skipped"
		}
		if st.Status != want {
			t.Errorf("step %s = %s, want %s", st.Key, st.Status, want)
		}
	}
}

func TestCourseRun_OtherOrgCannotSeeOrLaunch(t *testing.T) {
	org, other := uuid.New(), uuid.New()
	agent := courseAgent(org)
	svc, repo := newCourseSvc(&fakeGen{}, agent)

	if _, err := svc.CreateRun(context.Background(), model.CreateCourseRunRequest{
		AgentID: agent.ID, Brief: model.CourseBrief{Topic: "Go"},
	}, other, nil); err == nil {
		t.Fatal("an agent from another org must be refused")
	}

	run, err := svc.CreateRun(context.Background(), model.CreateCourseRunRequest{
		AgentID: agent.ID, Brief: model.CourseBrief{Topic: "Go", ChapterCount: 1},
	}, org, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, repo, run.ID, model.CourseRunCompleted)
	if _, err := svc.GetRun(context.Background(), run.ID, other); !errors.Is(err, ErrCourseRunNotFound) {
		t.Errorf("GetRun across orgs = %v, want not found", err)
	}
	if _, err := svc.ListChapters(context.Background(), run.ID, other); !errors.Is(err, ErrCourseRunNotFound) {
		t.Errorf("ListChapters across orgs = %v, want not found", err)
	}
}

func TestCourseRun_WrongBuiltinRefused(t *testing.T) {
	org := uuid.New()
	agent := courseAgent(org)
	agent.Metadata[model.MetadataKeyBuiltin] = model.BuiltinArticleWriter
	svc, _ := newCourseSvc(&fakeGen{}, agent)
	if _, err := svc.CreateRun(context.Background(), model.CreateCourseRunRequest{
		AgentID: agent.ID, Brief: model.CourseBrief{Topic: "Go"},
	}, org, nil); err == nil {
		t.Fatal("a non-course agent must be refused")
	}
}

func TestCourseRun_InvalidBriefRefused(t *testing.T) {
	org := uuid.New()
	agent := courseAgent(org)
	svc, _ := newCourseSvc(&fakeGen{}, agent)
	if _, err := svc.CreateRun(context.Background(), model.CreateCourseRunRequest{
		AgentID: agent.ID, Brief: model.CourseBrief{Topic: "Go", ChapterCount: 50},
	}, org, nil); err == nil {
		t.Fatal("too many chapters must be refused")
	}
}

func TestCourseRun_CancelAndFailure(t *testing.T) {
	org := uuid.New()
	agent := courseAgent(org)

	svc, repo := newCourseSvc(&fakeGen{block: true}, agent)
	run, err := svc.CreateRun(context.Background(), model.CreateCourseRunRequest{AgentID: agent.ID, Brief: model.CourseBrief{Topic: "Go"}}, org, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelRun(context.Background(), run.ID, org); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, repo, run.ID, model.CourseRunCancelled)
	if _, err := svc.CancelRun(context.Background(), run.ID, org); !errors.Is(err, ErrCourseNotCancellable) {
		t.Errorf("second cancel = %v, want not cancellable", err)
	}

	svc2, repo2 := newCourseSvc(&fakeGen{err: errors.New("model offline")}, agent)
	run2, _ := svc2.CreateRun(context.Background(), model.CreateCourseRunRequest{AgentID: agent.ID, Brief: model.CourseBrief{Topic: "Go"}}, org, nil)
	waitStatus(t, repo2, run2.ID, model.CourseRunFailed)
}

func TestCourseRun_NoNewRunsOnceShuttingDown(t *testing.T) {
	org := uuid.New()
	agent := courseAgent(org)
	svc, _ := newCourseSvc(&fakeGen{}, agent)
	svc.InterruptAll()
	if _, err := svc.CreateRun(context.Background(), model.CreateCourseRunRequest{AgentID: agent.ID, Brief: model.CourseBrief{Topic: "Go"}}, org, nil); err == nil {
		t.Error("no new runs once shutting down")
	}
	if n, err := svc.ResumeInterrupted(context.Background()); err != nil || n != 0 {
		t.Errorf("a server shutting down must not resume runs: %d, %v", n, err)
	}
}
