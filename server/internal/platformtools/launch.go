package platformtools

import (
	"context"
	"strings"

	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/tasklaunch"
)

type memoriesKey struct{}

// WithMemories attaches recalled user memories so launches can fold them into context.
func WithMemories(ctx context.Context, memories []string) context.Context {
	if len(memories) == 0 {
		return ctx
	}
	return context.WithValue(ctx, memoriesKey{}, memories)
}

func memoriesFrom(ctx context.Context) []string {
	v, _ := ctx.Value(memoriesKey{}).([]string)
	return v
}

func injectMemoryContext(ctx context.Context, vals map[string]string) {
	if vals == nil {
		return
	}
	hits := memoriesFrom(ctx)
	if len(hits) == 0 {
		return
	}
	extra := strings.Join(hits, "\n")
	if strings.TrimSpace(vals["context"]) == "" {
		vals["context"] = extra
		return
	}
	vals["context"] = strings.TrimSpace(vals["context"]) + "\n\nRemembered:\n" + extra
}

func lastProjectID(ctx context.Context) string {
	return lastEntityID(ctx, model.EntityProject)
}

func findBuiltinAgent(ctx context.Context, d Deps, builtin string) (*model.Agent, *Result) {
	if d.Agents == nil {
		return nil, &Result{Data: map[string]any{"message": "Agents are not configured."}}
	}
	ident := MustIdentity(ctx)
	page, err := d.Agents.List(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 100}, repository.AgentListFilter{})
	if err != nil {
		return nil, &Result{Data: map[string]any{"message": "Could not list agents."}}
	}
	for i := range page.Data {
		if agentschema.BuiltinOf(&page.Data[i]) == builtin {
			a := page.Data[i]
			return &a, nil
		}
	}
	return nil, &Result{Data: map[string]any{"message": "That specialist is not in this organisation."}}
}

func launchSpecialist(ctx context.Context, d Deps, builtin string, vals map[string]string) (*Result, error) {
	if d.Launch == nil {
		return nil, nil
	}
	agent, miss := findBuiltinAgent(ctx, d, builtin)
	if miss != nil {
		return miss, nil
	}
	return launchAgent(ctx, d, agent, vals)
}

func launchAgent(ctx context.Context, d Deps, agent *model.Agent, vals map[string]string) (*Result, error) {
	if d.Launch == nil {
		return nil, nil
	}
	if vals == nil {
		vals = map[string]string{}
	}
	injectMemoryContext(ctx, vals)
	ident := MustIdentity(ctx)
	res, dec, err := d.Launch.LaunchFromChat(ctx, ident.OrgID, ident.UserID, agent, vals, lastProjectID(ctx))
	if err != nil {
		return nil, err
	}
	if dec != nil && dec.Missing != "" {
		return &Result{Missing: []string{dec.Missing}, Question: dec.Question, Options: dec.Options}, nil
	}
	out := launchResultToTool(res)
	if out != nil && agent != nil {
		aref := model.EntityRef{
			Kind:  model.EntityAgent,
			ID:    agent.ID.String(),
			Label: agent.Name,
			Href:  "/panel/task-manager?agent=" + agent.ID.String(),
		}
		out.Entities = append(out.Entities, aref)
	}
	return out, nil
}

func launchResultToTool(res *tasklaunch.Result) *Result {
	if res == nil {
		return &Result{Data: map[string]any{"message": "Nothing ran."}}
	}
	data := map[string]any{"kind": res.Kind, "status": "started"}
	if res.Message != "" {
		data["message"] = res.Message
	}
	if res.SyncQueued {
		data["sync_queued"] = true
		data["status"] = "queued"
	}
	if res.Brief != nil {
		data["summary"] = res.Brief.Summary
		data["findings"] = res.Brief.Findings
		data["status"] = "done"
	}
	if res.ImageURL != "" {
		data["url"] = res.ImageURL
	}
	if res.RunID != nil {
		data["run_id"] = res.RunID.String()
	}
	if res.EvaluationID != nil {
		data["evaluation_id"] = res.EvaluationID.String()
	}
	var ents []model.EntityRef
	if res.Task != nil {
		href := "/panel/task-manager?project=" + res.Task.ProjectID.String() + "&task=" + res.Task.ID.String()
		if res.RunID != nil {
			href += "&run=" + res.RunID.String()
		}
		tref := model.EntityRef{Kind: model.EntityTask, ID: res.Task.ID.String(), Label: res.Task.Title, Href: href}
		pref := model.EntityRef{Kind: model.EntityProject, ID: res.Task.ProjectID.String(), Label: "project", Href: "/panel/projects?project=" + res.Task.ProjectID.String()}
		ents = append(ents, tref, pref)
		data["task"] = res.Task.Title
		return &Result{Data: data, Entity: &tref, Entities: ents}
	}
	return &Result{Data: data, Entities: ents}
}
