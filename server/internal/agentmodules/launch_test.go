package agentmodules_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/career"
	"github.com/jobshout/server/internal/images"
	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/pentester"
	"github.com/jobshout/server/internal/prreview"
	"github.com/jobshout/server/internal/research"
)

func launchIn(vals map[string]string, source string) agentmodule.LaunchInput {
	desc := "prior"
	return agentmodule.LaunchInput{
		OrgID:  uuid.New(),
		UserID: uuid.New(),
		Agent:  &model.Agent{ID: uuid.New()},
		Task:   &model.Task{ID: uuid.New(), Description: &desc},
		Values: vals,
		Source: source,
	}
}

type stubCareer struct {
	last model.EvaluateCareerRequest
	res  *model.CareerEvaluateResult
	err  error
}

func (s *stubCareer) Evaluate(_ context.Context, _, _ uuid.UUID, req model.EvaluateCareerRequest) (*model.CareerEvaluateResult, error) {
	s.last = req
	return s.res, s.err
}

func TestCareerLaunch(t *testing.T) {
	id := uuid.New()
	eval := &stubCareer{res: &model.CareerEvaluateResult{
		Evaluation: &model.CareerEvaluation{
			ID: id, ReportMarkdown: "report",
			Score: model.CareerScore{Recommendation: "apply"},
		},
	}}
	out, err := career.Module(eval).Launch(context.Background(), launchIn(map[string]string{
		"job_url": "https://boards.example/jobs/1", "mode": "full", "tailor_cv": "true",
	}, "task_manager"))
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.EvaluationID == nil || *out.EvaluationID != id {
		t.Fatalf("evaluation id = %+v", out)
	}
	if out.Status != "done" || !strings.Contains(out.Description, "prior") {
		t.Fatalf("out = %+v", out)
	}
	if eval.last.JobURL == "" || !eval.last.TailorCV {
		t.Fatalf("request = %+v", eval.last)
	}

	_, err = career.Module(eval).Launch(context.Background(), launchIn(nil, ""))
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("empty values err = %v", err)
	}

	eval.res = &model.CareerEvaluateResult{BlacklistHit: &model.CareerBlacklistEntry{Company: "Acme"}}
	_, err = career.Module(eval).Launch(context.Background(), launchIn(map[string]string{"jd_text": "role"}, ""))
	if err == nil || !strings.Contains(err.Error(), "blacklist") {
		t.Fatalf("blacklist err = %v", err)
	}
}

type stubMail struct {
	updated   int
	available bool
	bound     uuid.UUID
	enqueued  int
}

func (s *stubMail) UpdateConnection(context.Context, uuid.UUID, model.UpdateMailConnectionRequest) (*model.MailConnectionStatus, error) {
	s.updated++
	return &model.MailConnectionStatus{}, nil
}
func (s *stubMail) Available(context.Context, uuid.UUID) bool { return s.available }
func (s *stubMail) BindLaunchTask(_ uuid.UUID, taskID uuid.UUID) {
	s.bound = taskID
}
func (s *stubMail) EnqueueSync(context.Context, uuid.UUID) error {
	s.enqueued++
	return nil
}

