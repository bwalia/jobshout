package linuxpatch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/secretsrot"
)

// RunOptions drives a plan/patch/verify execution.
type RunOptions struct {
	Mode         string
	Workload     string
	HostsRaw     string
	SSHUser      string
	PreScript    string
	PostScript   string
	Services     []string
	RebootPolicy string
	DryRun       bool
	UseLLM       bool
	Instruction  string
	VaultMount   string
	VaultPath    string
	VaultKeyField string
	VaultCfg     secretsrot.Config // used when VaultPath set
}

// RunOutcome is returned to the service layer.
type RunOutcome struct {
	Phases      []model.LinuxPatchPhase
	HostResults []model.LinuxPatchHostResult
	Plan        model.LinuxPatchPlan
}

// Execute runs the Linux Patch workflow.
func Execute(ctx context.Context, cfg Config, llmClient llm.Client, opt RunOptions, onPhase func(model.LinuxPatchPhase)) (*RunOutcome, error) {
	if opt.Workload == "" {
		opt.Workload = "generic"
	}
	if opt.RebootPolicy == "" {
		opt.RebootPolicy = "if_needed"
	}
	user := opt.SSHUser
	if user == "" {
		user = cfg.SSHUser
	}
	hosts := ParseHosts(opt.HostsRaw, user)
	if len(hosts) == 0 {
		return nil, fmt.Errorf("at least one host is required")
	}
	profile := ProfileFor(opt.Workload)
	services := opt.Services
	if len(services) == 0 {
		services = append(services, profile.DefaultSvcs...)
	}

	out := &RunOutcome{}
	phases := []model.LinuxPatchPhase{
		{Key: "vault", Label: "Fetch SSH from Vault", Status: "pending"},
		{Key: "inventory", Label: "Inventory hosts", Status: "pending"},
		{Key: "plan", Label: "Build rolling plan", Status: "pending"},
		{Key: "execute", Label: "Execute rolling patch", Status: "pending"},
		{Key: "verify", Label: "Verify services", Status: "pending"},
	}
	setPhase := func(key, status, msg string) {
		now := time.Now()
		for i := range phases {
			if phases[i].Key != key {
				continue
			}
			phases[i].Status = status
			phases[i].Message = msg
			if status == "active" && phases[i].StartedAt == nil {
				phases[i].StartedAt = &now
			}
			if status == "completed" || status == "failed" || status == "skipped" {
				phases[i].EndedAt = &now
				if phases[i].StartedAt == nil {
					phases[i].StartedAt = &now
				}
			}
			if onPhase != nil {
				onPhase(phases[i])
			}
			break
		}
		out.Phases = append([]model.LinuxPatchPhase(nil), phases...)
	}

	runCfg := cfg
	if strings.TrimSpace(opt.VaultPath) != "" {
		setPhase("vault", "active", "Reading SSH material via Secrets/Vault client")
		note, err := ApplyVaultSSH(ctx, opt.VaultCfg, &runCfg, VaultSSHOptions{
			Mount: opt.VaultMount, Path: opt.VaultPath, KeyField: opt.VaultKeyField,
		})
		if err != nil {
			setPhase("vault", "failed", err.Error())
			return out, err
		}
		setPhase("vault", "completed", note)
	} else {
		setPhase("vault", "skipped", "No vault_path — using LINUX_PATCH_SSH_* env")
	}
	cfg = runCfg

	// Inventory
	setPhase("inventory", "active", fmt.Sprintf("Probing %d host(s)", len(hosts)))
	var inventory []string
	hostResults := make([]model.LinuxPatchHostResult, 0, len(hosts))
	for _, h := range hosts {
		hr := model.LinuxPatchHostResult{Host: h.Raw, Status: "inventoried"}
		if !cfg.HasAuth() {
			hr.Status = "skipped"
			hr.Messages = []string{"SSH auth not configured — plan-only inventory limited"}
			hostResults = append(hostResults, hr)
			inventory = append(inventory, h.Raw+": no-ssh-auth")
			continue
		}
		client, err := Dial(cfg, h)
		if err != nil {
			hr.Status = "unreachable"
			hr.Error = err.Error()
			hostResults = append(hostResults, hr)
			inventory = append(inventory, h.Raw+": unreachable "+err.Error())
			continue
		}
		osInfo, _ := DetectOS(client)
		hr.OSFlavor = osInfo.Flavor
		pkgs, _, _ := ListPendingUpdates(client, osInfo.Flavor)
		hr.Packages = pkgs
		inventory = append(inventory, fmt.Sprintf("%s flavor=%s pending=%d", h.Raw, osInfo.Flavor, len(pkgs)))
		_ = client.Close()
		hostResults = append(hostResults, hr)
	}
	out.HostResults = hostResults
	setPhase("inventory", "completed", strings.Join(inventory, " · "))

	// Plan
	setPhase("plan", "active", "Heuristic + optional LLM")
	plan := BuildHeuristicPlan(opt.Workload, hosts, opt.RebootPolicy)
	if opt.UseLLM && llmClient != nil {
		plan = EnrichPlanWithLLM(ctx, llmClient, plan, opt.Workload, opt.Instruction, inventory)
	}
	out.Plan = plan
	setPhase("plan", "completed", plan.Strategy)

	if opt.Mode == "plan" {
		setPhase("execute", "skipped", "plan-only")
		setPhase("verify", "skipped", "plan-only")
		return out, nil
	}

	// Reorder hosts by plan if possible
	ordered := orderHosts(hosts, plan.Order)

	if opt.Mode == "verify" {
		setPhase("execute", "skipped", "verify-only")
		setPhase("verify", "active", "Checking services")
		out.HostResults = verifyHosts(cfg, ordered, services, profile)
		setPhase("verify", "completed", "Verify finished")
		return out, nil
	}

	// patch
	setPhase("execute", "active", fmt.Sprintf("Rolling patch dry_run=%v", opt.DryRun))
	results := make([]model.LinuxPatchHostResult, 0, len(ordered))
	for i, h := range ordered {
		if ctx.Err() != nil {
			setPhase("execute", "failed", "cancelled")
			return out, ctx.Err()
		}
		hr := patchOne(ctx, cfg, h, i, len(ordered), opt, profile, services)
		results = append(results, hr)
		out.HostResults = results
		if hr.Status == "failed" && profile.ZeroDowntime {
			setPhase("execute", "failed", "Stopped roll after failure on "+h.Raw+" (HA safety)")
			setPhase("verify", "skipped", "aborted")
			return out, fmt.Errorf("host %s failed: %s", h.Raw, hr.Error)
		}
	}
	setPhase("execute", "completed", fmt.Sprintf("%d host(s) processed", len(results)))
	setPhase("verify", "completed", "Post-roll checks embedded per host")
	return out, nil
}

