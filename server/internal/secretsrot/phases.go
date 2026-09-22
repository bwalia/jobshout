package secretsrot

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jobshout/server/internal/model"
)

// validatePropagation applies the mode/engine rules on top of ParsePropagation.
func validatePropagation(opt RunOptions) error {
	p := opt.Propagate
	if p.Source == "github" {
		if opt.Mode != "plan" && opt.Mode != "rotate" {
			return fmt.Errorf("source=github supports plan and rotate only (GitHub secrets cannot be read back for verify/rollback)")
		}
		if len(opt.Keys) == 0 {
			return fmt.Errorf("source=github requires keys (the secret names to generate)")
		}
		if p.GitHubRepo == "" {
			return fmt.Errorf("source=github requires github_repo")
		}
		return nil
	}
	if p.HasTargets() && opt.Engine != "kv2" {
		return fmt.Errorf("propagation targets require engine=kv2")
	}
	return nil
}

// buildPhases is defaultPhases plus the configured propagation phases. They
// run after the Vault write + verify and before retire_old, so an old version
// is only retired once every consumer has the new one.
func buildPhases(opt RunOptions) []model.SecretsRotationPhase {
	p := opt.Propagate
	mk := func(key, label string) model.SecretsRotationPhase {
		return model.SecretsRotationPhase{Key: key, Label: label, Status: "pending"}
	}
	if p.Source == "github" {
		if opt.Mode == "plan" {
			return []model.SecretsRotationPhase{mk("preflight", "Preflight"), mk("plan", "Build plan")}
		}
		out := []model.SecretsRotationPhase{mk("preflight", "Preflight"), mk("generate", "Generate new values")}
		out = append(out, p.phases()...)
		return append(out, mk("audit", "Audit"))
	}
	base := defaultPhases(opt.Mode, opt.Engine)
	if !p.HasTargets() || opt.Mode == "plan" {
		return base
	}
	extra := p.phases()
	for i, ph := range base {
		if ph.Key == "retire_old" {
			out := append([]model.SecretsRotationPhase{}, base[:i]...)
			out = append(out, extra...)
			return append(out, base[i:]...)
		}
	}
	return append(base, extra...)
}

// skipPending marks every still-pending phase skipped with reason.
func skipPending(out *RunOutcome, setPhase func(string, string, string), reason string) {
	for _, ph := range append([]model.SecretsRotationPhase(nil), out.Phases...) {
		if ph.Status == "pending" {
			setPhase(ph.Key, "skipped", reason)
		}
	}
}

// keysFor returns the requested keys, or every key of data (sorted).
func keysFor(requested []string, data map[string]any) []string {
	if len(requested) > 0 {
		return requested
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// keyOnlyValues is a placeholder map for dry runs: key names, no values.
func keyOnlyValues(keys []string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = ""
	}
	return out
}

func runPlanGitHub(opt RunOptions, out *RunOutcome, setPhase func(string, string, string)) (*RunOutcome, error) {
	steps := []string{
		fmt.Sprintf("Generate new values for %s in memory (Vault is not read or written)", strings.Join(opt.Keys, ",")),
	}
	steps = append(steps, opt.Propagate.PlanSteps(opt.Keys)...)
	out.Result.PlanSteps = steps
	out.Result.KeysRotated = opt.Keys
	setPhase("plan", "completed", fmt.Sprintf("%d steps · source github", len(steps)))
	return out, nil
}

func rotateGitHub(ctx context.Context, opt RunOptions, out *RunOutcome, setPhase func(string, string, string), pr *propagator) (*RunOutcome, error) {
	setPhase("generate", "active", "Generating new values in memory")
	values := make(map[string]string, len(opt.Keys))
	for _, k := range opt.Keys {
		if v, ok := opt.NewSecret[k]; ok {
			values[k] = stringValues(map[string]any{k: v}, []string{k})[k]
			continue
		}
		values[k] = generateSecret()
	}
	out.Result.KeysRotated = opt.Keys
	if opt.DryRun {
		setPhase("generate", "completed", "Dry-run: would generate "+strings.Join(opt.Keys, ","))
		values = keyOnlyValues(opt.Keys)
	} else {
		setPhase("generate", "completed", fmt.Sprintf("Generated %d value(s) for %s (memory only)", len(values), strings.Join(opt.Keys, ",")))
	}
	if err := pr.run(ctx, values, 0, true); err != nil {
		skipPending(out, setPhase, "Not run: propagation failed")
		return out, err
	}
	if opt.DryRun {
		setPhase("audit", "completed", "Dry-run complete")
	} else {
		setPhase("audit", "completed", "Rotation recorded without plaintext secret values")
	}
	return out, nil
}

// ValidateLaunch checks the mode/engine/keys rules for a propagation before a
// run is queued, so bad combinations fail the launch instead of the run.
func ValidateLaunch(mode, engine string, keys []string, p Propagation) error {
	if engine == "" {
		engine = "kv2"
	}
	return validatePropagation(RunOptions{Mode: mode, Engine: engine, Keys: keys, Propagate: p})
}
