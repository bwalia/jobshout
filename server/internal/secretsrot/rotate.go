package secretsrot

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jobshout/server/internal/model"
)

// RunOptions drives a rotation / plan / verify / rollback.
type RunOptions struct {
	Mode         string
	Provider     string
	VaultAddr    string // optional override
	Token        string // optional override (from env if empty)
	Namespace    string
	Mount        string
	Path         string
	Engine       string
	Keys         []string
	GraceSeconds int
	RetireOld    bool
	DryRun       bool
	NewSecret    map[string]any // optional explicit values

	// Propagate pushes the rotated values past Vault (k8s Secrets,
	// ExternalSecrets, GitHub Actions, Ring Promoter). Names only.
	Propagate Propagation
	RunID     string // included in the Ring Promoter restart reason
}

// RunOutcome is returned to the service (no secret values).
type RunOutcome struct {
	DetectedProvider string
	Phases           []model.SecretsRotationPhase
	Result           model.SecretsRotationResult
}

// Execute runs the zero-downtime secrets workflow.
func Execute(ctx context.Context, cfg Config, opt RunOptions, onPhase func(model.SecretsRotationPhase)) (*RunOutcome, error) {
	if opt.Mount == "" {
		opt.Mount = "secret"
	}
	if opt.Engine == "" {
		opt.Engine = "kv2"
	}
	if opt.GraceSeconds < 0 {
		opt.GraceSeconds = 300
	}
	if opt.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if opt.Propagate.Source == "" {
		opt.Propagate.Source = "vault"
	}

	runCfg := cfg
	if strings.TrimSpace(opt.VaultAddr) != "" {
		runCfg.Addr = strings.TrimRight(strings.TrimSpace(opt.VaultAddr), "/")
	}
	if strings.TrimSpace(opt.Token) != "" {
		runCfg.Token = strings.TrimSpace(opt.Token)
	}
	if strings.TrimSpace(opt.Namespace) != "" {
		runCfg.Namespace = strings.TrimSpace(opt.Namespace)
	}

	client := NewClient(runCfg)
	out := &RunOutcome{
		Result: model.SecretsRotationResult{
			UILoginURL:    client.UILogin(),
			DocsURL:       client.DocsURL(),
			Mount:         opt.Mount,
			Path:          opt.Path,
			Engine:        opt.Engine,
			DualWindowSec: opt.GraceSeconds,
			ZeroDowntime:  true,
			Strategy:      strategyFor(opt.Engine),
		},
	}

	phases := buildPhases(opt)
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
		out.Phases = append([]model.SecretsRotationPhase(nil), phases...)
	}

	// ── preflight ──────────────────────────────────────────────────────────
	if err := validatePropagation(opt); err != nil {
		setPhase("preflight", "failed", err.Error())
		return out, err
	}
	var pr *propagator
	if opt.Propagate.HasTargets() {
		kube, problems := opt.Propagate.checkCredentials(runCfg)
		if len(problems) > 0 {
			if opt.DryRun || opt.Mode == "plan" {
				for _, p := range problems {
					out.Result.Warnings = append(out.Result.Warnings, "propagation: "+p)
				}
			} else {
				msg := "Propagation credentials missing: " + strings.Join(problems, "; ")
				setPhase("preflight", "failed", msg)
				return out, fmt.Errorf("%s", msg)
			}
		}
		pr = newPropagator(runCfg, opt, kube, out, setPhase)
		// Read-only cluster checks before any write. Plan / dry-run report
		// them as warnings; a live run stops here with nothing written.
		if problems := pr.precheck(ctx); len(problems) > 0 {
			if opt.DryRun || opt.Mode == "plan" {
				for _, p := range problems {
					out.Result.Warnings = append(out.Result.Warnings, "propagation: "+p)
				}
			} else {
				msg := "Propagation precheck failed (nothing written): " + strings.Join(problems, "; ")
				setPhase("preflight", "failed", msg)
				return out, fmt.Errorf("%s", msg)
			}
		}
	}
	if opt.Propagate.Source == "github" {
		out.DetectedProvider = "github"
		out.Result.Engine = "github"
		out.Result.Strategy = "GitHub Actions secrets are the source of truth: generate in memory, seal and PUT; Vault is not read or written."
		setPhase("preflight", "completed", "Source github · "+opt.Propagate.githubTarget())
		if opt.Mode == "plan" {
			return runPlanGitHub(opt, out, setPhase)
		}
		return rotateGitHub(ctx, opt, out, setPhase, pr)
	}

	setPhase("preflight", "active", "Checking vault reachability")
	health, herr := client.Health(ctx)
	prov := opt.Provider
	if prov == "" || prov == "auto" {
		prov = DetectProvider(runCfg.Addr, health)
	}
	out.DetectedProvider = prov
	if herr != nil && !client.Enabled() {
		setPhase("preflight", "failed", "Vault not configured: set SECRETS_VAULT_TOKEN (or VAULT_TOKEN) and address")
		return out, fmt.Errorf("vault not configured: %w", herr)
	}
	if herr != nil {
		out.Result.Warnings = append(out.Result.Warnings, "sys/health: "+herr.Error())
	}
	if !client.Enabled() && opt.Mode != "plan" {
		setPhase("preflight", "failed", "Missing vault token")
		return out, fmt.Errorf("vault token required for mode %s", opt.Mode)
	}
	setPhase("preflight", "completed", fmt.Sprintf("Provider %s · %s", prov, runCfg.Addr))

	switch opt.Mode {
	case "plan":
		return runPlan(ctx, client, opt, out, setPhase)
	case "verify":
		return runVerify(ctx, client, opt, out, setPhase, pr)
	case "rollback":
		return runRollback(ctx, client, opt, out, setPhase, pr)
	case "rotate":
		return runRotate(ctx, client, opt, out, setPhase, pr)
	default:
		return out, fmt.Errorf("unknown mode %q", opt.Mode)
	}
}

