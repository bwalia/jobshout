package linuxpatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner is the launch surface.
type Runner interface {
	CreateRun(ctx context.Context, req model.CreateLinuxPatchRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.LinuxPatchRun, error)
}

// Module is the Linux Patch specialist.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:   model.BuiltinLinuxPatch,
		Label:     "Linux Patch",
		Icon:      "server",
		TabSlug:   "linux-patch",
		Hint:      "SSH rolling patch with LLM planning — Couchbase, HC Vault, and WSLVault HA-aware.",
		ChatHint:  "To patch Linux hosts, call agent_execute on Linux Patch. Pass hosts and workload (wslvault|hashicorp_vault|couchbase|generic). Prefer mode=plan with use_llm first.",
		Schema:    schema(),
		Seed:      Seed,
		Launch:    launch(run),
		StayOnTab: true,
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinLinuxPatch,
		Hint:    "Rolling SSH patching with pre/post scripts, service recovery, and HA-aware workloads.",
		Fields: []agentschema.Field{
			{Key: "mode", Label: "Mode", Type: "select", Required: true, Default: "plan",
				Options: []model.ClarifyOption{
					{Label: "Plan (inventory + LLM)", Value: "plan"},
					{Label: "Patch (rolling)", Value: "patch"},
					{Label: "Verify services", Value: "verify"},
				}},
			{Key: "workload", Label: "Workload profile", Type: "select", Required: false, Default: "generic",
				Options: []model.ClarifyOption{
					{Label: "Generic Linux", Value: "generic"},
					{Label: "Couchbase (rebalance/HA)", Value: "couchbase"},
					{Label: "HashiCorp Vault (seal-aware)", Value: "hashicorp_vault"},
					{Label: "WSLVault (HA multi-region)", Value: "wslvault"},
				},
				Help: "WSLVault docs: https://www.wslvault.org/"},
			{Key: "hosts", Label: "Hosts", Type: "textarea", Required: true,
				Placeholder: "user@host1\nuser@host2:22",
				Help:        "One per line or comma-separated", Question: "Which hosts should I patch?"},
			{Key: "ssh_user", Label: "Default SSH user", Type: "text", Required: false, Default: "root"},
			{Key: "services", Label: "Services to ensure", Type: "text", Required: false,
				Placeholder: "nginx, vault"},
			{Key: "pre_script", Label: "Pre-patch script", Type: "textarea", Required: false,
				Placeholder: "#!/bin/bash\n# runs on each host before upgrade"},
			{Key: "post_script", Label: "Post-patch script", Type: "textarea", Required: false},
			{Key: "reboot_policy", Label: "Reboot policy", Type: "select", Required: false, Default: "if_needed",
				Options: []model.ClarifyOption{
					{Label: "If needed", Value: "if_needed"},
					{Label: "Never", Value: "never"},
					{Label: "Always", Value: "always"},
				}},
			{Key: "use_llm", Label: "LLM planning", Type: "select", Required: false, Default: "true",
				Options: []model.ClarifyOption{
					{Label: "On", Value: "true"},
					{Label: "Off (heuristic only)", Value: "false"},
				}},
			{Key: "dry_run", Label: "Dry run", Type: "select", Required: false, Default: "true",
				Options: []model.ClarifyOption{
					{Label: "Dry run (safe)", Value: "true"},
					{Label: "Live", Value: "false"},
				}},
			{Key: "vault_path", Label: "Vault path for SSH key", Type: "text", Required: false,
				Placeholder: "ops/ssh/patch-fleet",
				Help:        "KV v2 path on WSLVault/HC Vault; chains with Secrets Rotation credentials"},
			{Key: "vault_mount", Label: "Vault mount", Type: "text", Required: false, Default: "secret"},
			{Key: "vault_key_field", Label: "SSH key field name", Type: "text", Required: false,
				Default: "ssh_private_key"},
			{Key: "schedule_after", Label: "Schedule after (RFC3339)", Type: "text", Required: false,
				Placeholder: "2026-09-22T02:00:00Z", Help: "Advisory window noted on the plan"},
			{Key: "instruction", Label: "Note", Type: "textarea", Required: false,
				Placeholder: "e.g. patch london region first; keep manchester serving"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Patch: ", FromKey: "workload", Fallback: "linux"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "mode", Prefix: "Mode: "},
			{Key: "hosts", Prefix: "Hosts: "},
			{Key: "instruction"},
		},
	}
}

// Seed returns the builtin agent.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "SSH rolling Linux patcher with LLM planning. Fetches SSH keys from WSLVault/HashiCorp Vault (Secrets Rotation client). Workload profiles for Couchbase, HC Vault seal safety, and WSLVault HA. Chain via workflow: Secrets Rotation → Linux Patch."
	prompt := "You are the Linux Patch agent. Prefer mode=plan with use_llm before live patch. When SSH keys live in Vault, set vault_path (and optionally vault_mount). Orchestrate with the Secrets Rotation agent via workflows — workflow steps now call specialist Launch for go_native builtins. For wslvault use HA/multi-region failover. For hashicorp_vault warn about seal after reboot. Never print SSH keys or passwords."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameLinuxPatch,
		Role:         "Linux Patch Specialist",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinLinuxPatch},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("Linux Patch is not configured")
		}
		hosts := strings.TrimSpace(in.Values["hosts"])
		if hosts == "" {
			return nil, fmt.Errorf("hosts is required")
		}
		mode := strings.TrimSpace(in.Values["mode"])
		if mode == "" {
			mode = "plan"
		}
		payload := model.CreateLinuxPatchRunRequest{
			AgentID:       in.Agent.ID,
			TaskID:        &in.Task.ID,
			Mode:          mode,
			Workload:      strings.TrimSpace(in.Values["workload"]),
			Hosts:         hosts,
			SSHUser:       strings.TrimSpace(in.Values["ssh_user"]),
			PreScript:     in.Values["pre_script"],
			PostScript:    in.Values["post_script"],
			Services:      strings.TrimSpace(in.Values["services"]),
			RebootPolicy:  strings.TrimSpace(in.Values["reboot_policy"]),
			DryRun:        !strings.EqualFold(strings.TrimSpace(in.Values["dry_run"]), "false"),
			UseLLM:        !strings.EqualFold(strings.TrimSpace(in.Values["use_llm"]), "false"),
			ScheduleAfter: strings.TrimSpace(in.Values["schedule_after"]),
			VaultMount:    strings.TrimSpace(in.Values["vault_mount"]),
			VaultPath:     strings.TrimSpace(in.Values["vault_path"]),
			VaultKeyField: strings.TrimSpace(in.Values["vault_key_field"]),
			Instruction:   strings.TrimSpace(in.Values["instruction"]),
		}
		// default dry_run true when unset
		if strings.TrimSpace(in.Values["dry_run"]) == "" {
			payload.DryRun = true
		}
		created, err := run.CreateRun(ctx, payload, in.OrgID, &in.UserID)
		if err != nil {
			return nil, err
		}
		id := created.ID
		return &agentmodule.LaunchOutput{
			RunID:     &id,
			Message:   "Linux Patch run queued",
			ExtraMeta: map[string]any{model.TaskMetaRunID: id.String()},
		}, nil
	}
}
