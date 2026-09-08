package creditcontroller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Module is the Credit Controller Agent specialist.
//
// All specialists are wired this way: own package, then one Register call.
func Module(client *Client) agentmodule.Module {
	return agentmodule.Module{
		Builtin:   model.BuiltinCreditController,
		Label:     "Credit Controller",
		Icon:      "landmark",
		TabSlug:   "credit-controller",
		Hint:      "AP invoice mailbox, AI weekly/monthly batches, triage → approve → audit.",
		ChatHint:  "For AP invoice work — mailbox, AI sample generation, triage, or credit-controller month-end — call agent_execute on the Credit Controller Agent.",
		Schema:    schema(),
		Seed:      Seed,
		Launch:    launch(client),
		StayOnTab: true,
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			p := strings.ToLower(prompt)
			if vals["action"] != "" {
				return
			}
			switch {
			case strings.Contains(p, "weekly"):
				vals["action"] = "generate_weekly"
			case strings.Contains(p, "monthly") || strings.Contains(p, "generate"):
				vals["action"] = "generate_monthly"
			case strings.Contains(p, "triage") || strings.Contains(p, "batch"):
				vals["action"] = "triage_untriaged"
			case strings.Contains(p, "month-end") || strings.Contains(p, "month end"):
				vals["action"] = "month_end"
			default:
				vals["action"] = "summary"
			}
		},
		Ready: func(ctx context.Context, _ uuid.UUID) []agentmodule.Issue {
			if client == nil || !client.Enabled() {
				return []agentmodule.Issue{{
					Severity: "warning", Code: "aivc_unconfigured",
					Message: "AIVC_BASE_URL is not set. Point it at the aivc-agents API (default http://127.0.0.1:8000).",
				}}
			}
			if _, err := client.Ping(ctx); err != nil {
				return []agentmodule.Issue{{
					Severity: "warning", Code: "aivc_unreachable",
					Message: "Credit Controller runtime unreachable at " + client.baseURL + ". Run `make serve` in aivc-agents.",
				}}
			}
			return nil
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinCreditController,
		Hint:    "Run the credit-controller playbook against the AP invoice mailbox.",
		Fields: []agentschema.Field{
			{
				Key: "action", Label: "Action", Type: "select", Required: true, Default: "summary",
				Question: "What should the Credit Controller do?",
				Options: []model.ClarifyOption{
					{Label: "Mailbox summary", Value: "summary"},
					{Label: "Generate 10 monthly invoices (AI)", Value: "generate_monthly"},
					{Label: "Generate 10 weekly invoices (AI)", Value: "generate_weekly"},
					{Label: "Triage untriaged invoices", Value: "triage_untriaged"},
					{Label: "Triage one invoice", Value: "triage_one"},
					{Label: "Full month-end run", Value: "month_end"},
				},
			},
			{
				Key: "invoice_id", Label: "Invoice ID", Type: "text",
				Placeholder: "INV-1005", Help: "Required for “Triage one invoice”.",
				Question: "Which invoice id should I triage?",
			},
			{
				Key: "count", Label: "Batch size", Type: "number", Default: "10", Min: 1,
				Help: "Used for AI generate actions (1–20).",
			},
		},
		TitleRules: []agentschema.TitleRule{
			{IfKey: "action", Format: "Credit Controller: {action}", Fallback: "Credit Controller run"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "action", Prefix: "Action: "},
			{Key: "invoice_id", Prefix: "Invoice: "},
		},
	}
}

// Seed is the built-in Credit Controller Agent.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Runs the AP credit-controller job: invoice mailbox, AI weekly/monthly sample generation, durable triage, approval queue, and audit."
	prompt := "You are the Credit Controller Agent. You triage supplier invoices, escalate bank-detail and sanctions exceptions, enforce segregation of duties, and never invent PO or bank figures. Prefer agent_execute with a clear action (summary, generate_monthly, triage_untriaged, month_end)."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameCreditController,
		Role:         "Credit Controller",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinCreditController},
	}
}

