package platformtools

import (
	"context"
	"sort"
	"strings"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/tools"
)

// AlwaysLoad is the tier-1 tool set sent on every turn. catalog_search
// discloses the rest. Keep this list small: schemas cost tokens and degrade
// selection accuracy.
var AlwaysLoad = []string{
	"catalog_search",
	"help",
	"remember",
	"agent_list",
	"agent_execute",
	"task_list",
	"task_create",
	"task_get",
	"workflow_list",
	"workflow_run",
	"schedule_create",
	"schedule_list",
	"execution_get",
	"usage_summary",
	"my_permissions",
	"agent_board",
	"image_generate",
	"review_pull_request",
	"review_run_get",
}

// HumanLabel is the progress chip text shown while a tool runs.
func HumanLabel(name string) string {
	if s, ok := humanLabels[name]; ok {
		return s
	}
	return "Working…"
}

var humanLabels = map[string]string{
	"catalog_search":       "Looking up what I can do…",
	"help":                 "Checking what I can help with…",
	"remember":             "Saving that…",
	"agent_list":           "Looking up your agents…",
	"agent_get":            "Opening the agent…",
	"agent_create":         "Creating the agent…",
	"agent_update":         "Updating the agent…",
	"agent_set_status":     "Changing agent status…",
	"agent_delete":         "Deleting the agent…",
	"agent_execute":        "Running the agent…",
	"execution_get":        "Checking that run…",
	"execution_cancel":     "Stopping the run…",
	"execution_list":       "Listing past runs…",
	"goal_create":          "Setting a goal…",
	"goal_get":             "Checking the goal…",
	"agent_set_manager":    "Updating the org chart…",
	"task_list":            "Looking up tasks…",
	"task_create":          "Creating the task…",
	"task_get":             "Checking the task…",
	"task_update":          "Updating the task…",
	"task_transition":      "Moving the task…",
	"task_comment":         "Adding a comment…",
	"task_delete":          "Deleting the task…",
	"project_list":         "Looking up projects…",
	"project_create":       "Creating the project…",
	"sprint_list":          "Looking up sprints…",
	"sprint_create":        "Creating the sprint…",
	"sprint_add_job":       "Adding work to the sprint…",
	"workflow_list":        "Looking up workflows…",
	"workflow_get":         "Opening the workflow…",
	"workflow_run":         "Starting the workflow…",
	"workflow_run_get":     "Checking the workflow run…",
	"workflow_create":      "Creating the workflow…",
	"multi_agent_run":      "Starting the collaboration…",
	"agent_board":          "Checking who is working on what…",
	"research_run":         "Researching…",
	"trending_topics":      "Checking what's trending…",
	"article_generate":     "Starting the article…",
	"article_run_get":      "Checking the article run…",
	"article_publish":      "Publishing the article…",
	"article_run_cancel":   "Cancelling the article…",
	"pentest_start":        "Starting the security test…",
	"pentest_findings":     "Reading the findings…",
	"pentest_cancel":       "Cancelling the pentest…",
	"image_generate":       "Generating the image…",
	"review_pull_request":  "Queueing the PR review…",
	"review_run_get":       "Checking the PR review…",
	"review_run_list":      "Listing PR reviews…",
	"review_allowed_repos": "Checking which repos I can review…",
	"usage_summary":        "Checking usage…",
	"agent_analytics":      "Checking agent analytics…",
	"leaderboard":          "Checking the leaderboard…",
	"anomalies":            "Looking for anomalies…",
	"budget_status":        "Checking budgets…",
	"policy_list":          "Listing policies…",
	"audit_search":         "Searching the audit log…",
	"task_metrics":         "Checking delivery metrics…",
	"my_permissions":       "Checking your permissions…",
	"role_list":            "Listing roles…",
	"role_assign":          "Updating roles…",
}

type catalogEntry struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Domain      string         `json:"domain"`
	Schema      map[string]any `json:"schema"`
}