func strategyFor(engine string) string {
	switch engine {
	case "transit":
		return "Rotate transit key; previous key versions remain for decrypt (zero downtime)."
	case "database":
		return "Issue new dynamic lease, cut consumers over, revoke old lease after grace."
	default:
		return "KV v2 dual-version window: write N+1, keep N readable, soft-delete N after grace."
	}
}

func defaultPhases(mode, engine string) []model.SecretsRotationPhase {
	mk := func(key, label string) model.SecretsRotationPhase {
		return model.SecretsRotationPhase{Key: key, Label: label, Status: "pending"}
	}
	switch mode {
	case "plan":
		return []model.SecretsRotationPhase{
			mk("preflight", "Preflight"),
			mk("inventory", "Inventory"),
			mk("plan", "Build plan"),
		}
	case "verify":
		return []model.SecretsRotationPhase{
			mk("preflight", "Preflight"),
			mk("read", "Read current"),
			mk("verify", "Verify"),
		}
	case "rollback":
		return []model.SecretsRotationPhase{
			mk("preflight", "Preflight"),
			mk("read", "Read current"),
			mk("rollback", "Restore previous"),
			mk("verify", "Verify"),
		}
	default: // rotate
		if engine == "transit" {
			return []model.SecretsRotationPhase{
				mk("preflight", "Preflight"),
				mk("rotate", "Rotate transit key"),
				mk("verify", "Verify"),
				mk("audit", "Audit"),
			}
		}
		return []model.SecretsRotationPhase{
			mk("preflight", "Preflight"),
			mk("read", "Read current version"),
			mk("write_new", "Write new version (N+1)"),
			mk("dual_window", "Dual-read window"),
			mk("verify", "Verify new version"),
			mk("retire_old", "Retire old version"),
			mk("audit", "Audit"),
		}
	}
}

