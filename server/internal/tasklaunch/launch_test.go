package tasklaunch

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	_ "github.com/jobshout/server/internal/agentmodules"
	"github.com/jobshout/server/internal/model"
)

func TestTitleFrom_Specialists(t *testing.T) {
	title, desc := TitleFrom(model.BuiltinResearcher, map[string]string{
		"topic": "Kubernetes cost", "context": "spot nodes",
	})
	if title != "Research: Kubernetes cost" {
		t.Fatalf("title %q", title)
	}
	if desc == "" || desc[:5] != "Topic" {
		t.Fatalf("desc %q", desc)
	}

	title, _ = TitleFrom(model.BuiltinArticleWriter, map[string]string{"topic": "edge AI"})
	if title != "Write: edge AI" {
		t.Fatalf("article title %q", title)
	}

	title, _ = TitleFrom(model.BuiltinMail, map[string]string{})
	if title != "Mail: sync inbox and draft" {
		t.Fatalf("mail title %q", title)
	}

	title, desc = TitleFrom(model.BuiltinCareerOps, map[string]string{
		"job_url": "https://boards.greenhouse.io/acme/jobs/1",
	})
	if title != "Evaluate: https://boards.greenhouse.io/acme/jobs/1" {
		t.Fatalf("career title %q", title)
	}
	if !strings.Contains(desc, "URL:") {
		t.Fatalf("career desc %q", desc)
	}
}

func TestResolveProject_OneProject(t *testing.T) {
	org := uuid.New()
	p := model.Project{ID: uuid.New(), OrgID: org, Name: "Inbox"}
	s := &Service{Projects: stubProjects{items: []model.Project{p}}}
	got, err := s.ResolveProject(context.Background(), org, uuid.New(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != p.ID || got.Missing != "" {
		t.Fatalf("%+v", got)
	}
}

func TestResolveProject_TwoProjectsInterview(t *testing.T) {
	org := uuid.New()
	a := model.Project{ID: uuid.New(), OrgID: org, Name: "Platform"}
	b := model.Project{ID: uuid.New(), OrgID: org, Name: "Website"}
	s := &Service{Projects: stubProjects{items: []model.Project{a, b}}}
	got, err := s.ResolveProject(context.Background(), org, uuid.New(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Missing != "project" || len(got.Options) != 2 {
		t.Fatalf("%+v", got)
	}

	got, err = s.ResolveProject(context.Background(), org, uuid.New(), "Website", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != b.ID {
		t.Fatalf("named project = %s want %s", got.ProjectID, b.ID)
	}

	got, err = s.ResolveProject(context.Background(), org, uuid.New(), "that project", a.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != a.ID {
		t.Fatalf("last project = %s", got.ProjectID)
	}
}

type stubProjects struct {
	items []model.Project
}

func (s stubProjects) Create(_ context.Context, orgID, ownerID uuid.UUID, req model.CreateProjectRequest) (*model.Project, error) {
	return &model.Project{ID: uuid.New(), OrgID: orgID, OwnerID: &ownerID, Name: req.Name}, nil
}
func (s stubProjects) GetByID(_ context.Context, id uuid.UUID) (*model.Project, error) {
	for i := range s.items {
		if s.items[i].ID == id {
			return &s.items[i], nil
		}
	}
	return nil, nil
}
func (s stubProjects) List(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.Project], error) {
	return &model.PaginatedResponse[model.Project]{Data: s.items, Total: len(s.items)}, nil
}
func (s stubProjects) Update(context.Context, uuid.UUID, model.UpdateProjectRequest) (*model.Project, error) {
	return nil, nil
}
func (s stubProjects) Delete(context.Context, uuid.UUID) error { return nil }

func TestResolveProject_NoProjectsOffersCreate(t *testing.T) {
	org := uuid.New()
	s := &Service{Projects: stubProjects{}}

	// Nothing to go on: must offer a way forward, not a dead end.
	got, err := s.ResolveProject(context.Background(), org, uuid.New(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Missing != "project" {
		t.Fatalf("want a clarify, got %+v", got)
	}
	if len(got.Options) != 1 || got.Options[0].Value != NewProjectPrefix+"General" {
		t.Fatalf("want a create option, got %+v", got.Options)
	}

	// Answering that option creates the project and continues.
	got, err = s.ResolveProject(context.Background(), org, uuid.New(), NewProjectPrefix+"Acme", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Missing != "" || got.ProjectID == uuid.Nil {
		t.Fatalf("want a resolved project, got %+v", got)
	}
}

func TestNewProjectName(t *testing.T) {
	cases := map[string]string{
		"new:Acme":          "Acme",
		"new:":              "General",
		"NEW:Acme":          "Acme",
		"Acme":              "Acme",
		"":                  "",
		"that":              "",
		"it":                "",
		"x":                 "",
		uuid.New().String(): "",
	}
	for in, want := range cases {
		if got := newProjectName(in); got != want {
			t.Errorf("newProjectName(%q) = %q, want %q", in, got, want)
		}
	}
}
