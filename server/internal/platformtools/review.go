package platformtools

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
	"github.com/jobshout/server/internal/tools"
)

func registerReview(reg *Registry, d Deps) {
	if d.Reviews == nil {
		return
	}

	reg.Register(newTool(
		"review_pull_request",
		"Queue an AI review of a GitHub pull request via review-bot. Omit unknown fields; the tool will ask. Do not invent a repo or PR number. Pass repo as owner/name (or a github.com URL) and pr_number. Defaults to dry_run=true so findings are not posted. Poll with review_run_get.",
		"security", model.PermAgentsExecute, false, false,
		tools.ObjectSchema(map[string]any{
			"repo":      map[string]any{"type": "string", "description": "owner/name or a GitHub pull request URL. Omit if unknown."},
			"pr_number": map[string]any{"type": "integer", "description": "Pull request number. Omit if unknown."},
			"dry_run":   map[string]any{"type": "boolean", "description": "If true (default), do not post comments on GitHub"},
			"force":     map[string]any{"type": "boolean", "description": "Re-review even if this commit was already started"},
		}),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			if !d.Reviews.Enabled() {
				return &Result{Data: map[string]any{
					"available": false,
					"message":   "PR review is not enabled on this server.",
				}}, nil
			}
			repo, pr := parseRepoAndPR(strArg(input, "repo"), intArg(input, "pr_number", 0))
			if repo == "" {
				opts := repoOptions(d.Reviews.AllowedRepos())
				return &Result{
					Missing:  []string{"repo"},
					Options:  opts,
					Question: "Which GitHub repo should I review? Use owner/name.",
				}, nil
			}
			if pr < 1 {
				return &Result{Missing: []string{"pr_number"}, Question: "Which pull request number?"}, nil
			}
			dry := true
			if _, ok := input["dry_run"]; ok {
				dry = boolArg(input, "dry_run", true)
			}
			uid := ident.UserID
			run, err := d.Reviews.CreateRun(ctx, model.CreateReviewRunRequest{
				Repo:     repo,
				PRNumber: pr,
				DryRun:   &dry,
				Force:    boolArg(input, "force", false),
			}, ident.OrgID, &uid)
			if err != nil {
				return reviewErr(d, err)
			}
			ref := reviewRef(*run)
			return &Result{
				Data:   reviewRunData(run),
				Entity: &ref,
			}, nil
		},
	))

	reg.Register(newTool(
		"review_run_get",
		"Report a PR review run: status, verdict, summary and blocking findings. Poll until status is completed or failed.",
		"security", model.PermAgentsRead, false, true,
		tools.ObjectSchema(map[string]any{
			"run_id": map[string]any{"type": "string"},
		}, "run_id"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			id, err := uuid.Parse(strArg(input, "run_id"))
			if err != nil {
				return &Result{Missing: []string{"run_id"}, Question: "Which PR review should I check?"}, nil
			}
			run, err := d.Reviews.GetRun(ctx, id, ident.OrgID)
			if err != nil {
				return reviewErr(d, err)
			}
			ref := reviewRef(*run)
			return &Result{Data: reviewRunData(run), Entity: &ref}, nil
		},
	))

	reg.Register(newTool(
		"review_run_list",
		"List recent PR review runs in this organisation.",
		"security", model.PermAgentsRead, false, true,
		tools.ObjectSchema(map[string]any{
			"limit": map[string]any{"type": "integer"},
		}),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			limit := intArg(input, "limit", 10)
			if limit < 1 {
				limit = 10
			}
			page, err := d.Reviews.ListRuns(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: limit})
			if err != nil {
				return reviewErr(d, err)
			}
			rows := make([]map[string]any, 0, len(page.Data))
			ents := make([]model.EntityRef, 0, len(page.Data))
			for i := range page.Data {
				run := page.Data[i]
				rows = append(rows, reviewRunData(&run))
				ents = append(ents, reviewRef(run))
			}
			return &Result{Data: map[string]any{"runs": rows, "total": page.Total}, Entities: ents}, nil
		},
	))

	reg.Register(newTool(
		"review_allowed_repos",
		"Which GitHub repositories review-bot is allowed to review on this ring, and whether PR review is enabled.",
		"security", model.PermAgentsRead, false, true,
		tools.ObjectSchema(map[string]any{}),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			return &Result{Data: map[string]any{
				"enabled": d.Reviews.Enabled(),
				"allowed": d.Reviews.AllowedRepos(),
			}}, nil
		},
	))
}