func orderHosts(hosts []HostTarget, order []string) []HostTarget {
	if len(order) == 0 {
		return hosts
	}
	byRaw := map[string]HostTarget{}
	for _, h := range hosts {
		byRaw[h.Raw] = h
		byRaw[h.Host] = h
	}
	var out []HostTarget
	seen := map[string]bool{}
	for _, o := range order {
		if h, ok := byRaw[o]; ok && !seen[h.Raw] {
			out = append(out, h)
			seen[h.Raw] = true
		}
	}
	for _, h := range hosts {
		if !seen[h.Raw] {
			out = append(out, h)
		}
	}
	return out
}

func patchOne(ctx context.Context, cfg Config, h HostTarget, idx, total int, opt RunOptions, profile WorkloadProfile, services []string) model.LinuxPatchHostResult {
	hr := model.LinuxPatchHostResult{Host: h.Raw, Status: "ok"}
	if !cfg.HasAuth() {
		hr.Status = "failed"
		hr.Error = "SSH auth not configured"
		return hr
	}
	client, err := Dial(cfg, h)
	if err != nil {
		hr.Status = "failed"
		hr.Error = err.Error()
		return hr
	}
	defer client.Close()

	osInfo, _ := DetectOS(client)
	hr.OSFlavor = osInfo.Flavor
	pkgs, _, _ := ListPendingUpdates(client, osInfo.Flavor)
	hr.Packages = pkgs

	runLines := func(lines []string, label string) {
		for _, c := range lines {
			if c == "" {
				continue
			}
			if opt.DryRun {
				hr.Messages = append(hr.Messages, "dry-run "+label+": "+c)
				continue
			}
			out, err := Run(client, c)
			msg := strings.TrimSpace(out)
			if len(msg) > 240 {
				msg = msg[:240] + "…"
			}
			if err != nil {
				hr.Messages = append(hr.Messages, label+" err: "+msg)
			} else if msg != "" {
				hr.Messages = append(hr.Messages, label+": "+msg)
			}
		}
	}

	if profile.PreCommands != nil {
		runLines(profile.PreCommands(h.Raw, idx, total), "pre-workload")
	}
	if strings.TrimSpace(opt.PreScript) != "" {
		runLines([]string{opt.PreScript}, "pre-script")
	}

	if opt.DryRun {
		hr.Messages = append(hr.Messages, fmt.Sprintf("dry-run: would upgrade %d packages on %s", len(pkgs), osInfo.Flavor))
	} else {
		out, err := ApplyUpdates(client, osInfo.Flavor)
		if err != nil {
			hr.Status = "failed"
			hr.Error = "upgrade: " + err.Error()
			hr.Messages = append(hr.Messages, truncate(out, 300))
			return hr
		}
		hr.Messages = append(hr.Messages, "upgrade applied")
	}

	needReboot := NeedsReboot(client)
	doReboot := false
	switch opt.RebootPolicy {
	case "always":
		doReboot = !opt.DryRun
	case "never":
		doReboot = false
	default: // if_needed
		doReboot = needReboot && !opt.DryRun
	}
	if opt.DryRun && (opt.RebootPolicy == "always" || (opt.RebootPolicy != "never" && needReboot)) {
		hr.Messages = append(hr.Messages, "dry-run: would reboot")
	}
	if doReboot {
		hr.Rebooted = true
		_, _ = Run(client, "nohup bash -c 'sleep 1; systemctl reboot' >/dev/null 2>&1 &")
		hr.Messages = append(hr.Messages, "reboot triggered; waiting 45s")
		select {
		case <-ctx.Done():
			hr.Status = "failed"
			hr.Error = "cancelled during reboot wait"
			return hr
		case <-time.After(45 * time.Second):
		}
		// reconnect
		_ = client.Close()
		var err error
		for i := 0; i < 12; i++ {
			client, err = Dial(cfg, h)
			if err == nil {
				break
			}
			time.Sleep(10 * time.Second)
		}
		if err != nil {
			hr.Status = "failed"
			hr.Error = "reconnect after reboot: " + err.Error()
			return hr
		}
		defer client.Close()
	}

	if profile.PostCommands != nil {
		runLines(profile.PostCommands(h.Raw, idx, total), "post-workload")
	}
	if !opt.DryRun {
		ok, fixed, msgs := EnsureServices(client, services)
		hr.ServicesOK, hr.ServicesFix = ok, fixed
		hr.Messages = append(hr.Messages, msgs...)
	} else {
		hr.Messages = append(hr.Messages, "dry-run: would ensure services "+strings.Join(services, ","))
	}
	if strings.TrimSpace(opt.PostScript) != "" {
		runLines([]string{opt.PostScript}, "post-script")
	}
	if profile.HealthCheck != nil && !opt.DryRun {
		out, err := profile.HealthCheck(client)
		hr.Messages = append(hr.Messages, "health: "+truncate(strings.TrimSpace(out), 200))
		if err != nil {
			hr.Messages = append(hr.Messages, "health-err: "+err.Error())
		}
	}
	return hr
}

func verifyHosts(cfg Config, hosts []HostTarget, services []string, profile WorkloadProfile) []model.LinuxPatchHostResult {
	var results []model.LinuxPatchHostResult
	for _, h := range hosts {
		hr := model.LinuxPatchHostResult{Host: h.Raw, Status: "ok"}
		if !cfg.HasAuth() {
			hr.Status = "failed"
			hr.Error = "SSH auth not configured"
			results = append(results, hr)
			continue
		}
		client, err := Dial(cfg, h)
		if err != nil {
			hr.Status = "failed"
			hr.Error = err.Error()
			results = append(results, hr)
			continue
		}
		osInfo, _ := DetectOS(client)
		hr.OSFlavor = osInfo.Flavor
		ok, fixed, msgs := EnsureServices(client, services)
		hr.ServicesOK, hr.ServicesFix, hr.Messages = ok, fixed, msgs
		if profile.HealthCheck != nil {
			out, _ := profile.HealthCheck(client)
			hr.Messages = append(hr.Messages, "health: "+truncate(strings.TrimSpace(out), 200))
		}
		_ = client.Close()
		results = append(results, hr)
	}
	return results
}
