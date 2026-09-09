package waflab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/model"
)

// API is the subset of Client used by lab phases (mockable in tests).
type API interface {
	Login(ctx context.Context) error
	WAFTestTargets(ctx context.Context) ([]string, error)
	ListWAFRules(ctx context.Context) ([]map[string]any, error)
	ListWAFPolicies(ctx context.Context) ([]map[string]any, error)
	ListServers(ctx context.Context) ([]map[string]any, error)
	SeedWAFRules(ctx context.Context, profileID string) error
	UpsertWAFRules(ctx context.Context, rules []map[string]any) error
	UpsertWAFPolicies(ctx context.Context, policies []map[string]any) error
	UpsertRules(ctx context.Context, rules []map[string]any) error
	UpsertServers(ctx context.Context, servers []map[string]any) error
	DNSProvision(ctx context.Context, serverID, profileID string) (map[string]any, error)
	DNSLookup(ctx context.Context, domain string) (map[string]any, error)
	WAFTest(ctx context.Context, req WAFTestRequest) (*WAFTestResponse, error)
	ListWAFEvents(ctx context.Context) ([]map[string]any, error)
	Profile() string
}

// LabConfig comes from launch schema fields.
type LabConfig struct {
	SecureHost     string
	OpenHost       string
	OriginUpstream string
	PolicyID       string
	Mode           string // provision_and_test | provision_only | test_only
	ManageDNS      string // off | cloudflare
	DNSZone        string
	AttackSet      string
	Instruction    string
	ProfileID      string
	CloudflareOK   bool // true when CLOUDFLARE_API_TOKEN (or equivalent) is present
}

// StepRecorder receives phase progress.
type StepRecorder interface {
	RecordStep(phase, status, message string)
}

// LabResult is the output of RunLab.
type LabResult struct {
	Steps   []StepOutcome
	Results []HostResult
	Score   model.WAFLabScore
	Skipped []string
}

// StepOutcome is one finished/skipped phase.
type StepOutcome struct {
	Phase   string
	Status  string
	Message string
}