func TestMailLaunch(t *testing.T) {
	svc := &stubMail{}
	in := launchIn(map[string]string{}, "chat")
	out, err := mail.Module(svc).Launch(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if svc.updated != 0 {
		t.Fatal("blank values must not PATCH the playbook")
	}
	if out.Status != "done" || out.SyncQueued || svc.enqueued != 0 {
		t.Fatalf("disconnected out=%+v enqueued=%d", out, svc.enqueued)
	}

	svc.available = true
	in = launchIn(map[string]string{"senders": "ops@example.com"}, "chat")
	out, err = mail.Module(svc).Launch(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if svc.updated != 1 || svc.enqueued != 1 || svc.bound != in.Task.ID {
		t.Fatalf("updated=%d enqueued=%d bound=%s", svc.updated, svc.enqueued, svc.bound)
	}
	if !out.SyncQueued || out.Status != "in_progress" {
		t.Fatalf("out = %+v", out)
	}
}

type stubBlog struct{ last model.GenerateBlogRequest }

func (s *stubBlog) Generate(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string, req model.GenerateBlogRequest) (*model.BlogRun, error) {
	s.last = req
	return &model.BlogRun{ID: uuid.New()}, nil
}

func TestArticleLaunch(t *testing.T) {
	w := &stubBlog{}
	in := launchIn(map[string]string{"topic": "edge AI", "context": "ops"}, "chat")
	out, err := blog.Module(w).Launch(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if out.RunID == nil || w.last.TaskID == nil || *w.last.TaskID != in.Task.ID {
		t.Fatalf("run=%+v req=%+v", out, w.last)
	}
	if w.last.Briefs[0].Topic != "edge AI" {
		t.Fatalf("topic = %q", w.last.Briefs[0].Topic)
	}
}

type stubResearch struct {
	n    int
	last research.Request
}

func (s *stubResearch) Available() bool { return true }
func (s *stubResearch) Research(_ context.Context, _ uuid.UUID, req research.Request, _ research.ProgressFunc) (*research.Brief, error) {
	s.n++
	s.last = req
	return &research.Brief{Topic: req.Topic, Summary: "ok"}, nil
}

func TestResearchLaunch(t *testing.T) {
	r := &stubResearch{}
	out, err := research.Module(r).Launch(context.Background(), launchIn(map[string]string{
		"topic": "k8s cost", "context": "spot",
	}, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	if r.n != 1 || r.last.Topic != "k8s cost" || out.Status != "done" || out.Brief == nil {
		t.Fatalf("n=%d last=%+v out=%+v", r.n, r.last, out)
	}
}

type stubPentest struct{ last model.CreatePentestRunRequest }

func (s *stubPentest) CreateRun(_ context.Context, req model.CreatePentestRunRequest, _ uuid.UUID, _ *uuid.UUID) (*model.PentestRun, error) {
	s.last = req
	return &model.PentestRun{ID: uuid.New()}, nil
}

func TestPentestLaunch(t *testing.T) {
	p := &stubPentest{}
	out, err := pentester.Module(p).Launch(context.Background(), launchIn(map[string]string{
		"target": "https://int.example.com", "max_budget": "1000",
	}, "task_manager"))
	if err != nil {
		t.Fatal(err)
	}
	if out.RunID == nil || p.last.ScanMode != "quick" || p.last.MaxBudget == nil || *p.last.MaxBudget != 1000 {
		t.Fatalf("last=%+v out=%+v", p.last, out)
	}
}

type stubReview struct{ last model.CreateReviewRunRequest }

func (s *stubReview) CreateRun(_ context.Context, req model.CreateReviewRunRequest, _ uuid.UUID, _ *uuid.UUID) (*model.ReviewRun, error) {
	s.last = req
	return &model.ReviewRun{ID: uuid.New()}, nil
}

func TestPRReviewLaunch(t *testing.T) {
	r := &stubReview{}
	out, err := prreview.Module(r).Launch(context.Background(), launchIn(map[string]string{
		"repo": "acme/api", "pr_number": "12",
	}, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	if out.RunID == nil || r.last.PRNumber != 12 || r.last.DryRun == nil || !*r.last.DryRun {
		t.Fatalf("last=%+v", r.last)
	}
}

type stubImages struct {
	source string
	n      int
}

func (s *stubImages) Enabled() bool { return true }
func (s *stubImages) Generate(_ context.Context, _, _ uuid.UUID, prompt, source string) (string, *uuid.UUID, error) {
	s.n++
	s.source = source
	id := uuid.New()
	return "https://img.example/" + prompt, &id, nil
}

func TestImageLaunch_RecordsTaskManagerSource(t *testing.T) {
	g := &stubImages{}
	out, err := images.Module(g).Launch(context.Background(), launchIn(map[string]string{
		"prompt": "a harbour at night",
	}, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	if g.n != 1 || g.source != "task_manager" {
		t.Fatalf("source = %q (chat launch must still record task_manager)", g.source)
	}
	if out.ImageURL == "" || out.Status != "done" {
		t.Fatalf("out = %+v", out)
	}
}
