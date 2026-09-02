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
	got, err := s.ResolveProject(context.Background(), org, "", "")
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
	got, err := s.ResolveProject(context.Background(), org, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Missing != "project" || len(got.Options) != 2 {
		t.Fatalf("%+v", got)
	}

	got, err = s.ResolveProject(context.Background(), org, "Website", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != b.ID {
		t.Fatalf("named project = %s want %s", got.ProjectID, b.ID)
	}

	got, err = s.ResolveProject(context.Background(), org, "that project", a.ID.String())
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

func (s stubProjects) Create(context.Context, uuid.UUID, uuid.UUID, model.CreateProjectRequest) (*model.Project, error) {
	return nil, nil
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