func reviewErr(d Deps, err error) (*Result, error) {
	if errors.Is(err, service.ErrReviewNotConfigured) {
		return &Result{Data: map[string]any{
			"available": false,
			"message":   "PR review is not enabled on this server.",
		}}, nil
	}
	if errors.Is(err, service.ErrReviewRepoNotAllowed) {
		allowed := []string(nil)
		if d.Reviews != nil {
			allowed = d.Reviews.AllowedRepos()
		}
		return &Result{Data: map[string]any{
			"refused": true,
			"reason":  "That repo is not on the review allowlist.",
			"allowed": allowed,
		}}, nil
	}
	if errors.Is(err, service.ErrReviewRunNotFound) {
		return &Result{Missing: []string{"run_id"}, Question: "I couldn't find that PR review. Which run should I check?"}, nil
	}
	return nil, err
}

func parseRepoAndPR(repo string, pr int) (string, int) {
	s := strings.TrimSpace(repo)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")
	if i := strings.Index(s, "github.com/"); i >= 0 {
		rest := s[i+len("github.com/"):]
		parts := strings.Split(rest, "/")
		if len(parts) >= 2 {
			s = parts[0] + "/" + strings.TrimSuffix(parts[1], ".git")
			if pr < 1 && len(parts) >= 4 && (parts[2] == "pull" || parts[2] == "pulls") {
				num := parts[3]
				if j := strings.IndexAny(num, "?#"); j >= 0 {
					num = num[:j]
				}
				if n, err := strconv.Atoi(num); err == nil {
					pr = n
				}
			}
		}
	}
	if i := strings.IndexAny(s, " \t?"); i >= 0 {
		s = s[:i]
	}
	return s, pr
}

func repoOptions(allowed []string) []model.ClarifyOption {
	opts := make([]model.ClarifyOption, 0, len(allowed))
	for _, r := range allowed {
		opts = append(opts, model.ClarifyOption{Label: r, Value: r})
	}
	return opts
}

func reviewRunData(run *model.ReviewRun) map[string]any {
	data := map[string]any{
		"run_id":  run.ID.String(),
		"repo":    run.Repo,
		"pr":      run.PRNumber,
		"status":  run.Status,
		"dry_run": run.DryRun,
	}
	if run.Decision != nil && *run.Decision != "" {
		data["decision"] = *run.Decision
	}
	if run.Verdict != nil && *run.Verdict != "" {
		data["verdict"] = *run.Verdict
	}
	if run.Summary != nil && *run.Summary != "" {
		data["summary"] = *run.Summary
	}
	if run.GitHubURL != nil && *run.GitHubURL != "" {
		data["github_url"] = *run.GitHubURL
	}
	if run.ErrorMessage != nil && *run.ErrorMessage != "" {
		data["error"] = *run.ErrorMessage
	}
	if len(run.StageLog) > 0 {
		data["stage"] = run.StageLog[len(run.StageLog)-1]
	}
	if len(run.Result) > 0 {
		var parsed map[string]any
		if json.Unmarshal(run.Result, &parsed) == nil {
			if data["summary"] == nil {
				if v, ok := parsed["summary"].(string); ok && v != "" {
					data["summary"] = v
				}
			}
			if data["decision"] == nil {
				if v, ok := parsed["decision"].(string); ok && v != "" {
					data["decision"] = v
				}
			}
			if blocking, ok := parsed["blocking"].([]any); ok {
				data["blocking_count"] = len(blocking)
				titles := make([]string, 0, 8)
				for i, item := range blocking {
					if i >= 8 {
						break
					}
					if m, ok := item.(map[string]any); ok {
						if t, ok := m["title"].(string); ok && t != "" {
							titles = append(titles, t)
						}
					}
				}
				if len(titles) > 0 {
					data["blocking"] = titles
				}
			}
		}
	}
	return data
}