// RunLab executes the five phases according to mode.
func RunLab(ctx context.Context, api API, cfg LabConfig, rec StepRecorder) (*LabResult, error) {
	cfg = normalizeLabConfig(cfg)
	out := &LabResult{}
	record := func(phase, status, message string) {
		out.Steps = append(out.Steps, StepOutcome{Phase: phase, Status: status, Message: message})
		if rec != nil {
			rec.RecordStep(phase, status, message)
		}
	}

	doProvision := cfg.Mode == "provision_and_test" || cfg.Mode == "provision_only"
	doTest := cfg.Mode == "provision_and_test" || cfg.Mode == "test_only"

	// 1. Preflight
	if err := api.Login(ctx); err != nil {
		record("preflight", "failed", err.Error())
		return out, err
	}
	targets, err := api.WAFTestTargets(ctx)
	if err != nil {
		record("preflight", "failed", err.Error())
		return out, err
	}
	rules, _ := api.ListWAFRules(ctx)
	policies, _ := api.ListWAFPolicies(ctx)
	servers, _ := api.ListServers(ctx)
	record("preflight", "completed", fmt.Sprintf(
		"targets=%d rules=%d policies=%d servers=%d",
		len(targets), len(rules), len(policies), len(servers)))

	if doProvision {
		// 2. Rules + policy
		//
		// Seeding asks wslproxy to write its built-in OWASP rules if they are
		// not already on disk, so it does nothing on a deployment that has
		// been seeded once. Treat it as best-effort: some wslproxy builds
		// return 500 from /api/waf_rules/seed for every payload, and aborting
		// there stranded labs whose rules were all present already. What
		// actually matters is verified after the imports — that every rule the
		// policy names exists.
		seedNote := ""
		if err := api.SeedWAFRules(ctx, cfg.ProfileID); err != nil {
			seedNote = "; seed unavailable (" + err.Error() + ")"
		}
		modern, err := loadDemoMaps("waf_rules")
		if err != nil {
			record("rules_policy", "failed", "modern rules: "+err.Error())
			return out, err
		}
		if err := api.UpsertWAFRules(ctx, modern); err != nil {
			record("rules_policy", "failed", err.Error())
			return out, err
		}
		pols, err := loadDemoMaps("waf_policies")
		if err != nil {
			record("rules_policy", "failed", "policy: "+err.Error())
			return out, err
		}
		for _, p := range pols {
			if cfg.PolicyID != "" {
				p["id"] = cfg.PolicyID
			}
			p["profile_id"] = cfg.ProfileID
		}
		if err := api.UpsertWAFPolicies(ctx, pols); err != nil {
			record("rules_policy", "failed", err.Error())
			return out, err
		}
		// A policy naming rules wslproxy does not have would grade a WAF that
		// is quietly weaker than the report claims, so refuse to run rather
		// than publish a misleading score.
		present, err := api.ListWAFRules(ctx)
		if err != nil {
			record("rules_policy", "failed", "rule coverage check: "+err.Error())
			return out, err
		}
		if missing := missingPolicyRules(pols, present); len(missing) > 0 {
			msg := fmt.Sprintf("wslproxy is missing %d rule(s) the policy references: %s%s",
				len(missing), strings.Join(missing, ", "), seedNote)
			record("rules_policy", "failed", msg)
			return out, errors.New(msg)
		}
		record("rules_policy", "completed", fmt.Sprintf("imported %d modern rules, policy %s, all referenced rules present%s", len(modern), cfg.PolicyID, seedNote))

		// 3. Hosts
		route, secure, open := buildHostPayloads(cfg)
		if err := assertSecureOpenDifferOnlyOnWAF(secure, open); err != nil {
			record("hosts", "failed", err.Error())
			return out, err
		}
		if err := api.UpsertRules(ctx, []map[string]any{route}); err != nil {
			record("hosts", "failed", "routing rule: "+err.Error())
			return out, err
		}
		if err := api.UpsertServers(ctx, []map[string]any{secure, open}); err != nil {
			record("hosts", "failed", "servers: "+err.Error())
			return out, err
		}
		record("hosts", "completed", fmt.Sprintf("secure=%s open=%s origin=%s", cfg.SecureHost, cfg.OpenHost, cfg.OriginUpstream))

		// 4. DNS
		if cfg.ManageDNS != "cloudflare" || !cfg.CloudflareOK {
			msg := "DNS skipped (manage_dns=off or no Cloudflare token). Create CNAMEs manually: " +
				cfg.SecureHost + " and " + cfg.OpenHost
			record("dns", "skipped", msg)
			out.Skipped = append(out.Skipped, "dns")
		} else {
			for _, sid := range []string{
				"host:" + cfg.SecureHost,
				"host:" + cfg.OpenHost,
			} {
				if _, err := api.DNSProvision(ctx, sid, cfg.ProfileID); err != nil {
					record("dns", "failed", err.Error())
					return out, err
				}
			}
			record("dns", "completed", "provisioned CNAMEs via wslproxy dns/provision")
		}
	} else {
		record("rules_policy", "skipped", "mode=test_only")
		record("hosts", "skipped", "mode=test_only")
		record("dns", "skipped", "mode=test_only")
		out.Skipped = append(out.Skipped, "rules_policy", "hosts", "dns")
	}

	if !doTest {
		record("efficacy", "skipped", "mode=provision_only")
		out.Skipped = append(out.Skipped, "efficacy")
		return out, nil
	}

	// 5. Efficacy
	attacks := FilterBySet(cfg.AttackSet)
	var results []HostResult
	hosts := []struct {
		role string
		host string
	}{
		{"secure", cfg.SecureHost},
		{"open", cfg.OpenHost},
	}
	for _, a := range attacks {
		for _, h := range hosts {
			target := h.host
			if !strings.Contains(target, "://") {
				target = "https://" + target
			}
			resp, err := api.WAFTest(ctx, WAFTestRequest{
				Target:      target,
				Method:      a.Method,
				Path:        a.Path,
				Headers:     a.Headers,
				Body:        a.Body,
				ContentType: a.ContentType,
			})
			hr := HostResult{
				AttackID:    a.ID,
				AttackName:  a.Name,
				Category:    a.Category,
				HostRole:    h.role,
				Host:        h.host,
				Method:      a.Method,
				Path:        a.Path,
				Payload:     a.Body,
				ExpectBlock: a.ExpectBlock,
				Notes:       a.Notes,
			}
			if err != nil {
				hr.Notes = err.Error()
			} else if resp != nil {
				hr.Blocked = resp.Blocked
				hr.StatusCode = resp.Status
				hr.WAFRule = resp.WAFRule
				hr.WAFViolation = resp.WAFViolation
				hr.SupportID = resp.SupportID
				hr.LatencyMS = resp.LatencyMS
			}
			results = append(results, hr)
		}
	}
	out.Results = results
	out.Score = ScoreMatrix(results)
	_, _ = api.ListWAFEvents(ctx) // best-effort cross-check
	record("efficacy", "completed", fmt.Sprintf(
		"attacks=%d secure_blocked=%d/%d open_leaked=%d fp=%d suspicious=%v",
		out.Score.AttacksTotal, out.Score.SecureBlocked, out.Score.SecureExpected,
		out.Score.OpenLeaked, out.Score.FalsePositives, out.Score.Suspicious))
	return out, nil
}

