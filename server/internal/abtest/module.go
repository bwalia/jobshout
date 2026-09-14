package abtest

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/wslproxymcp"
)

// Module is the AB Testing Agent specialist (wslproxy traffic splits).
func Module(client *Client) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinABTesting,
		Label:    "AB Testing",
		Icon:     "split",
		TabSlug:  "ab-testing",
		Hint:     "Manage wslproxy weighted / canary traffic splits and observe live assignment.",
		ChatHint: "For A/B tests, canary weights, or traffic splits on wslproxy — call agent_execute on the AB Testing Agent.",
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
			case strings.Contains(p, "promot"):
				vals["action"] = "promote"
			case strings.Contains(p, "rollback") || strings.Contains(p, "roll back"):
				vals["action"] = "rollback"
			case strings.Contains(p, "weight") || strings.Contains(p, "split") || strings.Contains(p, "80") || strings.Contains(p, "canary"):
				vals["action"] = "set_weights"
			case strings.Contains(p, "observ") || strings.Contains(p, "sample") || strings.Contains(p, "converg"):
				vals["action"] = "observe"
			default:
				vals["action"] = "list"
			}
		},
		Ready: func(ctx context.Context, _ uuid.UUID) []agentmodule.Issue {
			if client == nil {
				return []agentmodule.Issue{{
					Severity: "warning", Code: "abtest_unconfigured",
					Message: "AB Testing client not initialised.",
				}}
			}
			if !client.LiveConfigured() {
				return []agentmodule.Issue{{
					Severity: "info", Code: "abtest_demo_mode",
					Message: "Demo fixtures active. Set WSLPROXY_BASE_URL + WSLPROXY_MCP_API_KEY for live MCP traffic tools.",
				}}
			}
			st := client.Status(ctx)
			if st["ok"] == false {
				return []agentmodule.Issue{{
					Severity: "warning", Code: "abtest_mcp_unreachable",
					Message: fmt.Sprint(st["message"]),
				}}
			}
			return nil
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinABTesting,
		Hint:    "List, weight, promote, rollback, or observe a wslproxy A/B experiment.",
		Fields: []agentschema.Field{
			{
				Key: "action", Label: "Action", Type: "select", Required: true, Default: "list",
				Options: []agentschema.Option{
					{Label: "List experiments", Value: "list"},
					{Label: "Set traffic weights", Value: "set_weights"},
					{Label: "Promote backend", Value: "promote"},
					{Label: "Rollback", Value: "rollback"},
					{Label: "Observe live split", Value: "observe"},
				},
			},
			{
				Key: "experiment_id", Label: "Experiment", Type: "text", Required: false, Default: "abtesting",
				Placeholder: "abtesting", Help: "Experiment id or host",
			},
			{
				Key: "stable_weight", Label: "v1 / stable weight %", Type: "number", Required: false, Default: "80",
				Help: "Used by set_weights",
			},
			{
				Key: "canary_weight", Label: "v2 / canary weight %", Type: "number", Required: false, Default: "20",
				Help: "Used by set_weights",
			},
			{
				Key: "promote_label", Label: "Promote label", Type: "text", Required: false, Default: "v2",
				Placeholder: "v2",
			},
			{
				Key: "observe_n", Label: "Observe samples", Type: "number", Required: false, Default: "40",
			},
		},
	}
}

// Seed creates the builtin agent row for a new organisation.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Manages wslproxy weighted and canary traffic splits (A/B testing). Drives update_traffic_split / promote / rollback over MCP and observes live assignment — replacement for the standalone abtesting.fictionally.org control UI."
	prompt := "You are the AB Testing Agent. Prefer agent_execute with actions list, set_weights, promote, rollback, or observe. Use wslproxy MCP for live weight changes. Never invent live traffic counts when demo mode is active — say so."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         "AB Testing Agent",
		Role:         "AB Testing & Canary",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinABTesting},
	}
}

func launch(client *Client) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if client == nil {
			return nil, fmt.Errorf("AB Testing agent not configured")
		}
		action := strings.TrimSpace(in.Values["action"])
		if action == "" {
			action = "list"
		}
		expID := strings.TrimSpace(in.Values["experiment_id"])
		if expID == "" {
			expID = "abtesting"
		}

		meta := map[string]any{"action": action, "experiment_id": expID}
		tab := "Open: /panel/task-manager?agent=ab-testing"

		var (
			msg  string
			body any
			err  error
		)

		switch action {
		case "list":
			list, mode, e := client.ListExperiments(ctx)
			err = e
			body = map[string]any{"mode": mode, "experiments": list}
			msg = "AB experiments listed"
		case "set_weights":
			sw := parseWeight(in.Values["stable_weight"], 80)
			cw := parseWeight(in.Values["canary_weight"], 20)
			body, err = client.SetWeights(ctx, expID, []wslproxymcp.BackendWeight{
				{Label: "v1", Weight: sw},
				{Label: "v2", Weight: cw},
			})
			msg = "Traffic weights updated"
		case "promote":
			label := strings.TrimSpace(in.Values["promote_label"])
			if label == "" {
				label = "v2"
			}
			body, err = client.Promote(ctx, expID, label)
			msg = "Backend promoted"
		case "rollback":
			body, err = client.Rollback(ctx, expID)
			msg = "Traffic rolled back"
		case "observe":
			n := int(parseWeight(in.Values["observe_n"], 40))
			body, err = client.Observe(ctx, expID, n)
			msg = "Observe samples ready"
		default:
			return nil, fmt.Errorf("unknown action %q", action)
		}
		if err != nil {
			return nil, err
		}
		raw, _ := json.MarshalIndent(body, "", "  ")
		desc := string(raw)
		if len(desc) > 4000 {
			desc = desc[:4000] + "…"
		}
		desc = desc + "\n\n" + tab
		meta["result"] = body
		return &agentmodule.LaunchOutput{
			Message:     msg,
			Description: desc,
			Status:      "done",
			ExtraMeta:   meta,
		}, nil
	}
}

func parseWeight(s string, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return f
}
