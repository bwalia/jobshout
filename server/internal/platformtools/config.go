package platformtools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/tools"
)

func registerConfig(reg *Registry, d Deps) {
	if d.LLMProviders != nil {
		reg.Register(newTool(
			"llm_provider_list",
			"List configured LLM providers and their default models.",
			"config", "", false, true,
			tools.ObjectSchema(map[string]any{}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				list, err := d.LLMProviders.List(ctx, ident.OrgID)
				if err != nil {
					return nil, err
				}
				type row struct {
					Name    string `json:"name"`
					Type    string `json:"type"`
					Model   string `json:"model"`
					Default bool   `json:"default"`
					Active  bool   `json:"active"`
				}
				rows := make([]row, 0, len(list))
				for _, p := range list {
					rows = append(rows, row{Name: p.Name, Type: p.ProviderType, Model: p.DefaultModel, Default: p.IsDefault, Active: p.IsActive})
				}
				return &Result{Data: map[string]any{"providers": rows}}, nil
			},
		))
	}

	if d.Scheduler != nil {
		reg.Register(newTool(
			"schedule_create",
			"Schedule recurring work. task_type is agent, workflow, multi_agent or blog (blog = automatic article writing). For agent/workflow pass the agent/workflow name. Use cron (e.g. 0 */5 * * * for every 5 hours) or a preset such as weekdays_9am.",
			"config", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"name":      map[string]any{"type": "string"},
				"task_type": map[string]any{"type": "string", "enum": []any{"agent", "workflow", "multi_agent", "blog"}},
				"prompt":    map[string]any{"type": "string"},
				"preset":    map[string]any{"type": "string", "description": "e.g. weekdays_9am, every_morning_9am, hourly"},
				"cron":      map[string]any{"type": "string", "description": "standard 5-field cron, e.g. 0 */5 * * *"},
				"agent":     map[string]any{"type": "string", "description": "agent name, required when task_type=agent"},
				"workflow":  map[string]any{"type": "string", "description": "workflow name, required when task_type=workflow"},
			}, "name", "task_type"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				uid := ident.UserID
				taskType := strArg(input, "task_type")

				preset := strArg(input, "preset")
				cronExpr := strArg(input, "cron")
				if preset != "" {
					cronExpr = mapPreset(preset)
				}
				if cronExpr == "" {
					preset = "weekdays_9am"
					cronExpr = mapPreset(preset)
				}
				sched, err := scheduleCronParser.Parse(cronExpr)
				if err != nil {
					return &Result{Missing: []string{"cron"}, Question: fmt.Sprintf("%q is not a valid cron expression. When should this run?", cronExpr)}, nil
				}

				t := &model.ScheduledTask{
					ID:             uuid.New(),
					OrgID:          ident.OrgID,
					Name:           strArg(input, "name"),
					TaskType:       taskType,
					InputPrompt:    strArg(input, "prompt"),
					ScheduleType:   "cron",
					CronExpression: &cronExpr,
					Status:         "active",
					CreatedBy:      &uid,
					Priority:       "medium",
					Tags:           []string{},
				}
				if preset != "" {
					t.SchedulePreset = &preset
				}
				// The runner only advances next_run_at after a run, so the
				// initial value must be set here or the schedule never fires.
				next := sched.Next(time.Now())
				t.NextRunAt = &next

				// Link the dispatch target: an agent/workflow schedule with no
				// target row is created fine but can never run.
				switch taskType {
				case "agent":
					if d.Agents == nil {
						return nil, fmt.Errorf("agent schedules are not available")
					}
					agents, err := listAllAgents(ctx, d, ident.OrgID)
					if err != nil {
						return nil, err
					}
					m := ByName(agents, strArg(input, "agent"), func(a model.Agent) string { return a.Name })
					if !m.Found {
						if len(m.Candidates) > 0 {
							return clarifyFromMatch("agent", strArg(input, "agent"), "agent", m.Candidates, func(a model.Agent) string { return a.Name }), nil
						}
						opts := make([]model.ClarifyOption, 0, len(agents))
						for _, a := range agents {
							opts = append(opts, model.ClarifyOption{Label: a.Name + " — " + a.Role, Value: a.Name})
						}
						return notFoundClarify("agent", strArg(input, "agent"), "agent", opts), nil
					}
					t.AgentID = &m.Exact.ID
				case "workflow":
					if d.Workflows == nil {
						return nil, fmt.Errorf("workflow schedules are not available")
					}
					res, err := d.Workflows.ListByOrg(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 100})
					if err != nil {
						return nil, err
					}
					m := ByName(res.Data, strArg(input, "workflow"), func(w model.Workflow) string { return w.Name })
					if !m.Found {
						if len(m.Candidates) > 0 {
							return clarifyFromMatch("workflow", strArg(input, "workflow"), "workflow", m.Candidates, func(w model.Workflow) string { return w.Name }), nil
						}
						opts := make([]model.ClarifyOption, 0, len(res.Data))
						for _, w := range res.Data {
							opts = append(opts, model.ClarifyOption{Label: w.Name, Value: w.Name})
						}
						return notFoundClarify("workflow", strArg(input, "workflow"), "workflow", opts), nil
					}
					t.WorkflowID = &m.Exact.ID
				}

				if err := d.Scheduler.CreateTask(ctx, t); err != nil {
					return nil, err
				}
				ref := model.EntityRef{Kind: model.EntitySchedule, ID: t.ID.String(), Label: t.Name, Href: "/scheduler"}
				return &Result{Data: map[string]any{
					"name": t.Name, "cron": cronExpr, "type": t.TaskType,
					"next_run_at": next.UTC().Format(time.RFC3339),
				}, Entity: &ref}, nil
			},
		))
		reg.Register(newTool(
			"schedule_list",
			"List scheduled tasks.",
			"config", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				res, err := d.Scheduler.ListTasks(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 50})
				if err != nil {
					return nil, err
				}
				type row struct {
					Name   string `json:"name"`
					Type   string `json:"type"`
					Status string `json:"status"`
					Cron   string `json:"cron,omitempty"`
				}
				rows := make([]row, 0, len(res.Data))
				for _, t := range res.Data {
					cron := ""
					if t.CronExpression != nil {
						cron = *t.CronExpression
					}
					rows = append(rows, row{Name: t.Name, Type: t.TaskType, Status: t.Status, Cron: cron})
				}
				return &Result{Data: map[string]any{"schedules": rows}}, nil
			},
		))
	}

	if d.Skills != nil {
		reg.Register(newTool(
			"skill_list",
			"List skills in the registry.",
			"config", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				list, err := d.Skills.List(ctx, ident.OrgID)
				if err != nil {
					return nil, err
				}
				type row struct {
					Name string `json:"name"`
					Slug string `json:"slug"`
					Kind string `json:"kind"`
				}
				rows := make([]row, 0, len(list))
				for _, s := range list {
					rows = append(rows, row{Name: s.Name, Slug: s.Slug, Kind: s.Kind})
				}
				return &Result{Data: map[string]any{"skills": rows}}, nil
			},
		))
		reg.Register(newTool(
			"skill_enable",
			"Enable a skill on a named agent.",
			"config", model.PermAgentsUpdate, false, false,
			tools.ObjectSchema(map[string]any{
				"skill": map[string]any{"type": "string"},
				"agent": map[string]any{"type": "string"},
			}, "skill", "agent"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				skills, err := d.Skills.List(ctx, ident.OrgID)
				if err != nil {
					return nil, err
				}
				sm := ByName(skills, strArg(input, "skill"), func(s model.Skill) string { return s.Name })
				if !sm.Found {
					sm = ByName(skills, strArg(input, "skill"), func(s model.Skill) string { return s.Slug })
				}
				if !sm.Found {
					return clarifyFromMatch("skill", strArg(input, "skill"), "skill", sm.Candidates, func(s model.Skill) string { return s.Name }), nil
				}
				agents, err := d.Agents.List(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 100}, repository.AgentListFilter{})
				if err != nil {
					return nil, err
				}
				am := ByName(agents.Data, strArg(input, "agent"), func(a model.Agent) string { return a.Name })
				if !am.Found {
					return clarifyFromMatch("agent", strArg(input, "agent"), "agent", am.Candidates, func(a model.Agent) string { return a.Name }), nil
				}
				if err := d.Skills.EnableForAgent(ctx, am.Exact.ID, sm.Exact.ID, nil); err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"skill": sm.Exact.Name, "agent": am.Exact.Name}}, nil
			},
		))
	}

	if d.Plugins != nil {
		reg.Register(newTool(
			"plugin_list",
			"List plugins.",
			"config", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				res, err := d.Plugins.ListByOrg(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 50})
				if err != nil {
					return nil, err
				}
				type row struct {
					Name   string `json:"name"`
					Status string `json:"status"`
				}
				rows := make([]row, 0, len(res.Data))
				for _, p := range res.Data {
					rows = append(rows, row{Name: p.Name, Status: p.Status})
				}
				return &Result{Data: map[string]any{"plugins": rows}}, nil
			},
		))
		reg.Register(newTool(
			"plugin_execute",
			"Execute a named plugin.",
			"config", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"name":  map[string]any{"type": "string"},
				"input": map[string]any{"type": "object"},
			}, "name"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				res, err := d.Plugins.ListByOrg(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 50})
				if err != nil {
					return nil, err
				}
				m := ByName(res.Data, strArg(input, "name"), func(p model.Plugin) string { return p.Name })
				if !m.Found {
					return clarifyFromMatch("plugin", strArg(input, "name"), "name", m.Candidates, func(p model.Plugin) string { return p.Name }), nil
				}
				in, _ := input["input"].(map[string]any)
				exec, err := d.Plugins.Execute(ctx, m.Exact.ID, ident.OrgID, model.ExecutePluginRequest{Input: in})
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"plugin": m.Exact.Name, "status": exec.Status}}, nil
			},
		))
	}

	if d.MCP != nil {
		reg.Register(newTool(
			"mcp_list_tools",
			"List MCP servers and the tools they expose.",
			"config", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{
				"server": map[string]any{"type": "string"},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				servers, err := d.MCP.List(ctx, ident.OrgID)
				if err != nil {
					return nil, err
				}
				name := strArg(input, "server")
				type srow struct {
					Name  string   `json:"name"`
					URL   string   `json:"url"`
					Tools []string `json:"tools,omitempty"`
				}
				rows := make([]srow, 0, len(servers))
				for _, s := range servers {
					if name != "" && !strings.EqualFold(s.Name, name) {
						continue
					}
					if s.OrgID != ident.OrgID {
						continue
					}
					r := srow{Name: s.Name, URL: s.URL}
					if tools, err := d.MCP.ListTools(ctx, s.ID); err == nil {
						for _, t := range tools {
							r.Tools = append(r.Tools, t.Name)
						}
					}
					rows = append(rows, r)
				}
				return &Result{Data: map[string]any{"servers": rows}}, nil
			},
		))
	}

	if d.Pool != nil && d.Agents != nil {
		reg.Register(newTool(
			"knowledge_add",
			"Add a text document to an agent's knowledge base.",
			"config", model.PermAgentsUpdate, false, false,
			tools.ObjectSchema(map[string]any{
				"agent":    map[string]any{"type": "string"},
				"filename": map[string]any{"type": "string"},
				"content":  map[string]any{"type": "string"},
			}, "agent", "filename", "content"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				agents, err := d.Agents.List(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 100}, repository.AgentListFilter{})
				if err != nil {
					return nil, err
				}
				m := ByName(agents.Data, strArg(input, "agent"), func(a model.Agent) string { return a.Name })
				if !m.Found {
					return clarifyFromMatch("agent", strArg(input, "agent"), "agent", m.Candidates, func(a model.Agent) string { return a.Name }), nil
				}
				filename := strArg(input, "filename")
				content := strArg(input, "content")
				var id uuid.UUID
				err = d.Pool.QueryRow(ctx,
					`INSERT INTO knowledge_files (agent_id, filename, content, size_bytes) VALUES ($1,$2,$3,$4) RETURNING id`,
					m.Exact.ID, filename, content, len(content),
				).Scan(&id)
				if err != nil {
					return nil, err
				}
				if d.Knowledge != nil {
					_ = d.Knowledge.IngestFile(ctx, ident.OrgID, m.Exact.ID, id, content)
				}
				return &Result{Data: map[string]any{"agent": m.Exact.Name, "filename": filename}}, nil
			},
		))
	}

	if d.KnowledgeSearch != nil && d.Embedder != nil && d.Agents != nil {
		kt := tools.NewKnowledgeTool(d.Embedder, d.KnowledgeSearch)
		reg.Register(newTool(
			"knowledge_search",
			"Search an agent's knowledge base for passages relevant to a query.",
			"config", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{
				"agent": map[string]any{"type": "string"},
				"query": map[string]any{"type": "string"},
			}, "agent", "query"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				agents, err := d.Agents.List(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 100}, repository.AgentListFilter{})
				if err != nil {
					return nil, err
				}
				m := ByName(agents.Data, strArg(input, "agent"), func(a model.Agent) string { return a.Name })
				if !m.Found {
					return clarifyFromMatch("agent", strArg(input, "agent"), "agent", m.Candidates, func(a model.Agent) string { return a.Name }), nil
				}
				out, err := kt.Execute(tools.WithAgent(ctx, m.Exact.ID), map[string]any{"query": strArg(input, "query")})
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"agent": m.Exact.Name, "passages": out}}, nil
			},
		))
	}

	if d.Integrations != nil {
		reg.Register(newTool(
			"integration_link",
			"Link a task to an external tracker (Jira, GitHub) via a named integration.",
			"config", model.PermTasksUpdate, false, false,
			tools.ObjectSchema(map[string]any{
				"integration": map[string]any{"type": "string"},
				"task":        map[string]any{"type": "string"},
				"direction":   map[string]any{"type": "string"},
			}, "integration", "task"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				list, err := d.Integrations.List(ctx, ident.OrgID)
				if err != nil {
					return nil, err
				}
				im := ByName(list, strArg(input, "integration"), func(i model.Integration) string { return i.Name })
				if !im.Found {
					return clarifyFromMatch("integration", strArg(input, "integration"), "integration", im.Candidates, func(i model.Integration) string { return i.Name }), nil
				}
				tasks, err := d.Tasks.ListByOrg(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 100})
				if err != nil {
					return nil, err
				}
				tm := ByName(tasks.Data, strArg(input, "task"), func(t model.Task) string { return t.Title })
				if !tm.Found {
					return clarifyFromMatch("task", strArg(input, "task"), "task", tm.Candidates, func(t model.Task) string { return t.Title }), nil
				}
				dir := strArg(input, "direction")
				if dir == "" {
					dir = "push"
				}
				_, err = d.Integrations.LinkTask(ctx, im.Exact.ID, tm.Exact.ID, dir)
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"integration": im.Exact.Name, "task": tm.Exact.Title}}, nil
			},
		))
	}

	if d.Notifications != nil {
		reg.Register(newTool(
			"notification_configure",
			"Create a Slack or Teams notification target. channel_type is slack or teams.",
			"config", model.PermOrgManage, false, false,
			tools.ObjectSchema(map[string]any{
				"name":         map[string]any{"type": "string"},
				"channel_type": map[string]any{"type": "string", "enum": []any{"slack", "teams"}},
				"webhook_url":  map[string]any{"type": "string"},
				"events":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			}, "name", "channel_type", "webhook_url"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				events, _ := input["events"].([]any)
				ev := make([]string, 0, len(events))
				for _, e := range events {
					ev = append(ev, fmt.Sprint(e))
				}
				if len(ev) == 0 {
					ev = []string{"task.failed"}
				}
				cfg, err := d.Notifications.Create(ctx, ident.OrgID, ident.UserID, model.CreateNotificationConfigRequest{
					Name:        strArg(input, "name"),
					ChannelType: strArg(input, "channel_type"),
					WebhookURL:  strArg(input, "webhook_url"),
					Events:      ev,
				})
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"name": cfg.Name, "channel": cfg.ChannelType}}, nil
			},
		))
		reg.Register(newTool(
			"notification_test",
			"Send a test notification to a named target.",
			"config", model.PermOrgManage, false, false,
			tools.ObjectSchema(map[string]any{
				"name": map[string]any{"type": "string"},
			}, "name"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				list, err := d.Notifications.List(ctx, ident.OrgID)
				if err != nil {
					return nil, err
				}
				m := ByName(list, strArg(input, "name"), func(c model.NotificationConfig) string { return c.Name })
				if !m.Found {
					return clarifyFromMatch("notification", strArg(input, "name"), "name", m.Candidates, func(c model.NotificationConfig) string { return c.Name }), nil
				}
				if err := d.Notifications.TestConfig(ctx, m.Exact.ID); err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"name": m.Exact.Name, "tested": true}}, nil
			},
		))
	}

	if d.Approvals != nil {
		reg.Register(newTool(
			"approval_list",
			"List pending (or all) human approvals.",
			"config", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{
				"status": map[string]any{"type": "string"},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				status := strArg(input, "status")
				if status == "" {
					status = "pending"
				}
				list, err := d.Approvals.List(ctx, ident.OrgID, status)
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"approvals": list, "count": len(list)}}, nil
			},
		))
		reg.Register(newTool(
			"approval_decide",
			"Approve or reject a pending approval. decision is approve or reject.",
			"config", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"approval_id": map[string]any{"type": "string"},
				"decision":    map[string]any{"type": "string", "enum": []any{"approve", "reject"}},
				"reason":      map[string]any{"type": "string"},
			}, "approval_id", "decision"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				id, err := uuid.Parse(strArg(input, "approval_id"))
				if err != nil {
					return &Result{Missing: []string{"approval_id"}, Question: "Which approval should I decide?"}, nil
				}
				a, err := d.Approvals.Decide(ctx, id, ident.UserID, strArg(input, "decision"), strArg(input, "reason"))
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"status": a.Status}}, nil
			},
		))
	}

	if d.Pool != nil && d.Agents != nil {
		reg.Register(newTool(
			"marketplace_search",
			"Search the agent marketplace.",
			"config", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{
				"query": map[string]any{"type": "string"},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				rows, err := d.Pool.Query(ctx, `SELECT id, name, COALESCE(description, ''), category FROM marketplace_agents WHERE is_public = true`)
				if err != nil {
					return nil, err
				}
				defer rows.Close()
				q := strings.ToLower(strArg(input, "query"))
				type row struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Desc string `json:"description"`
				}
				var out []row
				for rows.Next() {
					var id uuid.UUID
					var name, desc, cat string
					if err := rows.Scan(&id, &name, &desc, &cat); err != nil {
						continue
					}
					if q != "" && !strings.Contains(strings.ToLower(name+" "+desc+" "+cat), q) {
						continue
					}
					out = append(out, row{ID: id.String(), Name: name, Desc: desc})
				}
				return &Result{Data: map[string]any{"agents": out}}, nil
			},
		))
		reg.Register(newTool(
			"marketplace_import",
			"Import a marketplace agent into this organisation by name.",
			"config", model.PermAgentsCreate, false, false,
			tools.ObjectSchema(map[string]any{
				"name": map[string]any{"type": "string"},
			}, "name"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				var name, desc, provider, modelName, prompt string
				err := d.Pool.QueryRow(ctx,
					`SELECT name, description, model_provider, model_name, system_prompt FROM marketplace_agents WHERE lower(name) = lower($1) AND is_public = true LIMIT 1`,
					strArg(input, "name"),
				).Scan(&name, &desc, &provider, &modelName, &prompt)
				if err != nil {
					return &Result{Question: "I couldn't find that marketplace agent."}, nil
				}
				req := model.CreateAgentRequest{Name: name, Role: "imported", Description: &desc, ModelProvider: &provider, ModelName: &modelName, SystemPrompt: &prompt}
				a, err := d.Agents.Create(ctx, ident.OrgID, ident.UserID, req)
				if err != nil {
					return nil, err
				}
				ref := agentRef(*a)
				return &Result{Data: map[string]any{"name": a.Name}, Entity: &ref}, nil
			},
		))
	}

	if d.Sessions != nil {
		reg.Register(newTool(
			"session_snapshot",
			"Save a named snapshot of an LLM session, or create a new named session snapshot of the current conversation label.",
			"config", "", false, false,
			tools.ObjectSchema(map[string]any{
				"name":       map[string]any{"type": "string"},
				"session_id": map[string]any{"type": "string"},
			}, "name"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				name := strArg(input, "name")
				if idStr := strArg(input, "session_id"); idStr != "" {
					sid, err := uuid.Parse(idStr)
					if err != nil {
						return &Result{Question: "I couldn't read that session."}, nil
					}
					if err := d.Sessions.CreateSnapshot(ctx, &model.SessionSnapshot{SessionID: sid, Name: name}); err != nil {
						return nil, err
					}
					return &Result{Data: map[string]any{"name": name, "saved": true}}, nil
				}
				uid := ident.UserID
				s := &model.Session{ID: uuid.New(), OrgID: ident.OrgID, Name: name, Status: "active", CreatedBy: &uid}
				if err := d.Sessions.Create(ctx, s); err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"name": s.Name, "saved": true}}, nil
			},
		))
	}
}

// scheduleCronParser accepts the same cron flavour the scheduler runner uses.
var scheduleCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

func listAllAgents(ctx context.Context, d Deps, orgID uuid.UUID) ([]model.Agent, error) {
	res, err := d.Agents.List(ctx, orgID, model.PaginationParams{Page: 1, PerPage: 100}, repository.AgentListFilter{})
	if err != nil {
		return nil, err
	}
	return res.Data, nil
}

func mapPreset(p string) string {
	switch p {
	case "weekdays_9am":
		return "0 9 * * 1-5"
	case "every_morning_9am":
		return "0 9 * * *"
	case "every_midnight":
		return "0 0 * * *"
	case "hourly":
		return "0 * * * *"
	default:
		return "0 9 * * 1-5"
	}
}
