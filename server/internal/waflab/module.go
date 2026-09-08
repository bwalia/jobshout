package waflab

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner is the launch surface. The waf-lab service satisfies it.
type Runner interface {
	CreateRun(ctx context.Context, req model.CreateWAFLabRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.WAFLabRun, error)
}

// Module is the WAF Efficacy Lab specialist.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:   model.BuiltinWAFLab,
		Label:     "WAF Efficacy Lab",
		Icon:      "shield",
		TabSlug:   "waf-lab",
		Hint:      "Provision a before/after WAF pair on wslproxy and measure the attack matrix.",
		ChatHint:  "To run a WAF efficacy lab, call agent_execute on the WAF Efficacy Lab. Do not invent hosts.",
		Schema:    schema(),
		Seed:      Seed,
		Launch:    launch(run),
		StayOnTab: true,
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["secure_host"] != "" {
				return
			}
			if host := extractHost(prompt); host != "" {
				vals["secure_host"] = host
			}
		},
		Requirements: []agentmodule.Requirement{{
			Key: "wslproxy", Kind: "config",
			Message: "WSLPROXY_BASE_URL and WSLPROXY_API_TOKEN (or username/password) must be configured.",
		}},
		Ready: func(context.Context, uuid.UUID) []agentmodule.Issue {
			if run == nil {
				return []agentmodule.Issue{{
					Severity: "warning", Code: "wslproxy_unconfigured",
					Message: "WAF Efficacy Lab is not configured on this environment.",
				}}
			}
			type enabler interface{ Enabled() bool }
			if e, ok := run.(enabler); ok && !e.Enabled() {
				return []agentmodule.Issue{{
					Severity: "warning", Code: "wslproxy_unconfigured",
					Message: "WAF Efficacy Lab is not configured (set WSLPROXY_BASE_URL and credentials).",
				}}
			}
			return nil
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinWAFLab,
		Hint:    "Provision and/or test a WAF before/after pair via wslproxy.",
		Fields: []agentschema.Field{
			{Key: "wslproxy_base_url", Label: "wslproxy base URL", Type: "text", Required: true,
				Default: "https://lon1.pop0.uk", Placeholder: "https://lon1.pop0.uk",
				Help: "Must be https", Question: "What is the wslproxy base URL?"},
			{Key: "secure_host", Label: "Secure host (WAF on)", Type: "text", Required: true,
				Placeholder: "payments-secure.fictionally.org",
				Question:    "What is the secure (WAF-on) hostname?"},
			{Key: "open_host", Label: "Open host (WAF off)", Type: "text", Required: true,
				Placeholder: "payments-open.fictionally.org",
				Question:    "What is the open (WAF-off) hostname?"},
			{Key: "origin_upstream", Label: "Origin upstream", Type: "text", Required: false,
				Default: "127.0.0.1:30084", Placeholder: "127.0.0.1:30084"},
			{Key: "policy_id", Label: "WAF policy id", Type: "text", Required: false,
				Default: "waf-policy-payments-hard"},
			{Key: "mode", Label: "Mode", Type: "select", Required: true, Default: "provision_and_test",
				Options: []model.ClarifyOption{
					{Label: "Provision and test", Value: "provision_and_test"},
					{Label: "Provision only", Value: "provision_only"},
					{Label: "Test only", Value: "test_only"},
				}},
			{Key: "manage_dns", Label: "Manage DNS", Type: "select", Required: false, Default: "off",
				Options: []model.ClarifyOption{
					{Label: "Off (manual DNS)", Value: "off"},
					{Label: "Cloudflare via wslproxy", Value: "cloudflare"},
				}},
			{Key: "dns_zone", Label: "DNS zone", Type: "text", Required: false,
				Placeholder: "fictionally.org", Help: "Only when manage_dns=cloudflare"},
			{Key: "attack_set", Label: "Attack set", Type: "select", Required: false, Default: "full",
				Options: []model.ClarifyOption{
					{Label: "Full matrix", Value: "full"},
					{Label: "OWASP core", Value: "owasp_core"},
					{Label: "Modern / API", Value: "modern_api"},
					{Label: "v2 stages only", Value: "stages_only"},
				}},
			{Key: "instruction", Label: "Note (optional)", Type: "textarea",
				Placeholder: "Carried into the report"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "WAF lab: ", FromKey: "secure_host", Fallback: "lab"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "mode", Prefix: "Mode: "},
			{Key: "policy_id", Prefix: "Policy: "},
			{Key: "attack_set", Prefix: "Attack set: "},
			{Key: "instruction"},
		},
	}
}

