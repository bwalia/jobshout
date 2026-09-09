package tasklaunch

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// ProjectDecision is the result of applying the chat project rule.
type ProjectDecision struct {
	ProjectID uuid.UUID
	// Missing is "project" when the org has two or more projects and the
	// caller did not name one (and last_project is not a clear reference).
	Missing  string
	Question string
	Options  []model.ClarifyOption
}

// ResolveProject picks a project for a chat-triggered launch.
//
// One project → use it. More than one → interview unless hint matches a name
// or lastProjectID is a valid org project and the hint is a "that project"
// reference (empty last_project is not enough).
func (s *Service) ResolveProject(ctx context.Context, orgID, userID uuid.UUID, hint, lastProjectID string) (ProjectDecision, error) {
	if s.Projects == nil {
		return ProjectDecision{}, nil
	}
	page, err := s.Projects.List(ctx, orgID, model.PaginationParams{Page: 1, PerPage: 100})
	if err != nil {
		return ProjectDecision{}, err
	}
	projects := page.Data
	if len(projects) == 0 {
		// With no projects, a hint cannot name an existing one — the only
		// thing it can mean is what to call the first. Create it and carry on
		// rather than sending the user to Task Manager and losing the task
		// they were in the middle of.
		if name := newProjectName(hint); name != "" {
			created, err := s.Projects.Create(ctx, orgID, userID, model.CreateProjectRequest{
				Name:     name,
				Priority: "medium",
			})
			if err != nil {
				return ProjectDecision{}, err
			}
			if created != nil {
				return ProjectDecision{ProjectID: created.ID}, nil
			}
		}
		return ProjectDecision{
			Missing:  "project",
			Question: "There is no project yet. I can create one and carry straight on — take the default name, or tell me what to call it.",
			Options: []model.ClarifyOption{
				{Label: `Create "General" and continue`, Value: NewProjectPrefix + "General"},
			},
		}, nil
	}
	if len(projects) == 1 {
		return ProjectDecision{ProjectID: projects[0].ID}, nil
	}

	hint = strings.TrimSpace(hint)
	if hint != "" {
		if id, err := uuid.Parse(hint); err == nil {
			for _, p := range projects {
				if p.ID == id {
					return ProjectDecision{ProjectID: p.ID}, nil
				}
			}
		}
		lower := strings.ToLower(hint)
		var matches []model.Project
		for _, p := range projects {
			if strings.EqualFold(p.Name, hint) || strings.Contains(strings.ToLower(p.Name), lower) {
				matches = append(matches, p)
			}
		}
		if len(matches) == 1 {
			return ProjectDecision{ProjectID: matches[0].ID}, nil
		}
	}

	if lastProjectID != "" && isThatProjectHint(hint) {
		if id, err := uuid.Parse(lastProjectID); err == nil {
			for _, p := range projects {
				if p.ID == id {
					return ProjectDecision{ProjectID: p.ID}, nil
				}
			}
		}
	}

	opts := make([]model.ClarifyOption, 0, len(projects))
	for _, p := range projects {
		opts = append(opts, model.ClarifyOption{Label: p.Name, Value: p.ID.String()})
	}
	return ProjectDecision{
		Missing:  "project",
		Question: "Which project should this task go on?",
		Options:  opts,
	}, nil
}

func isThatProjectHint(hint string) bool {
	h := strings.ToLower(strings.TrimSpace(hint))
	switch h {
	case "", "that", "that project", "the same", "the same project", "same", "it":
		return true
	}
	return false
}

// NewProjectPrefix marks a clarify answer that means "create a project with
// this name". The prefix keeps creation explicit: a bare hint that happens to
// look like a name can still be treated as one, but the option we offer always
// round-trips unambiguously.
const NewProjectPrefix = "new:"

// newProjectName is the project name an answer asks us to create, or "" when
// the answer is not a name at all (a "that project" style reference).
func newProjectName(hint string) string {
	hint = strings.TrimSpace(hint)
	if strings.HasPrefix(strings.ToLower(hint), NewProjectPrefix) {
		if name := strings.TrimSpace(hint[len(NewProjectPrefix):]); name != "" {
			return name
		}
		return "General"
	}
	if hint == "" || isThatProjectHint(hint) {
		return ""
	}
	if _, err := uuid.Parse(hint); err == nil {
		// A stale id from a previous org/session is not a name.
		return ""
	}
	if len([]rune(hint)) < 2 {
		return "" // Create validates min=2; re-ask instead of erroring.
	}
	return hint
}
