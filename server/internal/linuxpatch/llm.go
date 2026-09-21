package linuxpatch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

// EnrichPlanWithLLM asks the configured model to refine order/steps for zero downtime.
func EnrichPlanWithLLM(ctx context.Context, client llm.Client, base model.LinuxPatchPlan, workload, instruction string, inventory []string) model.LinuxPatchPlan {
	if client == nil {
		return base
	}
	sys := `You are a Linux patching SRE assistant for JobShout. Propose a zero-downtime rolling patch plan.
Return ONLY compact JSON: {"order":["host",...],"steps":["..."],"warnings":["..."],"summary":"..."}
Rules: never invent credentials; prefer one host at a time; for wslvault use HA/multi-region failover; for hashicorp_vault warn about seal after reboot; for couchbase mention rebalance/auto-failover.`
	user := fmt.Sprintf("Workload: %s\nBase strategy: %s\nZero downtime capable: %v\nHosts: %s\nInventory:\n%s\nOperator note: %s\nHints: %s",
		workload, base.Strategy, base.ZeroDowntime, strings.Join(base.Order, ", "),
		strings.Join(inventory, "\n"), instruction, strings.Join(base.WorkloadHints, "; "))
	resp, err := client.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: sys},
			{Role: llm.RoleUser, Content: user},
		},
		MaxTokens:   800,
		Temperature: 0.2,
	})
	if err != nil || resp == nil {
		base.Warnings = append(base.Warnings, "LLM enrich skipped: "+errString(err))
		return base
	}
	raw := strings.TrimSpace(resp.Content)
	// extract JSON object if fenced
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}
	var parsed struct {
		Order    []string `json:"order"`
		Steps    []string `json:"steps"`
		Warnings []string `json:"warnings"`
		Summary  string   `json:"summary"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		base.LLMSummary = truncate(resp.Content, 600)
		base.Warnings = append(base.Warnings, "LLM returned non-JSON; kept heuristic plan")
		return base
	}
	if len(parsed.Order) > 0 {
		base.Order = parsed.Order
	}
	if len(parsed.Steps) > 0 {
		base.Steps = parsed.Steps
	}
	if len(parsed.Warnings) > 0 {
		base.Warnings = append(base.Warnings, parsed.Warnings...)
	}
	base.LLMSummary = parsed.Summary
	return base
}

func errString(err error) string {
	if err == nil {
		return "empty response"
	}
	return err.Error()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
