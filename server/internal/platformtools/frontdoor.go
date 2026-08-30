package platformtools

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
)

// startAgentRun sends a specialist's work through the same door the Task
// Manager uses.
//
// The specialist tools keep their own argument gathering — a chat turn asks in
// words where a form would show a field, and each tool knows the phrasing that
// suits it — but the execution itself is a hand-off to AgentRunService. That is
// what makes a run started from chat leave the same row, and appear on the same
// board, as one started from the Task Manager. Calling the specialist service
// directly, which is what these tools used to do, left nothing behind.
func startAgentRun(ctx context.Context, d Deps, builtin string, vals map[string]string) (*Result, error) {
	ident := MustIdentity(ctx)
	if d.AgentRuns == nil || d.Agents == nil {
		return &Result{Data: map[string]any{"message": "Running agents is not configured on this server."}}, nil
	}

	agent, err := findBuiltinAgent(ctx, d, builtin)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return &Result{Data: map[string]any{
			"message": fmt.Sprintf("No %s agent is configured for this organisation.", builtinLabel(builtin)),
		}}, nil
	}

	run, _, err := d.AgentRuns.Start(ctx, ident.OrgID, model.CreateAgentRunRequest{
		AgentID: agent.ID,
		Inputs:  vals,
	}, &ident.UserID, model.AgentRunSourceChat)
	if err != nil {
		// A missing slot is a question, not a failure: the same validation the
		// Task Manager renders against a form field is asked here in words.
		if miss, ok := service.AsMissingInput(err); ok {
			return &Result{Missing: miss.Missing, Question: miss.Question, Options: miss.Options}, nil
		}
		return nil, err
	}
	return runResult(run, agent), nil
}

// runResult describes a hand-off in the terms the model should reason about:
// the work has started and there is nothing left for this turn to do. It
// deliberately does not report the envelope's own "completed" status, which
// means "handed off" and reads, to a model, as "already finished" — an
// invitation to go and fetch a result that does not exist yet.
func runResult(run *model.AgentRun, agent *model.Agent) *Result {
	data := map[string]any{
		"started": true,
		"agent":   agent.Name,
		// The model is deliberately not handed the run id or an external handle.
		// The run has only just been queued, so any lookup this turn comes back
		// empty — and a model given an id will use it: it fetches the not-yet
		// result, gets nothing, and reports the successful start as a failure
		// ("I couldn't find that"). Observed live for research_run, which unlike
		// article_generate has no artifact page, so the model reached for
		// execution_get with this very id. The id still travels on the entity ref
		// below for the UI card and for later "publish it" resolution; the model
		// does not need it to tell the user the work has started.
		"message": fmt.Sprintf(
			"%s has started. Tell the user it is underway and where to watch it, "+
				"then stop: there is no result to fetch this turn, so do not call a "+
				"status, get, board, or execution tool for it now.", agent.Name),
	}
	if run.Status == model.AgentRunFailed && run.ErrorMessage != nil {
		data["started"] = false
		data["message"] = *run.ErrorMessage
	}
	ref := specialistRef(run, agent)
	return &Result{Data: data, Entity: ref, Entities: []model.EntityRef{*ref}}
}

// specialistRef links to the row that carries the work, not the envelope, so
// the chat card opens the article, scan or review the user just asked for.
func specialistRef(run *model.AgentRun, agent *model.Agent) *model.EntityRef {
	id := run.ID.String()
	if run.ExternalRunID != nil && *run.ExternalRunID != "" {
		id = *run.ExternalRunID
	}
	switch run.ExternalKind {
	case "blog_run":
		if u, err := uuid.Parse(id); err == nil {
			return &model.EntityRef{Kind: model.EntityArticle, ID: id, Label: agent.Name, Href: articleHref(u)}
		}
	case "pentest_run":
		return &model.EntityRef{Kind: model.EntityPentest, ID: id, Label: agent.Name, Href: pentestHref()}
	case "review_run":
		return &model.EntityRef{Kind: model.EntityReviewRun, ID: id, Label: agent.Name, Href: reviewHref()}
	case "mail_sync":
		return &model.EntityRef{Kind: model.EntityMailThread, ID: "", Label: "Mail inbox", Href: mailHref()}
	}
	// research_run and task_run have no page of their own yet; the agent's own
	// page is where their progress shows.
	ref := agentRef(*agent)
	return &ref
}

func findBuiltinAgent(ctx context.Context, d Deps, builtin string) (*model.Agent, error) {
	ident := MustIdentity(ctx)
	page, err := d.Agents.List(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 100}, repository.AgentListFilter{})
	if err != nil {
		return nil, err
	}
	for i := range page.Data {
		if page.Data[i].IsBuiltin(builtin) {
			return &page.Data[i], nil
		}
	}
	return nil, nil
}

func builtinLabel(builtin string) string {
	switch builtin {
	case model.BuiltinArticleWriter:
		return "article writing"
	case model.BuiltinResearcher:
		return "research"
	case model.BuiltinPentester:
		return "penetration testing"
	case model.BuiltinPRReviewer:
		return "pull request review"
	case model.BuiltinMail:
		return "mail"
	}
	return builtin
}
