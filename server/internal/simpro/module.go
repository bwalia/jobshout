package simpro

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

// Module is the Simpro Payments and Invoicing Agent specialist.
func Module(client *Client) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinSimproPayments,
		Label:    "Simpro Payments",
		Icon:     "receipt",
		TabSlug:  "simpro-payments",
		Hint:     "Simpro AR mailbox, payment status, month-end, and UK F-Gas refrigerant reporting.",
		ChatHint: "For Simpro invoicing, payments, or F-Gas refrigerant compliance — call agent_execute on the Simpro Payments and Invoicing Agent.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(client),
		StayOnTab: true,
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			p := strings.ToLower(prompt)
			if vals["action"] != "" {
				return
			}
			switch {
			case strings.Contains(p, "fgas") || strings.Contains(p, "f-gas") || strings.Contains(p, "refrigerant"):
				vals["action"] = "fgas_report"
			case strings.Contains(p, "aging") || strings.Contains(p, "overdue"):
				vals["action"] = "invoice_aging"
			case strings.Contains(p, "payment"):
				vals["action"] = "payment_status"
			case strings.Contains(p, "reconcil"):
				vals["action"] = "reconcile_preview"
			case strings.Contains(p, "month") || strings.Contains(p, "period close"):
				vals["action"] = "month_end"
			default:
				vals["action"] = "summary"
			}
		},
		Ready: func(ctx context.Context, _ uuid.UUID) []agentmodule.Issue {
			if client == nil {
				return []agentmodule.Issue{{
					Severity: "warning", Code: "simpro_unconfigured",
					Message: "Simpro client not initialised.",
				}}
			}
			st := client.Status(ctx)
			if client.LiveConfigured() && st["ok"] == false {
				return []agentmodule.Issue{{
					Severity: "warning", Code: "simpro_unreachable",
					Message: fmt.Sprint(st["message"]),
				}}
			}
			if !client.LiveConfigured() {
				return []agentmodule.Issue{{
					Severity: "info", Code: "simpro_demo_mode",
					Message: "Demo fixtures active (Watford HVAC). Set SIMPRO_API_KEY + SIMPRO_BASE_URL for live reads.",
				}}
			}
			return nil
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinSimproPayments,
		Hint:    "Run Simpro payments, invoicing, or F-Gas compliance playbooks.",
		Fields: []agentschema.Field{
			{
				Key: "action", Label: "Action", Type: "select", Required: true, Default: "summary",
				Question: "What should the Simpro Payments agent do?",
				Options: []model.ClarifyOption{
					{Label: "AR / payments summary", Value: "summary"},
					{Label: "Invoice aging", Value: "invoice_aging"},
					{Label: "Payment status", Value: "payment_status"},
					{Label: "Reconcile preview (read-only)", Value: "reconcile_preview"},
					{Label: "F-Gas / refrigerant report", Value: "fgas_report"},
					{Label: "Month-end checklist", Value: "month_end"},
				},
			},
		},
		TitleRules: []agentschema.TitleRule{
			{IfKey: "action", Format: "Simpro Payments: {action}", Fallback: "Simpro Payments run"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "action", Prefix: "Action: "},
		},
	}
}

// Seed is the built-in Simpro Payments and Invoicing Agent.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Automates Simpro AR invoicing and payment workflows, and closes the UK F-Gas refrigerant compliance gap with an audit-ready usage ledger. Demo fixtures always work; live reads when SIMPRO_API_KEY is set. Writes discussed with customer — not shipped."
	prompt := "You are the Simpro Payments and Invoicing Agent. You summarise open invoices, aging, payments, and F-Gas refrigerant usage for Simpro Premium builds. Prefer agent_execute with actions summary, invoice_aging, payment_status, reconcile_preview, fgas_report, or month_end. Never invent live Simpro figures when demo mode is active — say so. Do not claim write-back to Simpro is live; it is discussed-not-shipped."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameSimproPayments,
		Role:         "Simpro Payments & Invoicing",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinSimproPayments},
	}
}

func launch(client *Client) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if client == nil {
			return nil, fmt.Errorf("simpro client not configured")
		}
		action := strings.TrimSpace(in.Values["action"])
		if action == "" {
			action = "summary"
		}
		meta := map[string]any{"simpro_action": action}
		tab := "Open: /panel/task-manager?agent=simpro-payments"

		var (
			msg  string
			desc string
		)

		switch action {
		case "summary":
			sum, err := client.Summary(ctx)
			if err != nil {
				return nil, err
			}
			msg = "Simpro payments summary ready"
			desc = fmt.Sprintf("Mode=%s outstanding=£%.0f overdue=%d fgas_gaps=%d\n\n%s\n\n%s",
				sum.Mode, sum.Mailbox.OutstandingGBP, sum.Mailbox.Overdue, sum.FGas.Gaps, compactJSON(sum), tab)
			meta["summary"] = sum

		case "invoice_aging":
			body, err := client.AgingReport(ctx)
			if err != nil {
				return nil, err
			}
			msg = "Invoice aging report ready"
			desc = fmt.Sprintf("%s\n\n%s", compactJSON(body), tab)
			meta["aging"] = body

		case "payment_status":
			payments, mode, err := client.ListPayments(ctx)
			if err != nil {
				return nil, err
			}
			msg = "Payment status loaded"
			desc = fmt.Sprintf("Mode=%s payments=%d\n\n%s\n\n%s", mode, len(payments), compactJSON(payments), tab)
			meta["payments"] = payments
			meta["mode"] = mode

		case "reconcile_preview":
			body, err := client.ReconcilePreview(ctx)
			if err != nil {
				return nil, err
			}
			msg = "Reconciliation preview ready (read-only)"
			desc = fmt.Sprintf("%s\n\n%s", compactJSON(body), tab)
			meta["reconcile"] = body

		case "fgas_report":
			body, err := client.FGasReport(ctx)
			if err != nil {
				return nil, err
			}
			msg = "F-Gas refrigerant report ready"
			desc = fmt.Sprintf("%s\n\n%s", compactJSON(body), tab)
			meta["fgas"] = body

		case "month_end":
			body, err := client.MonthEndChecklist(ctx)
			if err != nil {
				return nil, err
			}
			msg = "Month-end checklist ready"
			desc = fmt.Sprintf("%s\n\n%s", compactJSON(body), tab)
			meta["month_end"] = body

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

func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	s := string(b)
	if len(s) > 1400 {
		return s[:1400] + "…"
	}
	return s
}
