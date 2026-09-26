package platformtools

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/tools"
)

func registerWorkflows(reg *Registry, d Deps) {
	page := model.PaginationParams{Page: 1, PerPage: 100}

	loadWorkflows := func(ctx context.Context) ([]model.Workflow, error) {
		ident := MustIdentity(ctx)
		res, err := d.Workflows.ListByOrg(ctx, ident.OrgID, page)
		if err != nil {
			return nil, err
		}
		return res.Data, nil
	}

	resolveWorkflow := func(ctx context.Context, name string) (*model.Workflow, *Result, error) {
		wfs, err := loadWorkflows(ctx)
		if err != nil {
			return nil, nil, err
		}
		opts := make([]model.ClarifyOption, 0, len(wfs))
		for _, w := range wfs {
			opts = append(opts, model.ClarifyOption{Label: w.Name, Value: w.Name})
		}
		if name == "" {
			return nil, notFoundClarify("workflow", "", "name", opts), nil
		}
		m := ByName(wfs, name, func(w model.Workflow) string { return w.Name })
		if m.Found {
			w := m.Exact
			return &w, nil, nil
		}
		if len(m.Candidates) > 0 {
			return nil, clarifyFromMatch("workflow", name, "name", m.Candidates, func(w model.Workflow) string { return w.Name }), nil
		}
		return nil, notFoundClarify("workflow", name, "name", opts), nil
	}

	reg.Register(newTool(
		"workflow_list",
		"List workflows in this organisation.",
		"workflows", model.PermWorkflowsRead, false, true,
		tools.ObjectSchema(map[string]any{}),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			wfs, err := loadWorkflows(ctx)
			if err != nil {
				return nil, err
			}
			type row struct {
				Name        string `json:"name"`
				Description string `json:"description,omitempty"`
				Steps       int    `json:"steps"`
			}
			rows := make([]row, 0, len(wfs))
			var entities []model.EntityRef
			for _, w := range wfs {
				desc := ""
				if w.Description != nil {
					desc = *w.Description
				}
				rows = append(rows, row{Name: w.Name, Description: desc, Steps: len(w.Steps)})
				entities = append(entities, workflowRef(w))
			}
			return &Result{Data: map[string]any{"workflows": rows}, Entities: entities}, nil
		},
	))

	reg.Register(newTool(
		"workflow_get",
		"Explain a workflow step by step.",
		"workflows", model.PermWorkflowsRead, false, true,
		tools.ObjectSchema(map[string]any{
			"name": map[string]any{"type": "string"},
		}, "name"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			w, clar, err := resolveWorkflow(ctx, strArg(input, "name"))
			if err != nil || clar != nil {
				return clar, err
			}
			full, err := d.Workflows.GetByID(ctx, w.ID)
			if err == nil && full != nil {
				w = full
			}
			ident := MustIdentity(ctx)
			if w.OrgID != ident.OrgID {
				return nil, errNotInOrg
			}
			type step struct {
				Name     string   `json:"name"`
				Depends  []string `json:"depends_on,omitempty"`
				Template string   `json:"task,omitempty"`
			}
			steps := make([]step, 0, len(w.Steps))
			for _, s := range w.Steps {
				steps = append(steps, step{Name: s.Name, Depends: s.DependsOn, Template: s.InputTemplate})
			}
			desc := ""
			if w.Description != nil {
				desc = *w.Description
			}
			ref := workflowRef(*w)
			return &Result{
				Data:     map[string]any{"name": w.Name, "description": desc, "steps": steps},
				Entity:   &ref,
				Entities: []model.EntityRef{ref},
			}, nil
		},
	))

	reg.Register(newTool(
		"workflow_run",
		"Start a workflow by name. Optional params become the run input (for example branch, environment).",
		"workflows", model.PermWorkflowsExec, false, false,
		tools.ObjectSchema(map[string]any{
			"name":   map[string]any{"type": "string"},
			"params": map[string]any{"type": "object", "description": "Input parameters extracted from the user"},
		}, "name"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			w, clar, err := resolveWorkflow(ctx, strArg(input, "name"))
			if err != nil || clar != nil {
				return clar, err
			}
			params, _ := input["params"].(map[string]any)
			run, err := d.Workflows.Execute(ctx, w.ID, ident.OrgID, ident.UserID, model.ExecuteWorkflowRequest{Input: params})
			if err != nil {
				return nil, err
			}
			wref := workflowRef(*w)
			rref := model.EntityRef{Kind: model.EntityWorkflowRun, ID: run.ID.String(), Label: w.Name, Href: workflowHref(w.ID)}
			return &Result{
				Data:     map[string]any{"workflow": w.Name, "status": run.Status},
				Entity:   &rref,
				Entities: []model.EntityRef{wref, rref},
			}, nil
		},
	))

	reg.Register(newTool(
		"workflow_run_get",
		"Report a workflow run's status and per-step results.",
		"workflows", model.PermWorkflowsRead, false, true,
		tools.ObjectSchema(map[string]any{
			"run_id": map[string]any{"type": "string"},
		}, "run_id"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			id, err := uuid.Parse(strArg(input, "run_id"))
			if err != nil {
				return &Result{Missing: []string{"run_id"}, Question: "Which workflow run should I check?"}, nil
			}
			run, err := d.Workflows.GetRunByID(ctx, id)
			if err != nil {
				return nil, err
			}
			if run.OrgID != ident.OrgID {
				return nil, errNotInOrg
			}
			ref := model.EntityRef{Kind: model.EntityWorkflowRun, ID: run.ID.String(), Label: "workflow run", Href: workflowHref(run.WorkflowID)}
			return &Result{
				Data: map[string]any{
					"status":  run.Status,
					"outputs": run.Outputs,
					"error":   run.ErrorMessage,
				},
				Entity: &ref,
			}, nil
		},
	))

	reg.Register(newTool(
		"workflow_create",
		"Create a workflow from a name, description and a list of steps. Each step needs name, agent (by name) and the task prompt. Requires confirmation because it changes automation.",
		"workflows", model.PermWorkflowsCreate, true, false,
		tools.ObjectSchema(map[string]any{
			"name":        map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
			"steps": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":   map[string]any{"type": "string"},
						"agent":  map[string]any{"type": "string"},
						"prompt": map[string]any{"type": "string"},
					},
				},
			},
		}, "name", "steps"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			rawSteps, _ := input["steps"].([]any)
			if len(rawSteps) == 0 {
				return &Result{Missing: []string{"steps"}, Question: "What steps should this workflow run, and which agents?"}, nil
			}
			agentsPage, err := d.Agents.List(ctx, ident.OrgID, page, repository.AgentListFilter{})
			if err != nil {
				return nil, err
			}
			var steps []model.CreateWorkflowStepRequest
			for i, raw := range rawSteps {
				m, _ := raw.(map[string]any)
				stepName := strArg(m, "name")
				agentName := strArg(m, "agent")
				prompt := strArg(m, "prompt")
				if stepName == "" {
					stepName = fmt.Sprintf("step-%d", i+1)
				}
				match := ByName(agentsPage.Data, agentName, func(a model.Agent) string { return a.Name })
				if !match.Found {
					return clarifyFromMatch("agent", agentName, "agent", match.Candidates, func(a model.Agent) string { return a.Name }), nil
				}
				steps = append(steps, model.CreateWorkflowStepRequest{
					Name: stepName, AgentID: match.Exact.ID.String(), InputTemplate: prompt, Position: i,
				})
			}
			req := model.CreateWorkflowRequest{Name: strArg(input, "name"), Steps: steps}
			if dscr := strArg(input, "description"); dscr != "" {
				req.Description = &dscr
			}
			w, err := d.Workflows.Create(ctx, ident.OrgID, ident.UserID, req)
			if err != nil {
				return nil, err
			}
			ref := workflowRef(*w)
			return &Result{
				Data:   map[string]any{"name": w.Name, "steps": len(w.Steps)},
				Entity: &ref,
				Effect: fmt.Sprintf("create a new workflow named %q with %d steps", w.Name, len(w.Steps)),
			}, nil
		},
	))

	if d.MultiAgent != nil {
		reg.Register(newTool(
			"multi_agent_run",
			"Start a planner/executor/reviewer collaboration. Resolve agent names to the three roles.",
			"workflows", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"prompt":   map[string]any{"type": "string"},
				"planner":  map[string]any{"type": "string"},
				"executor": map[string]any{"type": "string"},
				"reviewer": map[string]any{"type": "string"},
			}, "prompt"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				agentsPage, err := d.Agents.List(ctx, ident.OrgID, page, repository.AgentListFilter{})
				if err != nil {
					return nil, err
				}
				resolve := func(key string) (*model.Agent, *Result) {
					m := ByName(agentsPage.Data, strArg(input, key), func(a model.Agent) string { return a.Name })
					if m.Found {
						a := m.Exact
						return &a, nil
					}
					return nil, clarifyFromMatch("agent", strArg(input, key), key, m.Candidates, func(a model.Agent) string { return a.Name })
				}
				planner, clar := resolve("planner")
				if clar != nil {
					if strArg(input, "planner") == "" {
						return &Result{Missing: []string{"planner"}, Question: "Which agent should plan?"}, nil
					}
					return clar, nil
				}
				executor, clar := resolve("executor")
				if clar != nil {
					if strArg(input, "executor") == "" {
						return &Result{Missing: []string{"executor"}, Question: "Which agent should execute?"}, nil
					}
					return clar, nil
				}
				reviewer, clar := resolve("reviewer")
				if clar != nil {
					if strArg(input, "reviewer") == "" {
						return &Result{Missing: []string{"reviewer"}, Question: "Which agent should review?"}, nil
					}
					return clar, nil
				}
				job, err := d.MultiAgent.RunJob(ctx, ident.OrgID, model.RunMultiAgentRequest{
					TaskPrompt: strArg(input, "prompt"),
					PlannerID:  planner.ID,
					ExecutorID: executor.ID,
					ReviewerID: reviewer.ID,
				})
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{
					"status": job.Status, "planner": planner.Name, "executor": executor.Name, "reviewer": reviewer.Name,
				}}, nil
			},
		))

		reg.Register(newTool(
			"agent_board",
			"Show the live agent board: who is idle, planning, executing, reviewing or publishing.",
			"workflows", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				board, err := d.MultiAgent.Board(ctx, ident.OrgID)
				if err != nil {
					return nil, err
				}
				type row struct {
					Name     string `json:"name"`
					Role     string `json:"role"`
					Activity string `json:"activity"`
					Doing    string `json:"doing,omitempty"`
				}
				rows := make([]row, 0, len(board))
				for _, e := range board {
					doing := ""
					if e.CurrentJobPrompt != nil {
						doing = *e.CurrentJobPrompt
					}
					rows = append(rows, row{Name: e.Name, Role: e.Role, Activity: e.Activity, Doing: doing})
				}
				return &Result{Data: map[string]any{"board": rows}}, nil
			},
		))
	}
}