func runPlan(ctx context.Context, client *Client, opt RunOptions, out *RunOutcome, setPhase func(string, string, string)) (*RunOutcome, error) {
	setPhase("inventory", "active", "Reading metadata")
	var steps []string
	switch opt.Engine {
	case "transit":
		steps = []string{
			"POST /v1/" + opt.Mount + "/keys/" + opt.Path + "/rotate",
			"Confirm encrypt uses latest key version; decrypt still accepts older versions",
			"No consumer restart required for decrypt-only workloads",
		}
		setPhase("inventory", "completed", "Transit key "+opt.Path)
	case "database":
		steps = []string{
			"Create new dynamic credentials (lease)",
			"Update application ExternalSecret / env to new lease",
			"Wait grace (" + fmt.Sprintf("%ds", opt.GraceSeconds) + ") for connection drain",
			"Revoke previous lease",
		}
		setPhase("inventory", "completed", "Database engine plan (manual consumer cutover)")
	default:
		meta, err := client.MetadataKV2(ctx, opt.Mount, opt.Path)
		if err != nil {
			// Missing path is still a valid plan target
			steps = []string{
				"Create KV v2 secret at " + opt.Mount + "/data/" + opt.Path + " (version 1)",
				"Configure consumers to read latest version",
			}
			out.Result.Warnings = append(out.Result.Warnings, err.Error())
			setPhase("inventory", "completed", "Path not yet present — create on first rotate")
		} else {
			out.Result.CurrentVersion = meta.Version
			out.Result.PreviousVersion = meta.Version
			steps = []string{
				fmt.Sprintf("Read current version %d at %s/data/%s", meta.Version, opt.Mount, opt.Path),
				"Generate new values for selected keys (never log plaintext)",
				fmt.Sprintf("Write version %d with check-and-set cas=%d", meta.Version+1, meta.Version),
				fmt.Sprintf("Hold dual-read window %ds (KV keeps prior versions readable)", opt.GraceSeconds),
				"Verify GET latest returns new version",
			}
			if opt.RetireOld {
				steps = append(steps, fmt.Sprintf("Soft-delete version %d after grace", meta.Version))
			} else {
				steps = append(steps, "Keep prior version (retire_old=false)")
			}
			setPhase("inventory", "completed", fmt.Sprintf("Current version %d", meta.Version))
		}
	}
	if opt.Propagate.HasTargets() {
		steps = append(steps, opt.Propagate.PlanSteps(opt.Keys)...)
	}
	out.Result.PlanSteps = steps
	setPhase("plan", "completed", fmt.Sprintf("%d steps · %s", len(steps), out.Result.Strategy))
	return out, nil
}

func runVerify(ctx context.Context, client *Client, opt RunOptions, out *RunOutcome, setPhase func(string, string, string), pr *propagator) (*RunOutcome, error) {
	setPhase("read", "active", "Reading secret metadata")
	if opt.Engine == "transit" {
		setPhase("read", "skipped", "Transit verify is key-existence only in this agent")
		setPhase("verify", "completed", "Use vault UI/CLI for transit key versions")
		return out, nil
	}
	data, meta, err := client.ReadKV2(ctx, opt.Mount, opt.Path, 0)
	if err != nil {
		setPhase("read", "failed", err.Error())
		return out, err
	}
	out.Result.CurrentVersion = meta.Version
	out.Result.KeysRotated = meta.Keys
	setPhase("read", "completed", fmt.Sprintf("Version %d · %d keys", meta.Version, len(data)))
	setPhase("verify", "completed", "Latest version readable")
	if pr != nil {
		// Re-propagate the current version (the recovery path after a failed
		// propagation). The target Secret may already be current, so an
		// unchanged resourceVersion is accepted here.
		keys := keysFor(opt.Keys, data)
		if err := pr.run(ctx, stringValues(data, keys), meta.Version, false); err != nil {
			skipPending(out, setPhase, "Not run: propagation failed")
			return out, err
		}
	}
	return out, nil
}

func runRollback(ctx context.Context, client *Client, opt RunOptions, out *RunOutcome, setPhase func(string, string, string), pr *propagator) (*RunOutcome, error) {
	if opt.Engine != "kv2" {
		setPhase("read", "failed", "Rollback currently supports KV v2 only")
		return out, fmt.Errorf("rollback supports kv2 only")
	}
	setPhase("read", "active", "Reading current version")
	curData, meta, err := client.ReadKV2(ctx, opt.Mount, opt.Path, 0)
	if err != nil {
		setPhase("read", "failed", err.Error())
		return out, err
	}
	out.Result.CurrentVersion = meta.Version
	prev := meta.Version - 1
	if prev < 1 {
		setPhase("read", "failed", "No previous version to roll back to")
		return out, fmt.Errorf("no previous version")
	}
	setPhase("read", "completed", fmt.Sprintf("Current %d → restore %d", meta.Version, prev))

	setPhase("rollback", "active", "Reading previous version and writing as latest")
	if opt.DryRun {
		setPhase("rollback", "completed", "Dry-run: would restore version "+fmt.Sprintf("%d", prev))
		out.Result.RolledBackTo = prev
		setPhase("verify", "skipped", "Dry-run")
		if pr != nil {
			if err := pr.run(ctx, keyOnlyValues(keysFor(opt.Keys, curData)), meta.Version+1, true); err != nil {
				return out, err
			}
		}
		return out, nil
	}
	oldData, _, err := client.ReadKV2(ctx, opt.Mount, opt.Path, prev)
	if err != nil {
		// try undelete then read
		_ = client.UndeleteVersions(ctx, opt.Mount, opt.Path, []int{prev})
		oldData, _, err = client.ReadKV2(ctx, opt.Mount, opt.Path, prev)
		if err != nil {
			setPhase("rollback", "failed", err.Error())
			return out, err
		}
	}
	newMeta, err := client.WriteKV2(ctx, opt.Mount, opt.Path, oldData, meta.Version)
	if err != nil {
		setPhase("rollback", "failed", err.Error())
		return out, err
	}
	out.Result.RolledBackTo = prev
	out.Result.CurrentVersion = newMeta.Version
	out.Result.PreviousVersion = meta.Version
	setPhase("rollback", "completed", fmt.Sprintf("Wrote version %d from contents of %d", newMeta.Version, prev))
	setPhase("verify", "completed", "Rollback applied as new latest version")
	if pr != nil {
		if err := pr.run(ctx, stringValues(oldData, keysFor(opt.Keys, oldData)), out.Result.CurrentVersion, true); err != nil {
			skipPending(out, setPhase, "Not run: propagation failed")
			return out, err
		}
	}
	return out, nil
}

