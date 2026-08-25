package platformtools

import (
	"context"
	"time"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/tools"
)

func registerInsight(reg *Registry, d Deps) {
	month := func() (time.Time, time.Time) {
		now := time.Now().UTC()
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return from, now
	}

	if d.Analytics != nil {
		reg.Register(newTool(
			"usage_summary",
			"Organisation LLM usage and spend for the current month.",
			"insight", model.PermCostRead, false, true,
			tools.ObjectSchema(map[string]any{}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				from, to := month()
				sum, err := d.Analytics.OrgUsageSummary(ctx, ident.OrgID, from, to)
				if err != nil {
					return nil, err
				}
				return &Result{Data: sum}, nil
			},
		))
		if d.Agents != nil {
			reg.Register(newTool(
				"agent_analytics",
				"Per-agent analytics: success rate, latency, cost. Name the agent.",
				"insight", model.PermAnalyticsRead, false, true,
				tools.ObjectSchema(map[string]any{
					"name": map[string]any{"type": "string"},
				}, "name"),
				func(ctx context.Context, input map[string]any) (*Result, error) {
					ident := MustIdentity(ctx)
					agents, err := d.Agents.List(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 100}, repository.AgentListFilter{})
					if err != nil {
						return nil, err
					}
					m := ByName(agents.Data, strArg(input, "name"), func(a model.Agent) string { return a.Name })
					if !m.Found {
						return clarifyFromMatch("agent", strArg(input, "name"), m.Candidates, func(a model.Agent) string { return a.Name }), nil
					}
					from, to := month()
					a, err := d.Analytics.AgentAnalytics(ctx, m.Exact.ID, from, to)
					if err != nil {
						return nil, err
					}
					ref := agentRef(m.Exact)
					return &Result{Data: a, Entity: &ref}, nil
				},
			))
		}
		if d.Tasks != nil {
			reg.Register(newTool(
				"task_metrics",
				"Task completion counts for the organisation.",
				"insight", model.PermAnalyticsRead, false, true,
				tools.ObjectSchema(map[string]any{}),
				func(ctx context.Context, input map[string]any) (*Result, error) {
					ident := MustIdentity(ctx)
					res, err := d.Tasks.ListByOrg(ctx, ident.OrgID, model.PaginationParams{Page: 1, PerPage: 200})
					if err != nil {
						return nil, err
					}
					counts := map[string]int{}
					for _, t := range res.Data {
						counts[t.Status]++
					}
					return &Result{Data: map[string]any{"by_status": counts, "total": res.Total}}, nil
				},
			))
		}
	}

	if d.Leaderboard != nil {
		reg.Register(newTool(
			"leaderboard",
			"Top performing agents by success rate, latency and cost.",
			"insight", model.PermAnalyticsRead, false, true,
			tools.ObjectSchema(map[string]any{
				"limit": map[string]any{"type": "integer"},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				from, to := month()
				list, err := d.Leaderboard.Leaderboard(ctx, ident.OrgID, intArg(input, "limit", 10), from, to)
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"leaderboard": list}}, nil
			},
		))
		reg.Register(newTool(
			"anomalies",
			"Detect unusual agent behaviour (latency, cost, failure spikes).",
			"insight", model.PermAnalyticsRead, false, true,
			tools.ObjectSchema(map[string]any{}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				from, to := month()
				list, err := d.Leaderboard.DetectAnomalies(ctx, ident.OrgID, from, to)
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"anomalies": list}}, nil
			},
		))
	}

	if d.Governance != nil {
		reg.Register(newTool(
			"budget_status",
			"Current budget and remaining spend.",
			"insight", model.PermBudgetsRead, false, true,
			tools.ObjectSchema(map[string]any{}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				list, err := d.Governance.ListBudgets(ctx, ident.OrgID)
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"budgets": list}}, nil
			},
		))
		reg.Register(newTool(
			"policy_list",
			"List governance policies and what they enforce.",
			"insight", model.PermPoliciesRead, false, true,
			tools.ObjectSchema(map[string]any{}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				list, err := d.Governance.ListPolicies(ctx, ident.OrgID)
				if err != nil {
					return nil, err
				}
				return &Result{Data: map[string]any{"policies": list}}, nil
			},
		))
	}

	if d.Audit != nil {
		reg.Register(newTool(
			"audit_search",
			"Search the audit log: who did what recently. Optional action or resource filter.",
			"insight", model.PermAuditRead, false, true,
			tools.ObjectSchema(map[string]any{
				"action":   map[string]any{"type": "string"},
				"resource": map[string]any{"type": "string"},
			}),
			func(ctx context.Context, input map[string]any) (*Result, error) {
				ident := MustIdentity(ctx)
				params := model.AuditQueryParams{
					Action:   strArg(input, "action"),
					Resource: strArg(input, "resource"),
					Limit:    25,
				}
				list, err := d.Audit.ListActions(ctx, ident.OrgID, params)
				if err != nil {
					return nil, err
				}
				type row struct {
					Action   string `json:"action"`
					Resource string `json:"resource"`
					When     string `json:"when"`
				}
				rows := make([]row, 0, len(list))
				for _, a := range list {
					rows = append(rows, row{Action: a.Action, Resource: a.Resource, When: a.CreatedAt.Format(time.RFC3339)})
				}
				return &Result{Data: map[string]any{"events": rows}}, nil
			},
		))
	}
}
