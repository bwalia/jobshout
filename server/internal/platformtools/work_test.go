package platformtools

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

type fakeProjects struct {
	items []model.Project
}

func (f *fakeProjects) Create(_ context.Context, orgID, _ uuid.UUID, req model.CreateProjectRequest) (*model.Project, error) {
	p := model.Project{ID: uuid.New(), OrgID: orgID, Name: req.Name}
	f.items = append(f.items, p)
	return &p, nil
}
func (f *fakeProjects) GetByID(_ context.Context, id uuid.UUID) (*model.Project, error) {
	for i := range f.items {
		if f.items[i].ID == id {
			return &f.items[i], nil
		}
	}
	return nil, service.ErrAgentNotFound
}
func (f *fakeProjects) List(_ context.Context, orgID uuid.UUID, _ model.PaginationParams) (*model.PaginatedResponse[model.Project], error) {
	var data []model.Project
	for _, p := range f.items {
		if p.OrgID == orgID {
			data = append(data, p)
		}
	}
	return &model.PaginatedResponse[model.Project]{Data: data, Total: len(data)}, nil
}
func (f *fakeProjects) Update(context.Context, uuid.UUID, model.UpdateProjectRequest) (*model.Project, error) {
	return nil, nil
}
func (f *fakeProjects) Delete(context.Context, uuid.UUID) error { return nil }

type fakeTasks struct {
	items []model.Task
}

func (f *fakeTasks) Create(_ context.Context, createdBy uuid.UUID, req model.CreateTaskRequest) (*model.Task, error) {
	pid, _ := uuid.Parse(req.ProjectID)
	tk := model.Task{
		ID: uuid.New(), ProjectID: pid, Title: req.Title, Status: "todo", Priority: req.Priority, CreatedBy: &createdBy,
	}
	f.items = append(f.items, tk)
	return &tk, nil
}
func (f *fakeTasks) GetByID(_ context.Context, id uuid.UUID) (*model.Task, error) {
	for i := range f.items {
		if f.items[i].ID == id {
			return &f.items[i], nil
		}
	}
	return nil, service.ErrAgentNotFound
}
func (f *fakeTasks) List(_ context.Context, projectID uuid.UUID, _ model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	var data []model.Task
	for _, tk := range f.items {
		if tk.ProjectID == projectID {
			data = append(data, tk)
		}
	}
	return &model.PaginatedResponse[model.Task]{Data: data, Total: len(data)}, nil
}
func (f *fakeTasks) ListByOrg(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	return &model.PaginatedResponse[model.Task]{Data: f.items, Total: len(f.items)}, nil
}
func (f *fakeTasks) ListComments(context.Context, uuid.UUID) ([]model.TaskComment, error) {
	return nil, nil
}
func (f *fakeTasks) AddComment(context.Context, uuid.UUID, uuid.UUID, string) (*model.TaskComment, error) {
	return nil, nil
}
func (f *fakeTasks) Update(context.Context, uuid.UUID, model.UpdateTaskRequest) (*model.Task, error) {
	return nil, nil
}
func (f *fakeTasks) Delete(context.Context, uuid.UUID) error               { return nil }
func (f *fakeTasks) Transition(context.Context, uuid.UUID, string) error   { return nil }
func (f *fakeTasks) Reorder(context.Context, uuid.UUID, string, int) error { return nil }

var _ service.ProjectService = (*fakeProjects)(nil)
var _ service.TaskService = (*fakeTasks)(nil)

func TestTaskCreate_Persists(t *testing.T) {
	org := uuid.New()
	user := uuid.New()
	projects := &fakeProjects{items: []model.Project{{ID: uuid.New(), OrgID: org, Name: "Website"}}}
	tasks := &fakeTasks{}
	reg := NewRegistryWithTools(Deps{Tasks: tasks, Projects: projects})
	tool, ok := reg.Get("task_create")
	if !ok {
		t.Fatal("task_create missing")
	}
	ctx := WithIdentity(context.Background(), Identity{OrgID: org, UserID: user})
	res, err := tool.Run(ctx, map[string]any{"title": "Fix login timeout"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.Entity == nil {
		t.Fatal("expected created task entity")
	}
	if len(tasks.items) != 1 {
		t.Fatalf("created %d tasks; want 1", len(tasks.items))
	}
	if tasks.items[0].Title != "Fix login timeout" {
		t.Fatalf("title = %q", tasks.items[0].Title)
	}
}

func TestTaskCreate_AsksForProject(t *testing.T) {
	org := uuid.New()
	projects := &fakeProjects{items: []model.Project{
		{ID: uuid.New(), OrgID: org, Name: "Website"},
		{ID: uuid.New(), OrgID: org, Name: "Mobile"},
	}}
	reg := NewRegistryWithTools(Deps{Tasks: &fakeTasks{}, Projects: projects})
	tool, _ := reg.Get("task_create")
	ctx := WithIdentity(context.Background(), Identity{OrgID: org, UserID: uuid.New()})
	res, err := tool.Run(ctx, map[string]any{"title": "Fix login timeout"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Missing) == 0 || res.Missing[0] != "project" {
		t.Fatalf("expected clarify for project, missing=%v", res.Missing)
	}
}

func TestTaskCreate_OrgScoped(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	projects := &fakeProjects{items: []model.Project{
		{ID: uuid.New(), OrgID: orgA, Name: "A-only"},
		{ID: uuid.New(), OrgID: orgB, Name: "B-only"},
	}}
	reg := NewRegistryWithTools(Deps{Tasks: &fakeTasks{}, Projects: projects})
	tool, _ := reg.Get("task_create")
	ctx := WithIdentity(context.Background(), Identity{OrgID: orgA, UserID: uuid.New()})
	res, err := tool.Run(ctx, map[string]any{"title": "x", "project": "B-only"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.Question == "" {
		t.Fatal("must not create in another org's project")
	}
}

func TestCatalogSearch_HidesUnauthorized(t *testing.T) {
	reg := NewRegistry()
	reg.Register(TestingTool("agent_delete", model.PermAgentsDelete, true, nilRun))
	registerCatalog(reg)
	ctx := WithPermissions(WithIdentity(context.Background(), Identity{OrgID: uuid.New(), UserID: uuid.New()}), map[string]bool{})
	tool, ok := reg.Get("catalog_search")
	if !ok {
		t.Fatal("catalog_search missing")
	}
	res, err := tool.Run(ctx, map[string]any{"query": "delete"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := res.Data.(map[string]any)
	switch names := data["names"].(type) {
	case []string:
		for _, n := range names {
			if n == "agent_delete" {
				t.Fatal("unauthorized tool disclosed")
			}
		}
	}
}