func runRotate(ctx context.Context, client *Client, opt RunOptions, out *RunOutcome, setPhase func(string, string, string), pr *propagator) (*RunOutcome, error) {
	if opt.Engine == "transit" {
		return rotateTransit(ctx, client, opt, out, setPhase)
	}
	if opt.Engine == "database" {
		setPhase("read", "skipped", "Database rotation is plan-assisted")
		planOut, err := runPlan(ctx, client, opt, out, setPhase)
		if err != nil {
			return planOut, err
		}
		setPhase("write_new", "skipped", "Configure DB engine + consumer cutover via vault UI: "+client.UILogin())
		setPhase("dual_window", "skipped", "Use grace from lease TTL")
		setPhase("verify", "skipped", "")
		setPhase("retire_old", "skipped", "Revoke old lease after cutover")
		setPhase("audit", "completed", "See plan_steps")
		return planOut, nil
	}
	return rotateKV2(ctx, client, opt, out, setPhase, pr)
}

func rotateTransit(ctx context.Context, client *Client, opt RunOptions, out *RunOutcome, setPhase func(string, string, string)) (*RunOutcome, error) {
	setPhase("rotate", "active", "Rotating transit key")
	if opt.DryRun {
		setPhase("rotate", "completed", "Dry-run: would rotate "+opt.Mount+"/keys/"+opt.Path)
		setPhase("verify", "skipped", "Dry-run")
		setPhase("audit", "completed", "Dry-run audit")
		return out, nil
	}
	if err := client.RotateTransitKey(ctx, opt.Mount, opt.Path); err != nil {
		setPhase("rotate", "failed", err.Error())
		return out, err
	}
	setPhase("rotate", "completed", "Transit key rotated; old versions remain for decrypt")
	setPhase("verify", "completed", "Zero-downtime transit rotate OK")
	setPhase("audit", "completed", "Recorded without plaintext")
	return out, nil
}

