package platformtools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/research"
	"github.com/jobshout/server/internal/service"
	"github.com/jobshout/server/internal/strix"
	"github.com/jobshout/server/internal/tools"
)

func registerSpecialists(reg *Registry, d Deps) {
	if d.Research != nil {
		reg.Register(newTool(
			"research_run",
			"Run a web research task and return sourced findings.",
			"insight", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"topic":   map[string]any{"type": "string"},
				"context": map[string]any{"type": "string"},
			}, "topic"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				if !d.Research.Available() {
					return &Result{Data: map[string]any{"available": false, "message": "Research is not configured on this server."}}, nil
				}
				brief, err := d.Research.Research(ctx, ident.OrgID, research.Request{
					Topic:   strArg(input, "topic"),
					Context: strArg(input, "context"),
				}, nil)
				if err != nil {
					return nil, err
				}
				return &Result{Data: brief}, nil
			},
		))
		reg.Register(newTool(
			"trending_topics",
			"What is trending in AI infrastructure and technology right now.",
			"insight", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{
				"limit": map[string]any{"type": "integer"},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				items, err := d.Research.Trending(ctx, intArg(input, "limit", 8))
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"topics": items}}, nil
			},
		))
	}

	if d.Blog != nil {
		reg.Register(newTool(
			"article_generate",
			"Generate an article end to end from a topic.",
			"insight", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"topic":   map[string]any{"type": "string"},
				"context": map[string]any{"type": "string"},
			}, "topic"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				uid := ident.UserID
				run, err := d.Blog.Generate(ctx, ident.OrgID, &uid, "chat", model.GenerateBlogRequest{
					Topics: []string{strArg(input, "topic")},
				})
				if err != nil {
					return nil, err
				}
				ref := model.EntityRef{Kind: model.EntityArticle, ID: run.ID.String(), Label: strArg(input, "topic"), Href: articleHref(run.ID)}
				return &Result{Data: map[string]any{"status": run.Status}, Entity: &ref}, nil
			},
		))
		reg.Register(newTool(
			"article_run_get",
			"Report article run progress and generated titles.",
			"insight", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{
				"run_id": map[string]any{"type": "string"},
			}, "run_id"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				id, err := uuid.Parse(strArg(input, "run_id"))
				if err != nil {
					return &Result{Missing: []string{"article_run"}, Question: "Which article run should I check?"}, nil
				}
				run, err := d.Blog.GetByID(ctx, id)
				if err != nil {
					return nil, err
				}
				ident := MustIdentity(ctx)
				if run.OrgID != ident.OrgID {
					return nil, errNotInOrg
				}
				ref := model.EntityRef{Kind: model.EntityArticle, ID: run.ID.String(), Label: "article run", Href: articleHref(run.ID)}
				return &Result{Data: map[string]any{"status": run.Status, "error": run.ErrorMessage}, Entity: &ref}, nil
			},
		))
		reg.Register(newTool(
			"article_publish",
			"Publish a completed article run to the CMS as drafts.",
			"insight", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"run_id": map[string]any{"type": "string"},
			}, "run_id"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				id, err := uuid.Parse(strArg(input, "run_id"))
				if err != nil {
					return &Result{Missing: []string{"article_run"}, Question: "Which article run should I publish?"}, nil
				}
				run, err := d.Blog.Publish(ctx, ident.OrgID, id)
				if err != nil {
					return nil, err
				}
				ref := model.EntityRef{Kind: model.EntityArticle, ID: run.ID.String(), Label: "article run", Href: articleHref(run.ID)}
				return &Result{Data: map[string]any{"status": run.Status}, Entity: &ref}, nil
			},
		))
		reg.Register(newTool(
			"article_run_cancel",
			"Cancel an in-flight article run. Requires confirmation.",
			"insight", model.PermAgentsExecute, true, false,
			tools.ObjectSchema(map[string]any{
				"run_id": map[string]any{"type": "string"},
			}, "run_id"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				id, err := uuid.Parse(strArg(input, "run_id"))
				if err != nil {
					return &Result{Missing: []string{"article_run"}, Question: "Which article run should I cancel?"}, nil
				}
				run, err := d.Blog.Cancel(ctx, ident.OrgID, id)
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"status": run.Status}, Effect: "cancel the article run and mark it failed"}, nil
			},
		))
	}

	if d.Pentest != nil {
		reg.Register(newTool(
			"pentest_start",
			"Start a security test against a target. Only targets in the organisation's authorised scope are accepted; others are refused.",
			"security", model.PermAgentsExecute, true, false,
			tools.ObjectSchema(map[string]any{
				"target":      map[string]any{"type": "string"},
				"scan_mode":   map[string]any{"type": "string", "enum": []any{"quick", "standard", "deep"}},
				"instruction": map[string]any{"type": "string"},
			}, "target"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				target := strArg(input, "target")
				if !pentestTargetAllowed(target) {
					return &Result{Data: map[string]any{
						"refused": true,
						"reason":  "That target is not in the authorised pentest scope. I will not scan it.",
					}}, nil
				}
				mode := strArg(input, "scan_mode")
				if mode == "" {
					mode = "quick"
				}
				agents, err := d.Agents.List(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 100}, repository.AgentListFilter{})
				if err != nil {
					return nil, err
				}
				var pentester *model.Agent
				for i := range agents.Data {
					if agents.Data[i].IsBuiltin(model.BuiltinPentester) {
						pentester = &agents.Data[i]
						break
					}
				}
				if pentester == nil {
					return &Result{Data: map[string]any{"message": "No penetration testing agent is configured for this organisation."}}, nil
				}
				uid := ident.UserID
				run, err := d.Pentest.CreateRun(ctx, model.CreatePentestRunRequest{
					AgentID:     pentester.ID,
					Target:      target,
					ScanMode:    mode,
					Instruction: strArg(input, "instruction"),
				}, ident.OrgID, &uid)
				if err != nil {
					// Out-of-scope targets are refused in the service; surface that plainly.
					return &Result{Data: map[string]any{"refused": true, "reason": humaniseError(err)}}, nil
				}
				ref := model.EntityRef{Kind: model.EntityPentest, ID: run.ID.String(), Label: target, Href: pentestHref()}
				return &Result{
					Data:   map[string]any{"target": target, "status": run.Status, "mode": mode},
					Entity: &ref,
					Effect: fmt.Sprintf("start a %s pentest against %s", mode, target),
				}, nil
			},
		))
		reg.Register(newTool(
			"pentest_findings",
			"Report pentest findings in readable severity order.",
			"security", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{
				"run_id": map[string]any{"type": "string"},
			}, "run_id"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				id, err := uuid.Parse(strArg(input, "run_id"))
				if err != nil {
					return &Result{Missing: []string{"pentest_run"}, Question: "Which pentest should I report on?"}, nil
				}
				findings, err := d.Pentest.GetFindings(ctx, id, ident.OrgID)
				if err != nil {
					return nil, err
				}
				ref := model.EntityRef{Kind: model.EntityPentest, ID: id.String(), Label: "pentest", Href: pentestHref()}
				return &Result{Data: map[string]any{"findings": findings, "count": len(findings)}, Entity: &ref}, nil
			},
		))
		reg.Register(newTool(
			"pentest_cancel",
			"Cancel an in-flight pentest. Requires confirmation.",
			"security", model.PermAgentsExecute, true, false,
			tools.ObjectSchema(map[string]any{
				"run_id": map[string]any{"type": "string"},
			}, "run_id"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				id, err := uuid.Parse(strArg(input, "run_id"))
				if err != nil {
					return &Result{Missing: []string{"pentest_run"}, Question: "Which pentest should I cancel?"}, nil
				}
				run, err := d.Pentest.Cancel(ctx, id, ident.OrgID)
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"status": run.Status}, Effect: "cancel the in-flight pentest"}, nil
			},
		))
	}

	if d.Images != nil {
		reg.Register(newTool(
			"image_generate",
			"Generate an image from a text prompt.",
			"insight", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"prompt": map[string]any{"type": "string"},
				"width":  map[string]any{"type": "integer"},
				"height": map[string]any{"type": "integer"},
			}, "prompt"),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				uid := ident.UserID
				res, err := d.Images.Generate(ctx, service.GenerateImageRequest{
					OrgID:  ident.OrgID,
					UserID: &uid,
					Prompt: strArg(input, "prompt"),
					Width:  intArg(input, "width", 0),
					Height: intArg(input, "height", 0),
					Source: "chat",
				})
				if err != nil {
					return nil, err
				}
				b64 := base64.StdEncoding.EncodeToString(res.PNG)
				if len(b64) > 200 {
					b64 = b64[:200] + "…"
				}
				id := ""
				if res.RecordID != nil {
					id = res.RecordID.String()
				}
				ref := model.EntityRef{Kind: model.EntityImage, ID: id, Label: "generated image", Href: "/images"}
				return &Result{Data: map[string]any{"url": res.URL, "model": res.Model, "seed": res.Seed}, Entity: &ref}, nil
			},
		))
	}
}

func pentestTargetAllowed(target string) bool {
	cfg := strix.LoadConfig(nil)
	lower := strings.ToLower(target)
	if len(cfg.TargetAllowlist) > 0 {
		for _, a := range cfg.TargetAllowlist {
			if strings.Contains(lower, strings.ToLower(a)) {
				return true
			}
		}
		return false
	}
	for _, bad := range []string{"google.com", "facebook.com", "github.com", "microsoft.com", "amazon.com", "apple.com"} {
		if strings.Contains(lower, bad) {
			return false
		}
	}
	return strings.TrimSpace(target) != ""
}
