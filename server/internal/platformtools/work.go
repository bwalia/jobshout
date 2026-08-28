package platformtools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/tools"
)

func registerWork(reg *Registry, d Deps) {
	page := model.PaginationParams{Page: 1, PerPage: 100}

	loadProjects := func(ctx context.Context) ([]model.Project, error) {
		ident := MustIdentity(ctx)
		res, err := d.Projects.List(ctx, ident.OrgID, page)
		if err != nil {
			return nil, err
		}
		return res.Data, nil
	}

	resolveProject := func(ctx context.Context, name string) (*model.Project, *Result, error) {
		projects, err := loadProjects(ctx)
		if err != nil {
			return nil, nil, err
		}
		opts := make([]model.ClarifyOption, 0, len(projects))
		for _, p := range projects {
			opts = append(opts, model.ClarifyOption{Label: p.Name, Value: p.Name})
		}
		if name == "" {
			if len(projects) == 1 {
				p := projects[0]
				return &p, nil, nil
			}
			return nil, notFoundClarify("project", "", "project", opts), nil
		}
		m := ByName(projects, name, func(p model.Project) string { return p.Name })
		if m.Found {
			p := m.Exact
			return &p, nil, nil
		}
		if len(m.Candidates) > 0 {
			return nil, clarifyFromMatch("project", name, "project", m.Candidates, func(p model.Project) string { return p.Name }), nil
		}
		return nil, notFoundClarify("project", name, "project", opts), nil
	}

	scopedTask := func(ctx context.Context, id uuid.UUID) (*model.Task, error) {
		ident := MustIdentity(ctx)
		t, err := d.Tasks.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		p, err := d.Projects.GetByID(ctx, t.ProjectID)
		if err != nil || p.OrgID != ident.OrgID {
			return nil, errNotInOrg
		}
		return t, nil
	}

	resolveTask := func(ctx context.Context, title string) (*model.Task, *Result, error) {
		ident := MustIdentity(ctx)
		res, err := d.Tasks.ListByOrg(ctx, ident.OrgID, page)
		if err != nil {
			return nil, nil, err
		}
		if title == "" {
			opts := make([]model.ClarifyOption, 0, min(8, len(res.Data)))
			for i, t := range res.Data {
				if i >= 8 {
					break
				}
				opts = append(opts, model.ClarifyOption{Label: t.Title, Value: t.Title})
			}
			return nil, notFoundClarify("task", "", "title", opts), nil
		}
		m := ByName(res.Data, title, func(t model.Task) string { return t.Title })
		if m.Found {
			t := m.Exact
			return &t, nil, nil
		}
		if len(m.Candidates) > 0 {
			return nil, clarifyFromMatch("task", title, "title", m.Candidates, func(t model.Task) string { return t.Title }), nil
		}
		opts := make([]model.ClarifyOption, 0, min(8, len(res.Data)))
		for i, t := range res.Data {
			if i >= 8 {
				break
			}
			opts = append(opts, model.ClarifyOption{Label: t.Title, Value: t.Title})
		}
		return nil, notFoundClarify("task", title, "title", opts), nil
	}

	reg.Register(newTool(
		"task_list",
		"List tasks, optionally filtered by project name or status (backlog, todo, in_progress, review, done).",
		"work", model.PermTasksRead, false, true,
		tools.ObjectSchema(map[string]any{
			"project": map[string]any{"type": "string"},
			"status":  map[string]any{"type": "string"},
		}),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			var tasks []model.Task
			if projName := strArg(input, "project"); projName != "" {
				p, clar, err := resolveProject(ctx, projName)
				if err != nil || clar != nil {
					return clar, err
				}
				res, err := d.Tasks.List(ctx, p.ID, page)
				if err != nil {
					return nil, err
				}
				tasks = res.Data
			} else {
				res, err := d.Tasks.ListByOrg(ctx, ident.OrgID, page)
				if err != nil {
					return nil, err
				}
				tasks = res.Data
			}
			status := strArg(input, "status")
			type row struct {
				Title    string `json:"title"`
				Status   string `json:"status"`
				Priority string `json:"priority"`
			}
			var rows []row
			var entities []model.EntityRef
			for _, t := range tasks {
				if status != "" && !strings.EqualFold(t.Status, status) {
					continue
				}
				rows = append(rows, row{Title: t.Title, Status: t.Status, Priority: t.Priority})
				entities = append(entities, taskRef(t))
			}
			return &Result{Data: map[string]any{"tasks": rows, "count": len(rows)}, Entities: entities}, nil
		},
	))

	reg.Register(newTool(
		"task_create",
		"Create a task that is persisted. Requires a title. Resolve project by name; if omitted and there is exactly one project, use it. Otherwise ask which project.",
		"work", model.PermTasksCreate, false, false,
		tools.ObjectSchema(map[string]any{
			"title":       map[string]any{"type": "string"},
			"project":     map[string]any{"type": "string", "description": "Project name"},
			"priority":    map[string]any{"type": "string", "enum": []any{"low", "medium", "high", "critical"}},
			"description": map[string]any{"type": "string"},
		}, "title"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			title := strArg(input, "title")
			if title == "" {
				return &Result{Missing: []string{"title"}, Question: "What should the task be called?"}, nil
			}
			p, clar, err := resolveProject(ctx, strArg(input, "project"))
			if err != nil || clar != nil {
				if clar != nil {
					clar.Missing = []string{"project"}
				}
				return clar, err
			}
			priority := strArg(input, "priority")
			if priority == "" {
				priority = "medium"
			}
			req := model.CreateTaskRequest{
				ProjectID: p.ID.String(),
				Title:     title,
				Priority:  priority,
			}
			if dscr := strArg(input, "description"); dscr != "" {
				req.Description = &dscr
			}
			t, err := d.Tasks.Create(ctx, ident.UserID, req)
			if err != nil {
				return nil, err
			}
			ref := taskRef(*t)
			pref := projectRef(*p)
			return &Result{
				Data:     map[string]any{"title": t.Title, "priority": t.Priority, "status": t.Status, "project": p.Name},
				Entity:   &ref,
				Entities: []model.EntityRef{ref, pref},
			}, nil
		},
	))

	reg.Register(newTool(
		"task_get",
		"Get a task's current status, priority and details by title.",
		"work", model.PermTasksRead, false, true,
		tools.ObjectSchema(map[string]any{
			"title": map[string]any{"type": "string"},
			"id":    map[string]any{"type": "string"},
		}),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			var t *model.Task
			if idStr := strArg(input, "id"); idStr != "" {
				id, err := uuid.Parse(idStr)
				if err != nil {
					return &Result{Question: "I couldn't read that task."}, nil
				}
				got, err := scopedTask(ctx, id)
				if err != nil {
					return nil, err
				}
				t = got
			} else {
				got, clar, err := resolveTask(ctx, strArg(input, "title"))
				if err != nil || clar != nil {
					return clar, err
				}
				t = got
			}
			ref := taskRef(*t)
			return &Result{
				Data: map[string]any{
					"title": t.Title, "status": t.Status, "priority": t.Priority,
				},
				Entity: &ref, Entities: []model.EntityRef{ref},
			}, nil
		},
	))

	reg.Register(newTool(
		"task_update",
		"Update a task's title, priority or assignment.",
		"work", model.PermTasksUpdate, false, false,
		tools.ObjectSchema(map[string]any{
			"title":     map[string]any{"type": "string", "description": "Current title to find the task"},
			"new_title": map[string]any{"type": "string"},
			"priority":  map[string]any{"type": "string", "enum": []any{"low", "medium", "high", "critical"}},
		}, "title"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			t, clar, err := resolveTask(ctx, strArg(input, "title"))
			if err != nil || clar != nil {
				return clar, err
			}
			req := model.UpdateTaskRequest{}
			if v := strArg(input, "new_title"); v != "" {
				req.Title = &v
			}
			if v := strArg(input, "priority"); v != "" {
				req.Priority = &v
			}
			updated, err := d.Tasks.Update(ctx, t.ID, req)
			if err != nil {
				return nil, err
			}
			ref := taskRef(*updated)
			return &Result{Data: map[string]any{"title": updated.Title, "priority": updated.Priority, "status": updated.Status}, Entity: &ref}, nil
		},
	))

	reg.Register(newTool(
		"task_transition",
		"Move a task to a new status: backlog, todo, in_progress, review, done.",
		"work", model.PermTasksUpdate, false, false,
		tools.ObjectSchema(map[string]any{
			"title":  map[string]any{"type": "string"},
			"status": map[string]any{"type": "string", "enum": []any{"backlog", "todo", "in_progress", "review", "done"}},
		}, "title", "status"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			t, clar, err := resolveTask(ctx, strArg(input, "title"))
			if err != nil || clar != nil {
				return clar, err
			}
			status := strArg(input, "status")
			if err := d.Tasks.Transition(ctx, t.ID, status); err != nil {
				return nil, err
			}
			ref := taskRef(*t)
			return &Result{Data: map[string]any{"title": t.Title, "status": status}, Entity: &ref}, nil
		},
	))

	reg.Register(newTool(
		"task_comment",
		"Add a comment to a task.",
		"work", model.PermTasksUpdate, false, false,
		tools.ObjectSchema(map[string]any{
			"title":   map[string]any{"type": "string"},
			"comment": map[string]any{"type": "string"},
		}, "title", "comment"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			t, clar, err := resolveTask(ctx, strArg(input, "title"))
			if err != nil || clar != nil {
				return clar, err
			}
			_, err = d.Tasks.AddComment(ctx, t.ID, ident.UserID, strArg(input, "comment"))
			if err != nil {
				return nil, err
			}
			ref := taskRef(*t)
			return &Result{Data: map[string]any{"title": t.Title, "commented": true}, Entity: &ref}, nil
		},
	))

	reg.Register(newTool(
		"task_delete",
		"Permanently delete a task. Requires confirmation.",
		"work", model.PermTasksDelete, true, false,
		tools.ObjectSchema(map[string]any{
			"title": map[string]any{"type": "string"},
		}, "title"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			t, clar, err := resolveTask(ctx, strArg(input, "title"))
			if err != nil || clar != nil {
				return clar, err
			}
			if err := d.Tasks.Delete(ctx, t.ID); err != nil {
				return nil, err
			}
			return &Result{Data: map[string]any{"deleted": t.Title}, Effect: fmt.Sprintf("permanently delete the task %q", t.Title)}, nil
		},
	))

	reg.Register(newTool(
		"project_list",
		"List projects in this organisation.",
		"work", model.PermProjectsRead, false, true,
		tools.ObjectSchema(map[string]any{}),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			projects, err := loadProjects(ctx)
			if err != nil {
				return nil, err
			}
			type row struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			}
			rows := make([]row, 0, len(projects))
			var entities []model.EntityRef
			for _, p := range projects {
				rows = append(rows, row{Name: p.Name, Status: p.Status})
				entities = append(entities, projectRef(p))
			}
			return &Result{Data: map[string]any{"projects": rows}, Entities: entities}, nil
		},
	))

	reg.Register(newTool(
		"project_create",
		"Create a project.",
		"work", model.PermProjectsCreate, false, false,
		tools.ObjectSchema(map[string]any{
			"name":        map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
			"priority":    map[string]any{"type": "string"},
		}, "name"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			req := model.CreateProjectRequest{Name: strArg(input, "name"), Priority: strArg(input, "priority")}
			if dscr := strArg(input, "description"); dscr != "" {
				req.Description = &dscr
			}
			p, err := d.Projects.Create(ctx, ident.OrgID, ident.UserID, req)
			if err != nil {
				return nil, err
			}
			ref := projectRef(*p)
			return &Result{Data: map[string]any{"name": p.Name, "status": p.Status}, Entity: &ref}, nil
		},
	))

	if d.Sprints != nil {
		reg.Register(newTool(
			"sprint_list",
			"List sprints.",
			"work", model.PermProjectsRead, false, true,
			tools.ObjectSchema(map[string]any{}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				list, err := d.Sprints.List(ctx, ident.OrgID)
				if err != nil {
					return nil, err
				}
				type row struct {
					Name   string `json:"name"`
					Status string `json:"status"`
				}
				rows := make([]row, 0, len(list))
				for _, s := range list {
					rows = append(rows, row{Name: s.Name, Status: s.Status})
				}
				return &Result{Data: map[string]any{"sprints": rows}}, nil
			},
		))

		reg.Register(newTool(
			"sprint_create",
			"Create a sprint.",
			"work", model.PermProjectsCreate, false, false,
			tools.ObjectSchema(map[string]any{
				"name": map[string]any{"type": "string"},
			}, "name"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				uid := ident.UserID
				s, err := d.Sprints.Create(ctx, ident.OrgID, &uid, model.CreateSprintRequest{Name: strArg(input, "name")})
				if err != nil {
					return nil, err
				}
				ref := model.EntityRef{Kind: model.EntitySprint, ID: s.ID.String(), Label: s.Name, Href: sprintHref()}
				return &Result{Data: map[string]any{"name": s.Name, "status": s.Status}, Entity: &ref}, nil
			},
		))

		reg.Register(newTool(
			"sprint_add_job",
			"Add a task (as a job reference) to a sprint by sprint name and task title. If the sprint API needs a job id, this records the intent via AddJob when a job_id is supplied; otherwise explain that sprints track collaboration jobs.",
			"work", model.PermProjectsUpdate, false, false,
			tools.ObjectSchema(map[string]any{
				"sprint": map[string]any{"type": "string"},
				"job_id": map[string]any{"type": "string", "description": "Multi-agent job id to add"},
			}, "sprint"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				list, err := d.Sprints.List(ctx, ident.OrgID)
				if err != nil {
					return nil, err
				}
				m := ByName(list, strArg(input, "sprint"), func(s model.Sprint) string { return s.Name })
				if !m.Found {
					return clarifyFromMatch("sprint", strArg(input, "sprint"), "sprint", m.Candidates, func(s model.Sprint) string { return s.Name }), nil
				}
				jobID := strArg(input, "job_id")
				if jobID == "" {
					return &Result{Missing: []string{"job_id"}, Question: "Which collaboration job should I add to " + m.Exact.Name + "?"}, nil
				}
				jid, err := uuid.Parse(jobID)
				if err != nil {
					return &Result{Question: "I need a specific job to add to the sprint."}, nil
				}
				if err := d.Sprints.AddJob(ctx, m.Exact.ID, model.AddSprintJobRequest{JobID: jid}); err != nil {
					return nil, err
				}
				ref := model.EntityRef{Kind: model.EntitySprint, ID: m.Exact.ID.String(), Label: m.Exact.Name, Href: sprintHref()}
				return &Result{Data: map[string]any{"sprint": m.Exact.Name, "added": true}, Entity: &ref}, nil
			},
		))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
