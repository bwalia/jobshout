package platformtools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/tools"
)

func registerAgents(reg *Registry, d Deps) {
	page := model.PaginationParams{Page: 1, PerPage: 100}

	loadAgents := func(ctx context.Context) ([]model.Agent, error) {
		ident := MustIdentity(ctx)
		res, err := d.Agents.List(ctx, ident.OrgID, page, repository.AgentListFilter{})
		if err != nil {
			return nil, err
		}
		return res.Data, nil
	}

	resolveAgent := func(ctx context.Context, name string) (*model.Agent, *Result, error) {
		agents, err := loadAgents(ctx)
		if err != nil {
			return nil, nil, err
		}
		m := ByName(agents, name, func(a model.Agent) string { return a.Name })
		if m.Found {
			a := m.Exact
			return &a, nil, nil
		}
		opts := make([]model.ClarifyOption, 0, len(agents))
		for _, a := range agents {
			opts = append(opts, model.ClarifyOption{Label: a.Name + " — " + a.Role, Value: a.Name})
		}
		if len(m.Candidates) > 0 {
			return nil, clarifyFromMatch("agent", name, m.Candidates, func(a model.Agent) string { return a.Name }), nil
		}
		return nil, notFoundClarify("agent", name, opts), nil
	}

	scopedAgent := func(ctx context.Context, id uuid.UUID) (*model.Agent, error) {
		ident := MustIdentity(ctx)
		a, err := d.Agents.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if a.OrgID != ident.OrgID {
			return nil, errNotInOrg
		}
		return a, nil
	}

	reg.Register(newTool(
		"agent_list",
		"List the organisation's agents with role and status.",
		"agents", model.PermAgentsRead, false, true,
		tools.ObjectSchema(map[string]any{
			"search": map[string]any{"type": "string", "description": "Optional name or role filter"},
		}),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			agents, err := loadAgents(ctx)
			if err != nil {
				return nil, err
			}
			search := strings.ToLower(strArg(input, "search"))
			type row struct {
				Name   string `json:"name"`
				Role   string `json:"role"`
				Status string `json:"status"`
				Model  string `json:"model,omitempty"`
			}
			var rows []row
			var entities []model.EntityRef
			for _, a := range agents {
				if search != "" && !strings.Contains(strings.ToLower(a.Name+" "+a.Role), search) {
					continue
				}
				modelName := ""
				if a.ModelName != nil {
					modelName = *a.ModelName
				}
				rows = append(rows, row{Name: a.Name, Role: a.Role, Status: a.Status, Model: modelName})
				entities = append(entities, agentRef(a))
			}
			return &Result{Data: map[string]any{"agents": rows, "count": len(rows)}, Entities: entities}, nil
		},
	))

	reg.Register(newTool(
		"agent_get",
		"Describe one agent in detail: role, model, engine, system prompt, status.",
		"agents", model.PermAgentsRead, false, true,
		tools.ObjectSchema(map[string]any{
			"name": map[string]any{"type": "string", "description": "Agent name"},
		}, "name"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			a, clar, err := resolveAgent(ctx, strArg(input, "name"))
			if err != nil || clar != nil {
				return clar, err
			}
			desc := ""
			if a.Description != nil {
				desc = *a.Description
			}
			prompt := ""
			if a.SystemPrompt != nil {
				prompt = *a.SystemPrompt
			}
			modelName, provider := "", ""
			if a.ModelName != nil {
				modelName = *a.ModelName
			}
			if a.ModelProvider != nil {
				provider = *a.ModelProvider
			}
			ref := agentRef(*a)
			return &Result{
				Data: map[string]any{
					"name": a.Name, "role": a.Role, "status": a.Status,
					"description": desc, "model": modelName, "provider": provider,
					"engine": a.EngineType, "system_prompt": prompt,
				},
				Entity: &ref, Entities: []model.EntityRef{ref},
			}, nil
		},
	))

	reg.Register(newTool(
		"agent_create",
		"Create a new agent from a name, role and optional description and model.",
		"agents", model.PermAgentsCreate, false, false,
		tools.ObjectSchema(map[string]any{
			"name":        map[string]any{"type": "string"},
			"role":        map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
			"model":       map[string]any{"type": "string"},
			"provider":    map[string]any{"type": "string"},
			"system_prompt": map[string]any{"type": "string"},
		}, "name", "role"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			req := model.CreateAgentRequest{
				Name: strArg(input, "name"),
				Role: strArg(input, "role"),
			}
			if d := strArg(input, "description"); d != "" {
				req.Description = &d
			}
			if m := strArg(input, "model"); m != "" {
				req.ModelName = &m
			}
			if p := strArg(input, "provider"); p != "" {
				req.ModelProvider = &p
			}
			if s := strArg(input, "system_prompt"); s != "" {
				req.SystemPrompt = &s
			}
			a, err := d.Agents.Create(ctx, ident.OrgID, ident.UserID, req)
			if err != nil {
				return nil, err
			}
			ref := agentRef(*a)
			return &Result{Data: map[string]any{"name": a.Name, "role": a.Role, "status": a.Status}, Entity: &ref, Entities: []model.EntityRef{ref}}, nil
		},
	))

	reg.Register(newTool(
		"agent_update",
		"Update an agent's model, system prompt, description or role.",
		"agents", model.PermAgentsUpdate, false, false,
		tools.ObjectSchema(map[string]any{
			"name":          map[string]any{"type": "string", "description": "Current agent name"},
			"new_name":      map[string]any{"type": "string"},
			"role":          map[string]any{"type": "string"},
			"description":   map[string]any{"type": "string"},
			"model":         map[string]any{"type": "string"},
			"provider":      map[string]any{"type": "string"},
			"system_prompt": map[string]any{"type": "string"},
		}, "name"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			a, clar, err := resolveAgent(ctx, strArg(input, "name"))
			if err != nil || clar != nil {
				return clar, err
			}
			req := model.UpdateAgentRequest{}
			if v := strArg(input, "new_name"); v != "" {
				req.Name = &v
			}
			if v := strArg(input, "role"); v != "" {
				req.Role = &v
			}
			if v := strArg(input, "description"); v != "" {
				req.Description = &v
			}
			if v := strArg(input, "model"); v != "" {
				req.ModelName = &v
			}
			if v := strArg(input, "provider"); v != "" {
				req.ModelProvider = &v
			}
			if v := strArg(input, "system_prompt"); v != "" {
				req.SystemPrompt = &v
			}
			updated, err := d.Agents.Update(ctx, a.ID, req)
			if err != nil {
				return nil, err
			}
			ref := agentRef(*updated)
			return &Result{Data: map[string]any{"name": updated.Name, "role": updated.Role, "status": updated.Status}, Entity: &ref, Entities: []model.EntityRef{ref}}, nil
		},
	))

	reg.Register(newTool(
		"agent_set_status",
		"Activate, pause or set an agent offline. Status must be idle, active, paused or offline.",
		"agents", model.PermAgentsUpdate, false, false,
		tools.ObjectSchema(map[string]any{
			"name":   map[string]any{"type": "string"},
			"status": map[string]any{"type": "string", "enum": []any{"idle", "active", "paused", "offline"}},
		}, "name", "status"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			a, clar, err := resolveAgent(ctx, strArg(input, "name"))
			if err != nil || clar != nil {
				return clar, err
			}
			status := strArg(input, "status")
			if err := d.Agents.UpdateStatus(ctx, a.ID, status); err != nil {
				return nil, err
			}
			ref := agentRef(*a)
			ref.Label = a.Name
			return &Result{Data: map[string]any{"name": a.Name, "status": status}, Entity: &ref, Entities: []model.EntityRef{ref}}, nil
		},
	))

	reg.Register(newTool(
		"agent_delete",
		"Permanently delete an agent and its executions. Requires confirmation.",
		"agents", model.PermAgentsDelete, true, false,
		tools.ObjectSchema(map[string]any{
			"name": map[string]any{"type": "string"},
		}, "name"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			a, clar, err := resolveAgent(ctx, strArg(input, "name"))
			if err != nil || clar != nil {
				return clar, err
			}
			if err := d.Agents.Delete(ctx, a.ID); err != nil {
				return nil, err
			}
			return &Result{
				Data:   map[string]any{"deleted": a.Name},
				Effect: fmt.Sprintf("permanently delete the %s agent", a.Name),
			}, nil
		},
	))

	reg.Register(newTool(
		"agent_execute",
		"Run a one-off task on a named agent. If no agent is named, pick the best match from the org's agents and say why. The prompt is the work to do.",
		"agents", model.PermAgentsExecute, false, false,
		tools.ObjectSchema(map[string]any{
			"name":   map[string]any{"type": "string", "description": "Agent name. Omit to auto-select."},
			"prompt": map[string]any{"type": "string", "description": "The task to run"},
			"reason": map[string]any{"type": "string", "description": "Why this agent, when auto-selecting"},
		}, "prompt"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			prompt := strArg(input, "prompt")
			if prompt == "" {
				return &Result{Missing: []string{"prompt"}, Question: "What should the agent do?"}, nil
			}
			var agent *model.Agent
			name := strArg(input, "name")
			if name != "" {
				a, clar, err := resolveAgent(ctx, name)
				if err != nil || clar != nil {
					return clar, err
				}
				agent = a
			} else {
				agents, err := loadAgents(ctx)
				if err != nil {
					return nil, err
				}
				if len(agents) == 0 {
					return &Result{Question: "There are no agents in this organisation yet. Create one first?", Missing: []string{"agent"}}, nil
				}
				if len(agents) == 1 {
					agent = &agents[0]
				} else {
					opts := make([]model.ClarifyOption, 0, len(agents))
					for _, a := range agents {
						opts = append(opts, model.ClarifyOption{Label: a.Name + " — " + a.Role, Value: a.Name})
					}
					return &Result{
						Missing:  []string{"agent"},
						Options:  opts,
						Question: "Which agent should handle that?",
					}, nil
				}
			}
			exec, err := d.Exec.Execute(ctx, ident.OrgID, agent.ID, model.ExecuteAgentRequest{Prompt: prompt})
			if err != nil {
				return nil, err
			}
			out := ""
			if exec.Output != nil {
				out = *exec.Output
			}
			errMsg := ""
			if exec.ErrorMessage != nil {
				errMsg = *exec.ErrorMessage
			}
			aref := agentRef(*agent)
			eref := executionRef(*exec, agent.Name)
			return &Result{
				Data: map[string]any{
					"agent": agent.Name, "status": exec.Status,
					"output": out, "error": errMsg, "reason": strArg(input, "reason"),
				},
				Entity:   &eref,
				Entities: []model.EntityRef{aref, eref},
			}, nil
		},
	))

	if d.Exec != nil {
		reg.Register(newTool(
			"execution_get",
			"Report the status and result of an agent execution. Use the current context execution when id is omitted.",
			"agents", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{
				"execution_id": map[string]any{"type": "string"},
				"agent_name":   map[string]any{"type": "string"},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				idStr := strArg(input, "execution_id")
				if idStr == "" {
					return &Result{Missing: []string{"execution"}, Question: "Which run should I look up? Name the agent or refer to the last one."}, nil
				}
				id, err := uuid.Parse(idStr)
				if err != nil {
					return &Result{Question: "I need a specific run to look up."}, nil
				}
				exec, err := d.Exec.GetByID(ctx, id)
				if err != nil || exec == nil {
					return nil, fmt.Errorf("that run was not found")
				}
				if exec.OrgID != ident.OrgID {
					return nil, errNotInOrg
				}
				agentName := strArg(input, "agent_name")
				if a, err := scopedAgent(ctx, exec.AgentID); err == nil {
					agentName = a.Name
				}
				out := ""
				if exec.Output != nil {
					out = *exec.Output
				}
				errMsg := ""
				if exec.ErrorMessage != nil {
					errMsg = *exec.ErrorMessage
				}
				ref := executionRef(*exec, agentName)
				return &Result{
					Data: map[string]any{
						"agent": agentName, "status": exec.Status,
						"output": out, "error": errMsg,
					},
					Entity: &ref, Entities: []model.EntityRef{ref},
				}, nil
			},
		))

		reg.Register(newTool(
			"execution_cancel",
			"Cancel a running execution. Requires confirmation.",
			"agents", model.PermAgentsExecute, true, false,
			tools.ObjectSchema(map[string]any{
				"execution_id": map[string]any{"type": "string"},
			}, "execution_id"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				id, err := uuid.Parse(strArg(input, "execution_id"))
				if err != nil {
					return &Result{Missing: []string{"execution"}, Question: "Which run should I stop?"}, nil
				}
				exec, err := d.Exec.Cancel(ctx, ident.OrgID, id)
				if err != nil {
					return nil, err
				}
				ref := executionRef(*exec, "run")
				return &Result{
					Data:   map[string]any{"status": exec.Status},
					Entity: &ref,
					Effect: "stop the running execution",
				}, nil
			},
		))

		reg.Register(newTool(
			"execution_list",
			"List recent executions for a named agent.",
			"agents", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{
				"name":  map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
			}, "name"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				a, clar, err := resolveAgent(ctx, strArg(input, "name"))
				if err != nil || clar != nil {
					return clar, err
				}
				limit := intArg(input, "limit", 10)
				if limit <= 0 || limit > 50 {
					limit = 10
				}
				page, err := d.Exec.ListByAgent(ctx, a.ID, model.PaginationParams{Page: 1, PerPage: limit})
				if err != nil {
					return nil, err
				}
				type row struct {
					Status string `json:"status"`
					Prompt string `json:"prompt"`
				}
				rows := make([]row, 0, len(page.Data))
				for _, e := range page.Data {
					p := e.InputPrompt
					if len(p) > 120 {
						p = p[:120] + "…"
					}
					rows = append(rows, row{Status: e.Status, Prompt: p})
				}
				ref := agentRef(*a)
				return &Result{Data: map[string]any{"agent": a.Name, "runs": rows, "count": page.Total}, Entity: &ref}, nil
			},
		))
	}

	if d.Goals != nil {
		reg.Register(newTool(
			"goal_create",
			"Give an agent a long-running autonomous goal.",
			"agents", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"name": map[string]any{"type": "string", "description": "Agent name"},
				"goal": map[string]any{"type": "string"},
			}, "name", "goal"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				a, clar, err := resolveAgent(ctx, strArg(input, "name"))
				if err != nil || clar != nil {
					return clar, err
				}
				g, err := d.Goals.CreateGoal(ctx, ident.OrgID, a.ID, model.CreateGoalRequest{GoalText: strArg(input, "goal")})
				if err != nil {
					return nil, err
				}
				ref := model.EntityRef{Kind: model.EntityGoal, ID: g.ID.String(), Label: a.Name + " goal", Href: agentHref(a.ID)}
				return &Result{Data: map[string]any{"agent": a.Name, "goal": g.GoalText, "status": g.Status}, Entity: &ref}, nil
			},
		))

		reg.Register(newTool(
			"goal_get",
			"Report progress on an agent's most recent goal, or a specific goal id.",
			"agents", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{
				"name":    map[string]any{"type": "string"},
				"goal_id": map[string]any{"type": "string"},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				if idStr := strArg(input, "goal_id"); idStr != "" {
					id, err := uuid.Parse(idStr)
					if err != nil {
						return &Result{Question: "I couldn't read that goal."}, nil
					}
					g, err := d.Goals.GetGoal(ctx, id)
					if err != nil {
						return nil, err
					}
					ident := MustIdentity(ctx)
					if g.OrgID != ident.OrgID {
						return nil, errNotInOrg
					}
					return &Result{Data: map[string]any{"goal": g.GoalText, "status": g.Status, "iterations": g.Iterations}}, nil
				}
				a, clar, err := resolveAgent(ctx, strArg(input, "name"))
				if err != nil || clar != nil {
					if strArg(input, "name") == "" {
						return &Result{Missing: []string{"agent"}, Question: "Which agent's goal should I check?"}, nil
					}
					return clar, err
				}
				page, err := d.Goals.ListGoals(ctx, a.ID, model.PaginationParams{Page: 1, PerPage: 1})
				if err != nil {
					return nil, err
				}
				if len(page.Data) == 0 {
					return &Result{Data: map[string]any{"agent": a.Name, "goals": []any{}}}, nil
				}
				g := page.Data[0]
				return &Result{Data: map[string]any{"agent": a.Name, "goal": g.GoalText, "status": g.Status, "iterations": g.Iterations}}, nil
			},
		))
	}

	reg.Register(newTool(
		"agent_set_manager",
		"Assign an agent to report to another agent (org chart manager).",
		"agents", model.PermAgentsUpdate, false, false,
		tools.ObjectSchema(map[string]any{
			"name":    map[string]any{"type": "string", "description": "Agent to reassign"},
			"manager": map[string]any{"type": "string", "description": "Manager agent name"},
		}, "name", "manager"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			a, clar, err := resolveAgent(ctx, strArg(input, "name"))
			if err != nil || clar != nil {
				return clar, err
			}
			mgr, clar, err := resolveAgent(ctx, strArg(input, "manager"))
			if err != nil || clar != nil {
				return clar, err
			}
			mid := mgr.ID.String()
			updated, err := d.Agents.Update(ctx, a.ID, model.UpdateAgentRequest{ManagerID: &mid})
			if err != nil {
				return nil, err
			}
			ref := agentRef(*updated)
			return &Result{Data: map[string]any{"agent": a.Name, "manager": mgr.Name}, Entity: &ref}, nil
		},
	))
}
