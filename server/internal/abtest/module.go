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
		Builtin:   model.BuiltinABTesting,
		Label:     "AB Testing",
		Icon:      "split",
		TabSlug:   "ab-testing",
		Hint:      "Manage wslproxy weighted / canary traffic splits and observe live assignment.",
		ChatHint:  "For A/B tests, canary weights, or traffic splits on wslproxy — call agent_execute on the AB Testing Agent.",
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
			return ready(ctx, client)
		},
	}
}

// ready turns Status and the resolved experiment into preview issues.
func ready(ctx context.Context, client *Client) []agentmodule.Issue {
	if client == nil {
		return []agentmodule.Issue{{
			Severity: "warning", Code: "abtest_unconfigured",
			Message: "AB Testing client not initialised.",
		}}
	}
	st := client.Status(ctx)
	msg := fmt.Sprint(st["message"])
	switch st["mode"] {
	case ModeDemo:
		return []agentmodule.Issue{{Severity: "info", Code: "abtest_demo_mode", Message: msg}}
	case ModeUnavailable:
		return []agentmodule.Issue{{Severity: "warning", Code: "abtest_wslproxy_unreachable", Message: msg}}
	case ModeReadOnly:
		return []agentmodule.Issue{{Severity: "warning", Code: "abtest_no_write_path", Message: msg}}
	}
	var issues []agentmodule.Issue
	if mcpSt, _ := st["mcp"].(map[string]any); mcpSt != nil && mcpSt["tools_enabled"] == false && st["write_path"] == PathREST {
		issues = append(issues, agentmodule.Issue{
			Severity: "info", Code: "abtest_mcp_tools_disabled",
			Message: fmt.Sprint(mcpSt["message"], " Writes use the wslproxy admin API instead."),
		})
	}
	if list, _, err := client.ListExperiments(ctx); err == nil {
		for _, ex := range list {
			if !ex.Writable {
				issues = append(issues, agentmodule.Issue{
					Severity: "warning", Code: "abtest_experiment_read_only",
					Message: ex.ReadOnlyReason,
				})
			}
		}
	}
	return issues
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
				Key: "stable_weight", Label: "Stable weight %", Type: "number", Required: false, Default: "80",
				Help: "set_weights: weight for the rule's first backend",
			},
			{
				Key: "canary_weight", Label: "Canary weight %", Type: "number", Required: false, Default: "20",
				Help: "set_weights: weight for the rule's second backend",
			},
			{
				Key: "promote_label", Label: "Promote backend label", Type: "text", Required: false,
				Placeholder: "second backend", Help: "promote: backend label; empty promotes the rule's second backend",
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
			var ex *Experiment
			ex, err = client.GetExperiment(ctx, expID)
			if err != nil {
				break
			}
			if len(ex.Backends) < 2 {
				err = &NotWritableError{Experiment: ex.ID, Reason: readOnlyOr(ex, "the rule has fewer than two backends")}
				break
			}
			body, err = client.SetWeights(ctx, expID, []wslproxymcp.BackendWeight{
				{Label: ex.Backends[0].Label, Weight: parseWeight(in.Values["stable_weight"], 80)},
				{Label: ex.Backends[1].Label, Weight: parseWeight(in.Values["canary_weight"], 20)},
			})
			msg = "Traffic weights updated"
		case "promote":
			body, err = client.Promote(ctx, expID, in.Values["promote_label"])
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
			return nil, fmt.Errorf("AB Testing %s: %w", action, err)
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

func readOnlyOr(ex *Experiment, def string) string {
	if ex.ReadOnlyReason != "" {
		return ex.ReadOnlyReason
	}
	return def
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
