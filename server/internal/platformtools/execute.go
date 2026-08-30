package platformtools

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
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
		}
	}

	// Every builtin now walks the same interview. The Mail Agent used to be
	// special-cased here on whether the prompt contained the word "sync",
	// because agentschema had no Mail fields to work from; it has them now.
	// Reading drafts is a query rather than an execution, and the model reaches
	// mail_list_drafts directly for that.
	if slot, question, opts := schema.NextMissing(vals); slot != "" {
		if slot == "prompt" {
			question = fmt.Sprintf("What should %s do?", agent.Name)
		}
		return &Result{Missing: []string{slot}, Question: question, Options: opts}, nil
	}
	vals = schema.ApplyDefaults(vals)

	if agentschema.IsThinPrompt(prompt, agent.Name) && builtin == "" {
		return &Result{
			Missing:  []string{"prompt"},
			Question: fmt.Sprintf("What should %s do?", agent.Name),
		}, nil
	}
	if builtin == "" && vals["prompt"] == "" {
		vals["prompt"] = prompt
	}

	// Every execution goes through the Task Manager's front door, including
	// this one. Calling the specialist directly — which is what this used to do
	// — left no run row and no board entry, so work started from chat was
	// invisible everywhere else. The run id comes back immediately; the
	// specialist's own row carries progress from there.
	if d.AgentRuns == nil {
		return nil, fmt.Errorf("agent execution is not configured")
	}
	run, _, err := d.AgentRuns.Start(ctx, ident.OrgID, model.CreateAgentRunRequest{
		AgentID: agent.ID,
		// Narrowed to the schema's own slots: the tool call also carries the
		// agent's name and whatever reason the model volunteered, and recording
		// those as inputs would make the same request look different depending
		// on which surface sent it.
		Inputs: schema.Pick(vals),
	}, &ident.UserID, model.AgentRunSourceChat)
	if err != nil {
		// A missing slot is a question, not a failure: the same validation the
		// Task Manager renders against a form field is asked here in words.
		if miss, ok := service.AsMissingInput(err); ok {
			return &Result{Missing: miss.Missing, Question: miss.Question, Options: miss.Options}, nil
		}
		return nil, err
	}

	res := runResult(run, agent)
	if reason := strArg(input, "reason"); reason != "" {
		res.Data.(map[string]any)["reason"] = reason
	}
	return res, nil
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
