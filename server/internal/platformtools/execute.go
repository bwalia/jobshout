package platformtools

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

func runAgentExecute(ctx context.Context, d Deps, reg *Registry, input map[string]any) (*Result, error) {
	ident := MustIdentity(ctx)
	page := model.PaginationParams{Page: 1, PerPage: 100}
	agentsPage, err := d.Agents.List(ctx, ident.OrgID, page, repository.AgentListFilter{})
	if err != nil {
		return nil, err
	}
	agents := agentsPage.Data

	agent, clar := pickExecuteAgent(agents, strArg(input, "name"))
	if clar != nil {
		return clar, nil
	}

	builtin := agentschema.BuiltinOf(agent)
	schema := agentschema.ForBuiltin(builtin)
	vals := agentschema.ValuesFromArgs(input)
	prompt := strArg(input, "prompt")

	if !agentschema.IsThinPrompt(prompt, agent.Name) {
		switch builtin {
		case model.BuiltinResearcher, model.BuiltinArticleWriter:
			if vals["topic"] == "" {
				vals["topic"] = prompt
			}
		case model.BuiltinPentester:
			if vals["target"] == "" {
				vals["target"] = prompt
			}
		case model.BuiltinCareerOps:
			if vals["job_url"] == "" && vals["jd_text"] == "" {
				if looksLikeURL(prompt) {
					vals["job_url"] = prompt
				} else {
					vals["jd_text"] = prompt
				}
			}
		}
	}

	if builtin == model.BuiltinMail && strings.Contains(strings.ToLower(prompt), "draft") &&
		!strings.Contains(strings.ToLower(prompt), "sync") && d.Launch == nil {
		return dispatchTool(ctx, reg, "mail_list_drafts", input)
	}

	if slot, question, opts := schema.NextMissing(vals); slot != "" {
		if slot == "prompt" {
			question = fmt.Sprintf("What should %s do?", agent.Name)
		}
		return &Result{Missing: []string{slot}, Question: question, Options: opts}, nil
	}
	vals = schema.ApplyDefaults(vals)

	if builtin == model.BuiltinCareerOps && strings.TrimSpace(vals["job_url"]) == "" && strings.TrimSpace(vals["jd_text"]) == "" {
		return &Result{Missing: []string{"job_url"}, Question: "Paste a job URL, or the job description text."}, nil
	}

	if d.Launch != nil {
		return launchAgent(ctx, d, agent, vals)
	}

	if schema.SpecialistTool != "" {
		args := map[string]any{}
		for k, v := range vals {
			if v != "" {
				args[k] = v
			}
		}
		return dispatchTool(ctx, reg, schema.SpecialistTool, args)
	}

	if agentschema.IsThinPrompt(prompt, agent.Name) {
		return &Result{
			Missing:  []string{"prompt"},
			Question: fmt.Sprintf("What should %s do?", agent.Name),
		}, nil
	}
	if d.Exec == nil {
		return nil, fmt.Errorf("execution is not configured")
	}
	exec, err := d.Exec.Start(ctx, ident.OrgID, agent.ID, model.ExecuteAgentRequest{Prompt: prompt})
	if err != nil {
		return nil, err
	}
	aref := agentRef(*agent)
	eref := executionRef(*exec, agent.Name)
	status := exec.Status
	if status == "" {
		status = model.ExecutionStatusRunning
	}
	return &Result{
		Data: map[string]any{
			"agent":  agent.Name,
			"status": status,
			"reason": strArg(input, "reason"),
		},
		Entity:   &eref,
		Entities: []model.EntityRef{aref, eref},
	}, nil
}

func pickExecuteAgent(agents []model.Agent, name string) (*model.Agent, *Result) {
	opts := make([]model.ClarifyOption, 0, len(agents))
	for _, a := range agents {
		opts = append(opts, model.ClarifyOption{Label: a.Name + " — " + a.Role, Value: a.Name})
	}
	if name != "" {
		m := ByName(agents, name, func(a model.Agent) string { return a.Name })
		if m.Found {
			a := m.Exact
			return &a, nil
		}
		if len(m.Candidates) > 0 {
			return nil, clarifyFromMatch("agent", name, "name", m.Candidates, func(a model.Agent) string { return a.Name })
		}
		return nil, notFoundClarify("agent", name, "name", opts)
	}
	if len(agents) == 0 {
		return nil, &Result{
			Missing:  []string{"name"},
			Question: "There are no agents in this organisation yet. Create one first?",
		}
	}
	if len(agents) == 1 {
		return &agents[0], nil
	}
	return nil, &Result{
		Missing:  []string{"name"},
		Options:  opts,
		Question: "Which agent should handle that?",
	}
}

func dispatchTool(ctx context.Context, reg *Registry, name string, input map[string]any) (*Result, error) {
	if reg == nil {
		return &Result{Data: map[string]any{"message": "That specialist isn't available on this server."}}, nil
	}
	t, ok := reg.Get(name)
	if !ok {
		return &Result{Data: map[string]any{"message": "That specialist isn't available on this server."}}, nil
	}
	return t.Run(ctx, input)
}

func lastEntityID(ctx context.Context, kind string) string {
	ents := SessionEntitiesFrom(ctx)
	if ents == nil {
		return ""
	}
	e, ok := ents["last_"+kind]
	if !ok {
		return ""
	}
	return strings.TrimSpace(e.ID)
}

func looksLikeURL(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