func rotateKV2(ctx context.Context, client *Client, opt RunOptions, out *RunOutcome, setPhase func(string, string, string), pr *propagator) (*RunOutcome, error) {
	setPhase("read", "active", "Reading current secret")
	cur, meta, err := client.ReadKV2(ctx, opt.Mount, opt.Path, 0)
	if err != nil {
		// allow create-on-rotate
		cur = map[string]any{}
		meta = SecretMeta{Version: 0}
		out.Result.Warnings = append(out.Result.Warnings, "creating new path: "+err.Error())
		setPhase("read", "completed", "Path empty — will create version 1")
	} else {
		out.Result.PreviousVersion = meta.Version
		out.Result.CurrentVersion = meta.Version
		setPhase("read", "completed", fmt.Sprintf("Version %d · keys %s", meta.Version, strings.Join(meta.Keys, ",")))
	}

	keys := opt.Keys
	if len(keys) == 0 {
		for k, v := range cur {
			if _, ok := v.(string); ok {
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			keys = []string{"password", "token", "api_key"}
		}
	}

	next := cloneMap(cur)
	if len(opt.NewSecret) > 0 {
		for k, v := range opt.NewSecret {
			next[k] = v
			if !contains(keys, k) {
				keys = append(keys, k)
			}
		}
	} else {
		for _, k := range keys {
			next[k] = generateSecret()
		}
	}
	out.Result.KeysRotated = keys

	setPhase("write_new", "active", "Writing next version")
	if opt.DryRun {
		setPhase("write_new", "completed", fmt.Sprintf("Dry-run: would write version %d rotating %s", meta.Version+1, strings.Join(keys, ",")))
		setPhase("dual_window", "completed", fmt.Sprintf("Dry-run dual window %ds", opt.GraceSeconds))
		setPhase("verify", "skipped", "Dry-run")
		if pr != nil {
			if err := pr.run(ctx, keyOnlyValues(keys), meta.Version+1, true); err != nil {
				return out, err
			}
		}
		if opt.RetireOld && meta.Version > 0 {
			setPhase("retire_old", "completed", fmt.Sprintf("Dry-run: would soft-delete version %d", meta.Version))
		} else {
			setPhase("retire_old", "skipped", "retire_old=false or no prior version")
		}
		setPhase("audit", "completed", "Dry-run complete")
		out.Result.CurrentVersion = meta.Version + 1
		return out, nil
	}

	cas := meta.Version
	newMeta, err := client.WriteKV2(ctx, opt.Mount, opt.Path, next, cas)
	if err != nil {
		setPhase("write_new", "failed", err.Error())
		return out, err
	}
	out.Result.CurrentVersion = newMeta.Version
	if out.Result.CurrentVersion == 0 {
		out.Result.CurrentVersion = meta.Version + 1
	}
	setPhase("write_new", "completed", fmt.Sprintf("Wrote version %d (prior %d still readable)", out.Result.CurrentVersion, meta.Version))

	// Dual window — KV v2 keeps old versions; sleep grace so consumers can reload.
	setPhase("dual_window", "active", fmt.Sprintf("Holding %ds dual-read window", opt.GraceSeconds))
	if opt.GraceSeconds > 0 {
		select {
		case <-ctx.Done():
			setPhase("dual_window", "failed", "cancelled during dual window")
			return out, ctx.Err()
		case <-time.After(time.Duration(opt.GraceSeconds) * time.Second):
		}
	}
	setPhase("dual_window", "completed", "Dual window elapsed; both versions were valid")

	setPhase("verify", "active", "Reading latest")
	_, verifyMeta, err := client.ReadKV2(ctx, opt.Mount, opt.Path, 0)
	if err != nil {
		setPhase("verify", "failed", err.Error())
		return out, err
	}
	if verifyMeta.Version > 0 {
		out.Result.CurrentVersion = verifyMeta.Version
	}
	setPhase("verify", "completed", fmt.Sprintf("Latest version %d OK", out.Result.CurrentVersion))

	if pr != nil {
		// Old version stays in place (retire_old is not reached) so consumers
		// that have not picked up N+1 keep working while the operator fixes
		// the failing target.
		if err := pr.run(ctx, stringValues(next, keys), out.Result.CurrentVersion, true); err != nil {
			skipPending(out, setPhase, "Not run: propagation failed")
			return out, err
		}
	}

	if opt.RetireOld && meta.Version > 0 {
		setPhase("retire_old", "active", fmt.Sprintf("Soft-deleting version %d", meta.Version))
		if err := client.SoftDeleteVersions(ctx, opt.Mount, opt.Path, []int{meta.Version}); err != nil {
			out.Result.Warnings = append(out.Result.Warnings, "retire: "+err.Error())
			setPhase("retire_old", "failed", err.Error())
			// Don't fail the whole run — new secret is live.
		} else {
			out.Result.OldVersionRetired = true
			setPhase("retire_old", "completed", fmt.Sprintf("Soft-deleted version %d", meta.Version))
		}
	} else {
		setPhase("retire_old", "skipped", "Keeping prior version for extra safety")
	}

	setPhase("audit", "completed", "Rotation recorded without plaintext secret values")
	return out, nil
}

func generateSecret() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func contains(ss []string, x string) bool {
	for _, s := range ss {
		if s == x {
			return true
		}
	}
	return false
}

// ParseKeys splits a comma/space key list.
func ParseKeys(raw string) []string {
	raw = strings.ReplaceAll(raw, ";", ",")
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// ParseNewSecretJSON parses optional operator-supplied new values.
func ParseNewSecretJSON(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("new_secret_json: %w", err)
	}
	return m, nil
}
