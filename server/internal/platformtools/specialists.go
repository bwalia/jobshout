package platformtools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
	"github.com/jobshout/server/internal/strix"
	"github.com/jobshout/server/internal/tools"
)

func registerSpecialists(reg *Registry, d Deps) {
	if d.Research != nil {
		reg.Register(newTool(
			"research_run",
			"Run a web research task and return sourced findings. Omit unknown fields; the tool will ask. Do not invent a topic.",
			"insight", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"topic":   map[string]any{"type": "string", "description": "Subject to research. Omit if unknown."},
				"context": map[string]any{"type": "string"},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				if !d.Research.Available() {
					return &Result{Data: map[string]any{"available": false, "message": "Research is not configured on this server."}}, nil
				}
				topic := strArg(input, "topic")
				if topic == "" {
					return &Result{Missing: []string{"topic"}, Question: "What should I research?"}, nil
				}
				return startAgentRun(ctx, d, model.BuiltinResearcher, map[string]string{
					"topic":   topic,
					"context": strArg(input, "context"),
				})
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
			"Generate an article end to end from a topic. Omit unknown fields; the tool will ask. Do not invent a topic.",
			"insight", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"topic":   map[string]any{"type": "string", "description": "Subject to write about. Omit if unknown."},
				"context": map[string]any{"type": "string"},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				topic := strArg(input, "topic")
				if topic == "" {
					return &Result{Missing: []string{"topic"}, Question: "What should I write about?"}, nil
				}
				return startAgentRun(ctx, d, model.BuiltinArticleWriter, map[string]string{
					"topic":   topic,
					"context": strArg(input, "context"),
				})
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
					return &Result{Missing: []string{"run_id"}, Question: "Which article run should I check?"}, nil
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
					return &Result{Missing: []string{"run_id"}, Question: "Which article run should I publish?"}, nil
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
					return &Result{Missing: []string{"run_id"}, Question: "Which article run should I cancel?"}, nil
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
			"Start a security test against a target. Omit unknown fields; the tool will ask. Do not invent a target. Only targets in the organisation's authorised scope are accepted; others are refused.",
			"security", model.PermAgentsExecute, true, false,
			tools.ObjectSchema(map[string]any{
				"target":      map[string]any{"type": "string", "description": "URL or path to test. Omit if unknown."},
				"scan_mode":   map[string]any{"type": "string", "enum": []any{"quick", "standard", "deep"}},
				"instruction": map[string]any{"type": "string"},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				target := strArg(input, "target")
				if target == "" {
					return &Result{Missing: []string{"target"}, Question: "What URL or path should I test?"}, nil
				}
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
				res, err := startAgentRun(ctx, d, model.BuiltinPentester, map[string]string{
					"target":      target,
					"scan_mode":   mode,
					"instruction": strArg(input, "instruction"),
				})
				if err != nil {
					// Out-of-scope targets are refused in the service; surface that plainly.
					return &Result{Data: map[string]any{"refused": true, "reason": humaniseError(err)}}, nil
				}
				res.Effect = fmt.Sprintf("start a %s pentest against %s", mode, target)
				return res, nil
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
					return &Result{Missing: []string{"run_id"}, Question: "Which pentest should I report on?"}, nil
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
					return &Result{Missing: []string{"run_id"}, Question: "Which pentest should I cancel?"}, nil
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
			"Generate an image, picture, drawing or illustration from a text prompt. Call this when the user asks to draw, generate, or create a picture — not an agent.",
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
				id := ""
				if res.RecordID != nil {
					id = res.RecordID.String()
				}
				url := res.URL
				if url == "" && len(res.PNG) > 0 {
					// No object storage: keep the picture on the entity so chat
					// can render it. Do not put the bytes in Data — that would
					// dump a megabyte into the model's tool-result context.
					url = "data:image/png;base64," + base64.StdEncoding.EncodeToString(res.PNG)
				}
				ref := imageRef(id, url)
				data := map[string]any{"model": res.Model, "seed": res.Seed}
				if res.URL != "" {
					data["url"] = res.URL
				}
				return &Result{Data: data, Entity: &ref, Entities: []model.EntityRef{ref}}, nil
			},
		))
	}

	if d.Mail != nil {
		reg.Register(newTool(
			"mail_sync",
			"Sync the organisation Gmail inbox now, optionally saving the Mail Agent playbook first: which senders, subject prefixes and labels to watch, which pages to answer from, what to look for in them, and how the reply should read. The agent classifies new mail and drafts replies; nothing is sent.",
			"insight", model.PermAgentsExecute, false, false,
			tools.ObjectSchema(map[string]any{
				"senders":            map[string]any{"type": "string", "description": "Comma-separated senders to watch. Empty means all unread mail."},
				"subject_prefixes":   map[string]any{"type": "string", "description": "Comma-separated subject prefixes."},
				"labels":             map[string]any{"type": "string", "description": "Comma-separated Gmail labels."},
				"knowledge_urls":     map[string]any{"type": "string", "description": "http(s) pages to answer from, one per line or comma-separated."},
				"research_focus":     map[string]any{"type": "string", "description": "What to look for in those pages."},
				"reply_instructions": map[string]any{"type": "string", "description": "How the reply should read: tone, length, must-include."},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				if !d.Mail.Available(ctx, ident.OrgID) {
					return &Result{Data: map[string]any{
						"available": false,
						"message":   "Gmail is not connected. Open Mail Agent to connect the shared mailbox.",
					}}, nil
				}
				// Saving the playbook and syncing are both the mail runner's
				// job now; sending them through the front door is what makes a
				// sync started from chat appear on the board.
				res, err := startAgentRun(ctx, d, model.BuiltinMail, mailSyncInputs(input))
				if err != nil {
					return nil, err
				}
				res.Effect = "queue a mailbox sync"
				return res, nil
			},
		))
		reg.Register(newTool(
			"mail_list_drafts",
			"List Mail Agent drafts waiting for a human to approve (nothing is sent from here).",
			"insight", model.PermAgentsRead, false, true,
			tools.ObjectSchema(map[string]any{}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				out, err := d.Mail.ListPendingDrafts(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 20})
				if err != nil {
					return nil, err
				}
				items := make([]map[string]any, 0, len(out.Data))
				for _, dft := range out.Data {
					items = append(items, map[string]any{
						"id": dft.ID.String(), "subject": dft.Subject, "to": dft.ToEmail, "status": dft.Status,
					})
				}
				ref := model.EntityRef{Kind: model.EntityMailThread, ID: "", Label: "Mail drafts", Href: mailHref()}
				return &Result{Data: map[string]any{"drafts": items, "total": out.Total}, Entity: &ref}, nil
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

// mailSyncInputs narrows a tool call to the playbook fields the caller actually
// supplied, converting stringish tool args on the way. Only present keys
// travel: an omitted field must not wipe a playbook the operator saved in the
// Task Manager, which is the rule MailPlaybookPatch applies on the far side.
// Old comment follows, kept because it still names the canonical patch builder:
// patch builder in the service layer, so the chat tool and the Mail runner
// cannot disagree about what saving a playbook means.
func mailSyncInputs(input map[string]any) map[string]string {
	vals := map[string]string{}
	for k, v := range input {
		if v == nil {
			continue
		}
		vals[k] = strings.TrimSpace(fmt.Sprint(v))
	}
	return vals
}