func normalizeLabConfig(cfg LabConfig) LabConfig {
	if cfg.OriginUpstream == "" {
		cfg.OriginUpstream = DefaultOriginUpstream()
	}
	if cfg.PolicyID == "" {
		cfg.PolicyID = "waf-policy-payments-hard"
	}
	if cfg.Mode == "" {
		cfg.Mode = "provision_and_test"
	}
	if cfg.ManageDNS == "" {
		cfg.ManageDNS = "off"
	}
	if cfg.AttackSet == "" {
		cfg.AttackSet = "full"
	}
	if cfg.ProfileID == "" {
		cfg.ProfileID = "prod"
	}
	cfg.SecureHost = stripScheme(cfg.SecureHost)
	cfg.OpenHost = stripScheme(cfg.OpenHost)
	return cfg
}

func stripScheme(h string) string {
	h = strings.TrimSpace(h)
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	return strings.TrimRight(h, "/")
}

func buildHostPayloads(cfg LabConfig) (route, secure, open map[string]any) {
	ruleID := "payments-demo-default"
	route = map[string]any{
		"id": ruleID, "name": ruleID, "profile_id": cfg.ProfileID, "priority": 1,
		"rules_tags": []string{"wslproxy-waf-demo", "payments"},
		"match": map[string]any{
			"response": map[string]any{
				"strip_path": false, "auto_redirect_https": false, "code": 305, "allow": true,
				"routing":      map[string]any{"mode": "least_conn"},
				"is_consul":    false,
				"redirect_uri": cfg.OriginUpstream,
				"backends": []map[string]any{
					{"address": cfg.OriginUpstream, "weight": 100, "label": "payments-origin"},
				},
			},
			"rules": map[string]any{"path_key": "starts_with", "path": "/", "jwt_token_validation": "equals"},
		},
		"servers": []string{"host:" + cfg.SecureHost, "host:" + cfg.OpenHost},
	}
	base := func(host string) map[string]any {
		return map[string]any{
			"id": "host:" + host, "server_name": host, "proxy_server_name": host,
			"root": "/var/www/html", "index": "index.html",
			"access_log":    "logs/" + host + ".access.log",
			"error_log":     "logs/" + host + ".error.log",
			"config_status": false, "profile_id": cfg.ProfileID,
			"rules":       ruleID,
			"listens":     []map[string]any{{"listen": "80"}},
			"ssl_enabled": true, "ssl_force_https": true, "ssl_staging": false,
			"ssl_auto_renew": true, "cache_enabled": false,
			"match_cases": map[string]any{}, "custom_headers": []any{},
		}
	}
	secure = base(cfg.SecureHost)
	secure["waf_enabled"] = true
	secure["waf_policy_id"] = cfg.PolicyID
	secure["waf_mode_override"] = "block"
	open = base(cfg.OpenHost)
	open["waf_enabled"] = false
	return route, secure, open
}

// assertSecureOpenDifferOnlyOnWAF fails if non-WAF fields diverge.
func assertSecureOpenDifferOnlyOnWAF(secure, open map[string]any) error {
	wafKeys := map[string]bool{
		"waf_enabled": true, "waf_policy_id": true, "waf_mode_override": true,
		"id": true, "server_name": true, "proxy_server_name": true,
		"access_log": true, "error_log": true,
	}
	for k, sv := range secure {
		if wafKeys[k] {
			continue
		}
		ov, ok := open[k]
		if !ok {
			return fmt.Errorf("open host missing field %q (secure/open must differ only on waf fields)", k)
		}
		sb, _ := json.Marshal(sv)
		ob, _ := json.Marshal(ov)
		if string(sb) != string(ob) {
			return fmt.Errorf("secure/open differ on non-waf field %q: secure=%s open=%s", k, sb, ob)
		}
	}
	for k := range open {
		if wafKeys[k] {
			continue
		}
		if _, ok := secure[k]; !ok {
			return fmt.Errorf("secure host missing field %q", k)
		}
	}
	if secure["waf_enabled"] != true {
		return fmt.Errorf("secure host must have waf_enabled=true")
	}
	if open["waf_enabled"] != false {
		return fmt.Errorf("open host must have waf_enabled=false")
	}
	return nil
}

// missingPolicyRules reports which rule IDs the given policies reference that
// are absent from present. Returns nil when every reference resolves.
func missingPolicyRules(policies, present []map[string]any) []string {
	var want []string
	for _, p := range policies {
		refs, ok := p["waf_rules"].([]any)
		if !ok {
			continue
		}
		for _, r := range refs {
			if id, ok := r.(string); ok && id != "" {
				want = append(want, id)
			}
		}
	}
	if len(want) == 0 {
		return nil
	}
	have := make(map[string]bool, len(present))
	for _, r := range present {
		if id, ok := r["id"].(string); ok {
			have[id] = true
		}
	}
	var missing []string
	seen := map[string]bool{}
	for _, id := range want {
		if !have[id] && !seen[id] {
			seen[id] = true
			missing = append(missing, id)
		}
	}
	return missing
}