func launch(client *Client) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if client == nil || !client.Enabled() {
			return nil, fmt.Errorf("credit controller runtime not configured (set AIVC_BASE_URL)")
		}
		action := strings.TrimSpace(in.Values["action"])
		if action == "" {
			action = "summary"
		}
		count := 10
		if v := strings.TrimSpace(in.Values["count"]); v != "" {
			fmt.Sscanf(v, "%d", &count)
		}

		var (
			msg  string
			desc string
			meta = map[string]any{"credit_controller_action": action}
		)

		switch action {
		case "summary":
			sum, err := client.CreditControllerSummary(ctx)
			if err != nil {
				return nil, err
			}
			msg = "Credit controller summary ready"
			desc = fmt.Sprintf("Mailbox summary loaded. Open: /panel/task-manager?agent=credit-controller\n\n%v", compactJSON(sum))
			meta["summary"] = sum

		case "generate_monthly", "generate_weekly":
			cadence := "monthly"
			if action == "generate_weekly" {
				cadence = "weekly"
			}
			out, err := client.GenerateInvoices(ctx, cadence, count)
			if err != nil {
				return nil, err
			}
			msg = fmt.Sprintf("Generated %s invoice batch", cadence)
			if m, ok := out["message"].(string); ok && m != "" {
				msg = m
			}
			desc = msg + "\n\nOpen: /panel/task-manager?agent=credit-controller"
			meta["batch"] = out

		case "triage_one":
			id := strings.TrimSpace(in.Values["invoice_id"])
			if id == "" {
				return nil, fmt.Errorf("invoice_id is required for triage_one")
			}
			out, err := client.Triage(ctx, id)
			if err != nil {
				return nil, err
			}
			msg = fmt.Sprintf("Triaged %s", id)
			desc = fmt.Sprintf("%s → status=%v\n\nOpen: /panel/task-manager?agent=credit-controller", id, out["status"])
			meta["triage"] = out

		case "triage_untriaged":
			list, err := client.ListInvoices(ctx)
			if err != nil {
				return nil, err
			}
			ids := untriagedIDs(list)
			if len(ids) == 0 {
				msg = "No untriaged invoices"
				desc = msg
				break
			}
			if len(ids) > 15 {
				ids = ids[:15]
			}
			out, err := client.TriageBatch(ctx, ids)
			if err != nil {
				return nil, err
			}
			msg = fmt.Sprintf("Triaged %d invoices", len(ids))
			desc = fmt.Sprintf("%s (awaiting=%v succeeded=%v)\n\nOpen: /panel/task-manager?agent=credit-controller",
				msg, out["awaiting_approval"], out["succeeded"])
			meta["triage_batch"] = out

		case "month_end":
			if _, err := client.GenerateInvoices(ctx, "monthly", count); err != nil {
				return nil, err
			}
			list, err := client.ListInvoices(ctx)
			if err != nil {
				return nil, err
			}
			ids := untriagedIDs(list)
			if len(ids) > 15 {
				ids = ids[:15]
			}
			var batch map[string]any
			if len(ids) > 0 {
				batch, err = client.TriageBatch(ctx, ids)
				if err != nil {
					return nil, err
				}
			}
			msg = "Month-end credit controller run complete"
			desc = "Generated monthly samples and triaged the mailbox. Approve exceptions on the Credit Controller tab.\n\nOpen: /panel/task-manager?agent=credit-controller"
			meta["triage_batch"] = batch

		default:
			return nil, fmt.Errorf("unknown action %q", action)
		}

		return &agentmodule.LaunchOutput{
			Message:     msg,
			Description: desc,
			Status:      "done",
			ExtraMeta:   meta,
		}, nil
	}
}

func untriagedIDs(list map[string]any) []string {
	raw, _ := list["invoices"].([]any)
	var ids []string
	for _, row := range raw {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		if m["workflow"] != nil {
			continue
		}
		id, _ := m["invoice_id"].(string)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	s := string(b)
	if len(s) > 1200 {
		return s[:1200] + "…"
	}
	return s
}
