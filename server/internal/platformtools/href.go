package platformtools

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func agentHref(id uuid.UUID) string    { return "/agents/" + id.String() }
func taskHref(id uuid.UUID) string     { return "/task-manager" }
func projectHref(id uuid.UUID) string {
	return "/panel/projects?project=" + id.String()
}
func workflowHref(id uuid.UUID) string { return "/workflows/" + id.String() }
func executionHref(agentID uuid.UUID) string {
	return "/agents/" + agentID.String()
}
func articleHref(id uuid.UUID) string { return "/articles/" + id.String() }
func pentestHref() string             { return "/agents/pentest" }
func reviewHref() string              { return "/agents/review" }
func mailHref() string                { return "/panel/task-manager?agent=mail" }
func careerHref() string              { return "/panel/task-manager?agent=career" }
func sprintHref() string              { return "/sprints" }

func agentRef(a model.Agent) model.EntityRef {
	return model.EntityRef{Kind: model.EntityAgent, ID: a.ID.String(), Label: a.Name, Href: agentHref(a.ID)}
}

func taskRef(t model.Task) model.EntityRef {
	return model.EntityRef{Kind: model.EntityTask, ID: t.ID.String(), Label: t.Title, Href: taskHref(t.ID)}
}

func projectRef(p model.Project) model.EntityRef {
	return model.EntityRef{Kind: model.EntityProject, ID: p.ID.String(), Label: p.Name, Href: projectHref(p.ID)}
}

func workflowRef(w model.Workflow) model.EntityRef {
	return model.EntityRef{Kind: model.EntityWorkflow, ID: w.ID.String(), Label: w.Name, Href: workflowHref(w.ID)}
}

func executionRef(e model.AgentExecution, agentName string) model.EntityRef {
	label := agentName
	if label == "" {
		label = "execution"
	}
	return model.EntityRef{Kind: model.EntityExecution, ID: e.ID.String(), Label: label, Href: executionHref(e.AgentID)}
}

func reviewRef(run model.ReviewRun) model.EntityRef {
	label := fmt.Sprintf("%s#%d", run.Repo, run.PRNumber)
	return model.EntityRef{Kind: model.EntityReviewRun, ID: run.ID.String(), Label: label, Href: reviewHref()}
}

func imageRef(id, url string) model.EntityRef {
	return model.EntityRef{
		Kind:  model.EntityImage,
		ID:    id,
		Label: "generated image",
		Href:  "/images",
		URL:   url,
	}
}

// clarifyFromMatch asks the user to pick among candidates. kind is the English
// noun in the question ("agent"); slot is the JSON property the tool reads
// ("name"). Missing must be the schema field so chip answers merge correctly.
func clarifyFromMatch[T any](kind, query, slot string, candidates []T, nameOf func(T) string) *Result {
	opts := make([]model.ClarifyOption, 0, len(candidates))
	for _, c := range candidates {
		n := nameOf(c)
		opts = append(opts, model.ClarifyOption{Label: n, Value: n})
	}
	q := query
	if q == "" {
		q = kind
	}
	question := fmt.Sprintf("I found more than one %s matching %q. Which did you mean?", kind, q)
	if len(candidates) == 0 {
		question = fmt.Sprintf("I couldn't find a %s named %q. Pick one of these, or tell me another name.", kind, q)
	}
	if slot == "" {
		slot = kind
	}
	return &Result{
		Missing:  []string{slot},
		Options:  opts,
		Question: question,
		Data:     map[string]any{"candidates": labelsOf(candidates, nameOf)},
	}
}

func notFoundClarify(kind, query, slot string, options []model.ClarifyOption) *Result {
	q := strings.TrimSpace(query)
	question := fmt.Sprintf("I couldn't find a %s named %q.", kind, q)
	if q == "" {
		question = fmt.Sprintf("Which %s should I use?", kind)
	}
	if len(options) > 0 {
		question += " Here are the ones I can see."
	}
	if slot == "" {
		slot = kind
	}
	return &Result{Missing: []string{slot}, Options: options, Question: question}
}
