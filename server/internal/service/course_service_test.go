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
	stale    []uuid.UUID
}

func newMemCourseRepo() *memCourseRepo {
	return &memCourseRepo{runs: map[uuid.UUID]*model.CourseRun{}, chapters: map[uuid.UUID][]model.CourseChapter{}}
}

func (m *memCourseRepo) CreateRun(_ context.Context, r *model.CourseRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *r
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
	cp := *r
	return &cp, nil
}

func (m *memCourseRepo) ListRuns(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.CourseRun], error) {
	return &model.PaginatedResponse[model.CourseRun]{}, nil
}

func (m *memCourseRepo) UpdateSteps(_ context.Context, id uuid.UUID, s []model.CourseRunStep) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[id].Steps = s
	return nil
}

func (m *memCourseRepo) SaveOutline(_ context.Context, id uuid.UUID, o *model.CourseOutline, s []model.CourseSource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[id].Outline, m.runs[id].Sources = o, s
	return nil
}

func (m *memCourseRepo) SetCover(_ context.Context, id uuid.UUID, u string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[id].CoverURL = u
	return nil
}

func (m *memCourseRepo) AppendWarning(_ context.Context, id uuid.UUID, w string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[id].Warnings = append(m.runs[id].Warnings, w)
	return nil
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
	return true, nil
}

func (m *memCourseRepo) TouchHeartbeat(context.Context, uuid.UUID) error { return nil }

func (m *memCourseRepo) ListStaleRunning(context.Context, time.Time) ([]uuid.UUID, error) {
	return m.stale, nil
}

func (m *memCourseRepo) UpsertChapter(_ context.Context, ch *model.CourseChapter) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chapters[ch.RunID] = append(m.chapters[ch.RunID], *ch)
	return nil
}

func (m *memCourseRepo) ListChapters(_ context.Context, id uuid.UUID) ([]model.CourseChapter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.chapters[id], nil
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

func (a *courseAgents) FindByID(context.Context, uuid.UUID) (*model.Agent, error) { return a.agent, nil }

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

func TestCourseRun_InterruptAllAndReap(t *testing.T) {
	org := uuid.New()
	agent := courseAgent(org)
	svc, repo := newCourseSvc(&fakeGen{block: true}, agent)
	run, _ := svc.CreateRun(context.Background(), model.CreateCourseRunRequest{AgentID: agent.ID, Brief: model.CourseBrief{Topic: "Go"}}, org, nil)

	svc.InterruptAll()
	waitStatus(t, repo, run.ID, model.CourseRunFailed)
	if _, err := svc.CreateRun(context.Background(), model.CreateCourseRunRequest{AgentID: agent.ID, Brief: model.CourseBrief{Topic: "Go"}}, org, nil); err == nil {
		t.Error("no new runs once shutting down")
	}

	orphan := uuid.New()
	repo.runs[orphan] = &model.CourseRun{ID: orphan, OrgID: org, Status: model.CourseRunRunning}
	repo.stale = []uuid.UUID{orphan}
	if n, err := svc.ReapOrphans(context.Background()); err != nil || n != 1 {
		t.Fatalf("reap = %d, %v", n, err)
	}
	if repo.status(orphan) != model.CourseRunFailed {
		t.Error("orphan should be failed")
	}
}