// Seed is the built-in WAF Efficacy Lab agent.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Provisions a before/after WAF pair on wslproxy (secure vs open) and measures an honest attack matrix — including BOLA gaps and false positives."
	prompt := "You are the WAF Efficacy Lab agent. Drive wslproxy to seed rules, bind policies to vhosts, optionally provision DNS, then fire the attack catalogue through POST /api/waf/test. Report blocked/leaked/false-positive scores honestly. Never claim BOLA is blocked by a signature WAF."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameWAFLab,
		Role:         "WAF Efficacy Lab Agent",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinWAFLab},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("WAF Efficacy Lab is not configured")
		}
		base := strings.TrimSpace(in.Values["wslproxy_base_url"])
		if base == "" {
			base = "https://lon1.pop0.uk"
		}
		if err := validateHTTPS(base); err != nil {
			return nil, err
		}
		payload := model.CreateWAFLabRunRequest{
			AgentID:         in.Agent.ID,
			TaskID:          &in.Task.ID,
			WSLProxyBaseURL: base,
			SecureHost:      strings.TrimSpace(in.Values["secure_host"]),
			OpenHost:        strings.TrimSpace(in.Values["open_host"]),
			OriginUpstream:  strings.TrimSpace(in.Values["origin_upstream"]),
			PolicyID:        strings.TrimSpace(in.Values["policy_id"]),
			Mode:            strings.TrimSpace(in.Values["mode"]),
			ManageDNS:       strings.TrimSpace(in.Values["manage_dns"]),
			DNSZone:         strings.TrimSpace(in.Values["dns_zone"]),
			AttackSet:       strings.TrimSpace(in.Values["attack_set"]),
			Instruction:     strings.TrimSpace(in.Values["instruction"]),
		}
		if payload.Mode == "" {
			payload.Mode = "provision_and_test"
		}
		if payload.ManageDNS == "" {
			payload.ManageDNS = "off"
		}
		if payload.AttackSet == "" {
			payload.AttackSet = "full"
		}
		if payload.OriginUpstream == "" {
			payload.OriginUpstream = "127.0.0.1:30084"
		}
		if payload.PolicyID == "" {
			payload.PolicyID = "waf-policy-payments-hard"
		}
		created, err := run.CreateRun(ctx, payload, in.OrgID, &in.UserID)
		if err != nil {
			return nil, err
		}
		id := created.ID
		return &agentmodule.LaunchOutput{
			RunID:     &id,
			Message:   "WAF Efficacy Lab run queued",
			ExtraMeta: map[string]any{model.TaskMetaRunID: id.String()},
		}, nil
	}
}

func validateHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("wslproxy_base_url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("wslproxy_base_url must be https")
	}
	if u.Host == "" {
		return fmt.Errorf("wslproxy_base_url missing host")
	}
	return nil
}

// extractHost picks a hostname-looking token from free text for AbsorbPrompt.
func extractHost(prompt string) string {
	fields := strings.Fields(prompt)
	for _, f := range fields {
		f = strings.Trim(f, ".,;:\"'")
		f = strings.TrimPrefix(f, "https://")
		f = strings.TrimPrefix(f, "http://")
		f = strings.TrimRight(f, "/")
		if strings.Contains(f, ".") && !strings.Contains(f, " ") && len(f) > 3 {
			// Prefer payments-secure style hosts.
			if strings.Contains(f, "secure") || strings.Contains(f, "payments") {
				return f
			}
		}
	}
	for _, f := range fields {
		f = strings.Trim(f, ".,;:\"'")
		f = strings.TrimPrefix(f, "https://")
		f = strings.TrimPrefix(f, "http://")
		f = strings.TrimRight(f, "/")
		if strings.Count(f, ".") >= 1 && !strings.ContainsAny(f, "/?#") {
			return f
		}
	}
	return ""
}

// CloudflareConfigured reports whether a CF token is available for the DNS phase.
func CloudflareConfigured() bool {
	return strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")) != ""
}