func registerCatalog(reg *Registry) {
	reg.Register(newTool(
		"catalog_search",
		"Find additional tools by capability. Call this when the user's request is not covered by the tools you already have. Returns matching tool names, descriptions and JSON schemas; those tools become available on the next turn.",
		"config",
		"",
		false, true,
		tools.ObjectSchema(map[string]any{
			"query": map[string]any{"type": "string", "description": "What you need to do, in plain language"},
		}, "query"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			query := strings.ToLower(strArg(input, "query"))
			ident := MustIdentity(ctx)
			perms, _ := ctxPerms(ctx)
			_ = ident
			type scored struct {
				entry catalogEntry
				score int
			}
			var ranked []scored
			for _, t := range reg.FilterByPermissions(perms) {
				if inAlwaysLoad(t.Name()) {
					continue
				}
				blob := strings.ToLower(t.Name() + " " + t.Description() + " " + t.Domain())
				score := catalogScore(query, blob)
				if score <= 0 {
					continue
				}
				ranked = append(ranked, scored{
					entry: catalogEntry{
						Name: t.Name(), Description: t.Description(), Domain: t.Domain(), Schema: t.Schema(),
					},
					score: score,
				})
			}
			sort.Slice(ranked, func(i, j int) bool {
				if ranked[i].score != ranked[j].score {
					return ranked[i].score > ranked[j].score
				}
				return ranked[i].entry.Name < ranked[j].entry.Name
			})
			if len(ranked) > 12 {
				ranked = ranked[:12]
			}
			matches := make([]catalogEntry, 0, len(ranked))
			names := make([]string, 0, len(ranked))
			for _, s := range ranked {
				matches = append(matches, s.entry)
				names = append(names, s.entry.Name)
			}
			AddDisclosedTools(ctx, names) // value not persisted; loop reads from returned names
			return &Result{Data: map[string]any{"tools": matches, "names": names}}, nil
		},
	))
}

func inAlwaysLoad(name string) bool {
	for _, n := range AlwaysLoad {
		if n == name {
			return true
		}
	}
	return false
}

var catalogStop = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "to": true, "for": true,
	"and": true, "or": true, "in": true, "on": true, "my": true, "me": true,
	"i": true, "you": true, "we": true, "from": true, "with": true, "that": true,
	"this": true, "it": true, "is": true, "be": true, "do": true, "can": true,
	"please": true, "want": true, "need": true,
}

// catalogScore ranks how well a tool blob matches a natural-language query.
// The old substring check required the entire query to appear in the description,
// so "generate an image of a tiger" missed image_generate.
func catalogScore(query, blob string) int {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return 1
	}
	if strings.Contains(blob, query) {
		return 100
	}
	score := 0
	for _, tok := range strings.Fields(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return ' '
	}, query)) {
		if len(tok) < 3 || catalogStop[tok] {
			continue
		}
		if strings.Contains(blob, tok) {
			score++
		}
	}
	return score
}

func ctxPerms(ctx context.Context) (map[string]bool, bool) {
	v := ctx.Value(permsKey{})
	m, ok := v.(map[string]bool)
	return m, ok
}

type permsKey struct{}

func WithPermissions(ctx context.Context, perms map[string]bool) context.Context {
	return context.WithValue(ctx, permsKey{}, perms)
}

// ToolDefs converts platform tools to llm.ToolDef for the native loop.
func ToolDefs(ts []PlatformTool) []llm.ToolDef {
	out := make([]llm.ToolDef, 0, len(ts))
	for _, t := range ts {
		out = append(out, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		})
	}
	return out
}

// SelectForTurn returns always-load tools plus any extra names, filtered by RBAC.
func (r *Registry) SelectForTurn(allowed map[string]bool, extra []string) []PlatformTool {
	want := map[string]bool{}
	for _, n := range AlwaysLoad {
		want[n] = true
	}
	for _, n := range extra {
		want[n] = true
	}
	filtered := r.FilterByPermissions(allowed)
	out := make([]PlatformTool, 0, len(want))
	for _, t := range filtered {
		if want[t.Name()] {
			out = append(out, t)
		}
	}
	return out
}
