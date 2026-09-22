package secretsrot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jobshout/server/internal/model"
)

// Propagation pushes rotated values beyond Vault: plain k8s Secrets,
// ExternalSecret force-sync, GitHub Actions secrets and a Ring Promoter
// restart. Every field is a NAME; credentials are resolved server-side from
// Config so nothing sensitive is ever a launch value or a DB column.
type Propagation struct {
	Source          string // vault (default) | github
	Cluster         string
	ExternalSecrets []NamespacedName
	K8sSecrets      []NamespacedName
	GitHubRepo      string
	GitHubEnv       string
	GitHubNames     map[string]string // vault key → GitHub secret name override
	RPApp           string
	RPRings         []string
	RPDeployments   []string // override; else computed from Secret consumers
}

// NamespacedName is a namespace/name pair.
type NamespacedName struct{ Namespace, Name string }

func (n NamespacedName) String() string { return n.Namespace + "/" + n.Name }

// PropagationInput is the raw (string) form stored on the run row.
type PropagationInput struct {
	Source, Cluster, ExternalSecrets, K8sSecrets string
	GitHubRepo, GitHubEnvironment, GitHubNames   string
	RPApp, RPRings, RPDeployments                string
}

// Propagation phase keys, in execution order for source=vault.
const (
	PhaseK8sSecret      = "k8s_secret"
	PhaseExternalSecret = "external_secret"
	PhaseGitHubSecrets  = "github_secrets"
	PhaseRingRestart    = "ring_restart"
)

var (
	dns1123Re = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)
	rpNameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)
)

func splitList(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == ' ' || r == '\t'
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

func parseNamespaced(field, raw string) ([]NamespacedName, error) {
	var out []NamespacedName
	for _, item := range splitList(raw) {
		ns, name, ok := strings.Cut(item, "/")
		if !ok || !dns1123Re.MatchString(ns) || !dns1123Re.MatchString(name) || len(ns) > 63 || len(name) > 253 {
			return nil, fmt.Errorf("%s: %q is not namespace/name", field, item)
		}
		out = append(out, NamespacedName{Namespace: ns, Name: name})
	}
	return out, nil
}

// ParsePropagation validates the raw propagation fields. It is mode-agnostic;
// Execute adds the mode/engine checks.
func ParsePropagation(in PropagationInput) (Propagation, error) {
	p := Propagation{
		Source:     strings.ToLower(strings.TrimSpace(in.Source)),
		Cluster:    strings.TrimSpace(in.Cluster),
		GitHubRepo: strings.TrimSpace(in.GitHubRepo),
		GitHubEnv:  strings.TrimSpace(in.GitHubEnvironment),
		RPApp:      strings.TrimSpace(in.RPApp),
	}
	if p.Source == "" {
		p.Source = "vault"
	}
	if p.Source != "vault" && p.Source != "github" {
		return p, fmt.Errorf("source must be vault or github")
	}
	if p.Cluster != "" && !ValidClusterName(p.Cluster) {
		return p, fmt.Errorf("cluster %q: use the kubeconfig name (a-z, 0-9, -)", p.Cluster)
	}
	var err error
	if p.ExternalSecrets, err = parseNamespaced("external_secrets", in.ExternalSecrets); err != nil {
		return p, err
	}
	if p.K8sSecrets, err = parseNamespaced("k8s_secrets", in.K8sSecrets); err != nil {
		return p, err
	}
	if (len(p.ExternalSecrets) > 0 || len(p.K8sSecrets) > 0) && p.Cluster == "" {
		return p, fmt.Errorf("cluster is required for external_secrets / k8s_secrets")
	}
	if p.GitHubRepo != "" && !githubRepoRe.MatchString(p.GitHubRepo) {
		return p, fmt.Errorf("github_repo must be owner/name")
	}
	if p.GitHubEnv != "" && !githubEnvRe.MatchString(p.GitHubEnv) {
		return p, fmt.Errorf("github_environment %q is not a valid environment name", p.GitHubEnv)
	}
	if names := splitList(in.GitHubNames); len(names) > 0 {
		p.GitHubNames = map[string]string{}
		for _, pair := range names {
			k, v, ok := strings.Cut(pair, "=")
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			if !ok || k == "" || v == "" {
				return p, fmt.Errorf("github_secret_names: %q is not key=SECRET_NAME", pair)
			}
			if err := validGitHubSecretName(v); err != nil {
				return p, fmt.Errorf("github_secret_names: %w", err)
			}
			p.GitHubNames[k] = v
		}
	}
	if (p.GitHubEnv != "" || len(p.GitHubNames) > 0) && p.GitHubRepo == "" {
		return p, fmt.Errorf("github_repo is required for github_environment / github_secret_names")
	}
	p.RPRings = splitList(in.RPRings)
	for _, r := range p.RPRings {
		if !rpNameRe.MatchString(r) {
			return p, fmt.Errorf("rp_rings: invalid ring %q", r)
		}
	}
	p.RPDeployments = splitList(in.RPDeployments)
	for _, d := range p.RPDeployments {
		if !dns1123Re.MatchString(d) {
			return p, fmt.Errorf("rp_deployments: invalid deployment %q", d)
		}
	}
	if len(p.RPDeployments) > 0 && len(p.RPRings) == 0 {
		return p, fmt.Errorf("rp_deployments requires rp_rings")
	}
	if len(p.RPRings) > 0 && p.RPApp == "" {
		p.RPApp = "jobshout"
	}
	if p.RPApp != "" && !rpNameRe.MatchString(p.RPApp) {
		return p, fmt.Errorf("rp_app: invalid app %q", p.RPApp)
	}
	if p.Source == "github" {
		if p.GitHubRepo == "" {
			return p, fmt.Errorf("source=github requires github_repo")
		}
		if len(p.ExternalSecrets) > 0 {
			return p, fmt.Errorf("external_secrets read from Vault; they cannot be used with source=github")
		}
	}
	return p, nil
}

// HasTargets reports whether any propagation is configured.
func (p Propagation) HasTargets() bool {
	return len(p.K8sSecrets) > 0 || len(p.ExternalSecrets) > 0 || p.GitHubRepo != "" || len(p.RPRings) > 0
}

// phaseOrder is the execution order of the configured propagation phases.
func (p Propagation) phaseOrder() []string {
	var keys []string
	if p.Source == "github" && p.GitHubRepo != "" {
		keys = append(keys, PhaseGitHubSecrets)
	}
	if len(p.K8sSecrets) > 0 {
		keys = append(keys, PhaseK8sSecret)
	}
	if len(p.ExternalSecrets) > 0 {
		keys = append(keys, PhaseExternalSecret)
	}
	if p.Source != "github" && p.GitHubRepo != "" {
		keys = append(keys, PhaseGitHubSecrets)
	}
	if len(p.RPRings) > 0 {
		keys = append(keys, PhaseRingRestart)
	}
	return keys
}

func (p Propagation) phases() []model.SecretsRotationPhase {
	labels := map[string]string{
		PhaseK8sSecret:      "Patch Kubernetes Secrets",
		PhaseExternalSecret: "Force-sync ExternalSecrets",
		PhaseGitHubSecrets:  "Update GitHub Actions secrets",
		PhaseRingRestart:    "Restart rings (Ring Promoter)",
	}
	var out []model.SecretsRotationPhase
	for _, k := range p.phaseOrder() {
		out = append(out, model.SecretsRotationPhase{Key: k, Label: labels[k], Status: "pending"})
	}
	return out
}

func (p Propagation) githubTarget() string {
	if p.GitHubEnv != "" {
		return p.GitHubRepo + "@" + p.GitHubEnv
	}
	return p.GitHubRepo
}

// githubNames maps each propagated key to its GitHub secret name.
func (p Propagation) githubNames(keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		name := p.GitHubNames[k]
		if name == "" {
			name = GitHubSecretName(k)
		}
		if err := validGitHubSecretName(name); err != nil {
			return nil, fmt.Errorf("key %q: %w (set github_secret_names)", k, err)
		}
		out[k] = name
	}
	return out, nil
}

// PlanSteps describes the propagation without values.
func (p Propagation) PlanSteps(keys []string) []string {
	keyList := strings.Join(keys, ",")
	if keyList == "" {
		keyList = "(rotated keys)"
	}
	var steps []string
	for _, ph := range p.phaseOrder() {
		switch ph {
		case PhaseK8sSecret:
			for _, s := range p.K8sSecrets {
				steps = append(steps, fmt.Sprintf("Patch Secret %s on %s: merge keys %s (other keys untouched)", s, p.Cluster, keyList))
			}
		case PhaseExternalSecret:
			for _, s := range p.ExternalSecrets {
				steps = append(steps, fmt.Sprintf("Annotate ExternalSecret %s on %s with force-sync; wait for Ready and a new target Secret revision", s, p.Cluster))
			}
		case PhaseGitHubSecrets:
			if len(keys) == 0 {
				steps = append(steps, "Seal and PUT a GitHub Actions secret per rotated key (KEY upper-cased unless mapped) on "+p.githubTarget())
				continue
			}
			names, err := p.githubNames(keys)
			if err != nil {
				steps = append(steps, "GitHub Actions secrets on "+p.githubTarget()+": "+err.Error())
				continue
			}
			list := make([]string, 0, len(keys))
			for _, k := range keys {
				list = append(list, names[k])
			}
			steps = append(steps, fmt.Sprintf("Seal and PUT GitHub Actions secrets %s on %s", strings.Join(list, ","), p.githubTarget()))
		case PhaseRingRestart:
			which := "Deployments in namespace <ring> that reference a rotated Secret"
			if len(p.RPDeployments) > 0 {
				which = strings.Join(p.RPDeployments, ",")
			} else if len(p.K8sSecrets) == 0 && len(p.ExternalSecrets) == 0 {
				which = "Ring Promoter default deployments"
			}
			for _, r := range p.RPRings {
				steps = append(steps, fmt.Sprintf("Ring Promoter: restart %s/%s (%s), wait for the job and ring health", p.RPApp, r, which))
			}
		}
	}
	return steps
}

// checkCredentials verifies the named server-side credentials exist. It never
// returns or logs their content. Returns the kube client when a cluster is used.
func (p Propagation) checkCredentials(cfg Config) (*KubeClient, []string) {
	var problems []string
	var kube *KubeClient
	if len(p.K8sSecrets) > 0 || len(p.ExternalSecrets) > 0 {
		kc, err := NewKubeClientForCluster(cfg.KubeconfigDir, p.Cluster, cfg.Timeout)
		if err != nil {
			problems = append(problems, err.Error())
		} else {
			kube = kc
		}
	}
	if p.GitHubRepo != "" && cfg.GitHubToken == "" {
		problems = append(problems, "SECRETS_ROT_GITHUB_TOKEN is not set")
	}
	if len(p.RPRings) > 0 && cfg.RPToken == "" {
		problems = append(problems, "RP_API_TOKEN is not set")
	}
	for _, r := range p.RPRings {
		if r == "prod" && cfg.RPProdPassword == "" {
			problems = append(problems, "RP_PROD_PASSWORD is not set (Ring Promoter requires it for ring prod)")
		}
	}
	return kube, problems
}

// stringValues converts rotated values to strings for k8s / GitHub.
func stringValues(src map[string]any, keys []string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		v, ok := src[k]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			out[k] = t
		default:
			b, _ := json.Marshal(t)
			out[k] = string(b)
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// propagator runs the propagation phases for one run.
type propagator struct {
	cfg       Config
	opt       RunOptions
	p         Propagation
	kube      *KubeClient
	out       *RunOutcome
	set       func(string, string, string)
	esoRV     map[string]string // ExternalSecret → target Secret resourceVersion before the Vault write
	esoTarget map[string]string // ExternalSecret → target Secret name

	// redact holds the in-memory values (plain and base64) so any upstream
	// error text that happens to echo one is scrubbed before it is stored.
	redact []string
}

func (pr *propagator) setValues(values map[string]string) {
	pr.redact = pr.redact[:0]
	for _, v := range values {
		if len(v) < 4 {
			continue
		}
		pr.redact = append(pr.redact, v, base64.StdEncoding.EncodeToString([]byte(v)))
	}
}

func (pr *propagator) scrub(s string) string {
	for _, v := range pr.redact {
		s = strings.ReplaceAll(s, v, "[redacted]")
	}
	return s
}

func (pr *propagator) summary() *model.SecretsRotationPropagation {
	if pr.out.Result.Propagation == nil {
		pr.out.Result.Propagation = &model.SecretsRotationPropagation{Source: pr.p.Source, Cluster: pr.p.Cluster}
	}
	return pr.out.Result.Propagation
}

func (pr *propagator) addTarget(t model.SecretsRotationTarget) {
	t.Message = pr.scrub(t.Message)
	s := pr.summary()
	s.Targets = append(s.Targets, t)
}

// precheck runs read-only cluster checks before anything is written: every
// k8s_secrets target must exist and must not be Helm-managed (a helm upgrade
// would silently revert the patch), and each ExternalSecret's target Secret
// resourceVersion is recorded so the wait can tell a genuine re-sync from a
// no-op. It returns one problem string per failed check.
func (pr *propagator) precheck(ctx context.Context) []string {
	if pr.kube == nil {
		return nil
	}
	var problems []string
	for _, s := range pr.p.K8sSecrets {
		if err := pr.checkPatchable(ctx, s); err != nil {
			problems = append(problems, err.Error())
		}
	}
	pr.esoRV = map[string]string{}
	pr.esoTarget = map[string]string{}
	for _, es := range pr.p.ExternalSecrets {
		obj, _, err := pr.kube.GetExternalSecret(ctx, es.Namespace, es.Name)
		if err != nil {
			problems = append(problems, pr.esoErr(es, err).Error())
			continue
		}
		pr.esoTarget[es.String()] = obj.targetName()
		rv, err := pr.kube.SecretResourceVersion(ctx, es.Namespace, obj.targetName())
		if err != nil {
			problems = append(problems, fmt.Sprintf("externalsecret %s target: %v", es, err))
			continue
		}
		pr.esoRV[es.String()] = rv
	}
	return problems
}

// checkPatchable refuses Secrets that do not exist or that Helm owns.
func (pr *propagator) checkPatchable(ctx context.Context, s NamespacedName) error {
	info, err := pr.kube.GetSecretInfo(ctx, s.Namespace, s.Name)
	if err != nil {
		return fmt.Errorf("secret %s: %w", s, err)
	}
	if !info.Exists {
		return fmt.Errorf("secret %s not found on cluster %s", s, pr.p.Cluster)
	}
	if info.HelmManaged {
		return fmt.Errorf("secret %s is Helm-managed; a helm upgrade would overwrite a direct patch. Change the chart values / Vault source instead, or move the keys to an extraSecretRefs Secret", s)
	}
	return nil
}

func (pr *propagator) esoErr(es NamespacedName, err error) error {
	if errors.Is(err, errNoESO) {
		return fmt.Errorf("External Secrets Operator not found on cluster %s (ExternalSecret %s)", pr.p.Cluster, es)
	}
	return err
}

// run executes every configured phase. values holds the rotated (or restored)
// key→value pairs in memory only. vaultVersion is 0 for source=github.
// requireChange=false (verify re-propagation) accepts an ExternalSecret whose
// target Secret was already current.
func (pr *propagator) run(ctx context.Context, values map[string]string, vaultVersion int, requireChange bool) error {
	pr.setValues(values)
	sum := pr.summary()
	sum.VaultVersion = vaultVersion
	keys := sortedKeys(values)
	for _, phase := range pr.p.phaseOrder() {
		var err error
		switch phase {
		case PhaseK8sSecret:
			err = pr.runK8sSecrets(ctx, values, keys)
		case PhaseExternalSecret:
			err = pr.runExternalSecrets(ctx, requireChange)
		case PhaseGitHubSecrets:
			err = pr.runGitHub(ctx, values, keys)
		case PhaseRingRestart:
			err = pr.runRings(ctx)
		}
		if err != nil {
			return pr.failure(phase, err, vaultVersion)
		}
	}
	return nil
}

func (pr *propagator) failure(phase string, err error, vaultVersion int) error {
	var where string
	if pr.p.Source == "github" {
		where = "New values were generated for source=github but are stored nowhere else; re-run mode=rotate to push a fresh set."
	} else {
		where = fmt.Sprintf("Vault %s/%s is at version %d. Re-run mode=verify with the same targets to re-propagate that version.", pr.opt.Mount, pr.opt.Path, vaultVersion)
	}
	return fmt.Errorf("propagation failed at %s: %s. %s", phase, pr.scrub(err.Error()), where)
}

func (pr *propagator) runK8sSecrets(ctx context.Context, values map[string]string, keys []string) error {
	pr.set(PhaseK8sSecret, "active", fmt.Sprintf("Patching %d Secret(s) on %s", len(pr.p.K8sSecrets), pr.p.Cluster))
	if pr.opt.DryRun {
		for _, s := range pr.p.K8sSecrets {
			pr.addTarget(model.SecretsRotationTarget{Kind: PhaseK8sSecret, Name: s.String(), Status: "dry_run"})
		}
		pr.set(PhaseK8sSecret, "completed", fmt.Sprintf("Dry-run: would merge keys %s into %s", strings.Join(keys, ","), joinNN(pr.p.K8sSecrets)))
		return nil
	}
	for _, s := range pr.p.K8sSecrets {
		// Re-check right before writing: ownership can change after preflight.
		err := pr.checkPatchable(ctx, s)
		rv := ""
		if err == nil {
			rv, err = pr.kube.PatchSecretKeys(ctx, s.Namespace, s.Name, values)
		}
		if err != nil {
			pr.addTarget(model.SecretsRotationTarget{Kind: PhaseK8sSecret, Name: s.String(), Status: "failed", Message: err.Error()})
			pr.set(PhaseK8sSecret, "failed", s.String()+": "+err.Error())
			return fmt.Errorf("%s: %w", s, err)
		}
		pr.addTarget(model.SecretsRotationTarget{Kind: PhaseK8sSecret, Name: s.String(), Status: "completed", ResourceVersion: rv})
	}
	pr.set(PhaseK8sSecret, "completed", fmt.Sprintf("Merged keys %s into %s", strings.Join(keys, ","), joinNN(pr.p.K8sSecrets)))
	return nil
}

func (pr *propagator) runExternalSecrets(ctx context.Context, requireChange bool) error {
	pr.set(PhaseExternalSecret, "active", fmt.Sprintf("Force-syncing %d ExternalSecret(s) on %s", len(pr.p.ExternalSecrets), pr.p.Cluster))
	if pr.opt.DryRun {
		for _, s := range pr.p.ExternalSecrets {
			pr.addTarget(model.SecretsRotationTarget{Kind: PhaseExternalSecret, Name: s.String(), Status: "dry_run"})
		}
		pr.set(PhaseExternalSecret, "completed", "Dry-run: would annotate force-sync and wait for "+joinNN(pr.p.ExternalSecrets))
		return nil
	}
	for _, s := range pr.p.ExternalSecrets {
		rv, err := pr.syncExternalSecret(ctx, s, requireChange)
		if err != nil {
			pr.addTarget(model.SecretsRotationTarget{Kind: PhaseExternalSecret, Name: s.String(), Status: "failed", Message: err.Error()})
			pr.set(PhaseExternalSecret, "failed", s.String()+": "+err.Error())
			return fmt.Errorf("%s: %w", s, err)
		}
		pr.addTarget(model.SecretsRotationTarget{Kind: PhaseExternalSecret, Name: s.String(), Status: "completed", ResourceVersion: rv})
	}
	pr.set(PhaseExternalSecret, "completed", "Synced "+joinNN(pr.p.ExternalSecrets))
	return nil
}

func (pr *propagator) syncExternalSecret(ctx context.Context, s NamespacedName, requireChange bool) (string, error) {
	obj, version, err := pr.kube.GetExternalSecret(ctx, s.Namespace, s.Name)
	if err != nil {
		return "", pr.esoErr(s, err)
	}
	target := obj.targetName()
	if pr.esoTarget == nil {
		pr.esoTarget = map[string]string{}
	}
	pr.esoTarget[s.String()] = target
	baseline, haveBaseline := pr.esoRV[s.String()]
	if !haveBaseline {
		baseline, err = pr.kube.SecretResourceVersion(ctx, s.Namespace, target)
		if err != nil {
			return "", err
		}
	}
	ts := time.Now().Unix()
	if err := pr.kube.AnnotateForceSync(ctx, version, s.Namespace, s.Name, ts); err != nil {
		return "", err
	}
	timeout := pr.cfg.ESOTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	poll := pr.cfg.ESOPollInterval
	if poll <= 0 {
		poll = 2 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		es, _, err := pr.kube.GetExternalSecret(ctx, s.Namespace, s.Name)
		if err != nil {
			return "", err
		}
		ready, readyMsg := false, ""
		for _, c := range es.Status.Conditions {
			if c.Type == "Ready" {
				ready = c.Status == "True"
				readyMsg = strings.TrimSpace(c.Reason + " " + c.Message)
			}
		}
		refreshed := false
		if t, perr := time.Parse(time.RFC3339, es.Status.RefreshTime); perr == nil {
			refreshed = t.Unix() >= ts
		}
		rv, err := pr.kube.SecretResourceVersion(ctx, s.Namespace, target)
		if err != nil {
			return "", err
		}
		changed := !requireChange || (rv != "" && rv != baseline)
		if ready && refreshed && changed {
			return rv, nil
		}
		if time.Now().After(deadline) {
			if len(readyMsg) > 200 {
				readyMsg = readyMsg[:200] + "…"
			}
			return "", fmt.Errorf("not synced within %s (ready=%t refreshed=%t secret %s changed=%t; %s)",
				timeout, ready, refreshed, target, changed, readyMsg)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (pr *propagator) runGitHub(ctx context.Context, values map[string]string, keys []string) error {
	target := pr.p.githubTarget()
	pr.set(PhaseGitHubSecrets, "active", "Updating GitHub Actions secrets on "+target)
	names, err := pr.p.githubNames(keys)
	if err != nil {
		pr.set(PhaseGitHubSecrets, "failed", err.Error())
		return err
	}
	byName := make(map[string]string, len(keys))
	order := make([]string, 0, len(keys))
	for _, k := range keys {
		byName[names[k]] = values[k]
		order = append(order, names[k])
	}
	if pr.opt.DryRun {
		for _, n := range order {
			pr.addTarget(model.SecretsRotationTarget{Kind: "github_secret", Name: target + "/" + n, Status: "dry_run"})
		}
		pr.set(PhaseGitHubSecrets, "completed", fmt.Sprintf("Dry-run: would seal and PUT %s on %s", strings.Join(order, ","), target))
		return nil
	}
	gh := NewGitHubClient(pr.cfg.GitHubAPI, pr.cfg.GitHubToken, pr.cfg.Timeout)
	written, err := gh.PutSecrets(ctx, pr.p.GitHubRepo, pr.p.GitHubEnv, byName, order)
	done := map[string]bool{}
	for _, n := range written {
		done[n] = true
		pr.addTarget(model.SecretsRotationTarget{Kind: "github_secret", Name: target + "/" + n, Status: "completed"})
	}
	if err != nil {
		for _, n := range order {
			if !done[n] {
				pr.addTarget(model.SecretsRotationTarget{Kind: "github_secret", Name: target + "/" + n, Status: "failed", Message: err.Error()})
			}
		}
		pr.set(PhaseGitHubSecrets, "failed", err.Error())
		return err
	}
	pr.set(PhaseGitHubSecrets, "completed", fmt.Sprintf("Updated %s on %s", strings.Join(written, ","), target))
	return nil
}

func (pr *propagator) runRings(ctx context.Context) error {
	pr.set(PhaseRingRestart, "active", fmt.Sprintf("Restarting %s rings %s", pr.p.RPApp, strings.Join(pr.p.RPRings, "→")))
	if pr.opt.DryRun {
		for _, r := range pr.p.RPRings {
			pr.addTarget(model.SecretsRotationTarget{Kind: "ring", Name: pr.p.RPApp + "/" + r, Status: "dry_run"})
		}
		pr.set(PhaseRingRestart, "completed", fmt.Sprintf("Dry-run: would restart %s rings %s in order", pr.p.RPApp, strings.Join(pr.p.RPRings, ",")))
		return nil
	}
	rp := NewRPClient(pr.cfg)
	var restartNotes []string
	reason := strings.TrimSpace(fmt.Sprintf("secrets rotation %s %s/%s", pr.opt.RunID, pr.opt.Mount, pr.opt.Path))
	for i, r := range pr.p.RPRings {
		name := pr.p.RPApp + "/" + r
		pr.set(PhaseRingRestart, "active", fmt.Sprintf("Restarting %s (%d/%d)", name, i+1, len(pr.p.RPRings)))
		fail := func(jobID string, err error) error {
			pr.addTarget(model.SecretsRotationTarget{Kind: "ring", Name: name, Status: "failed", JobID: jobID, Message: err.Error()})
			for _, rest := range pr.p.RPRings[i+1:] {
				pr.addTarget(model.SecretsRotationTarget{Kind: "ring", Name: pr.p.RPApp + "/" + rest, Status: "skipped", Message: "earlier ring failed"})
			}
			pr.set(PhaseRingRestart, "failed", name+": "+err.Error())
			return fmt.Errorf("%s: %w", name, err)
		}
		deployments, skip, err := pr.ringDeployments(ctx, r)
		if err != nil {
			return fail("", err)
		}
		if skip != "" {
			pr.addTarget(model.SecretsRotationTarget{Kind: "ring", Name: name, Status: "skipped", Message: skip})
			restartNotes = append(restartNotes, r+": "+skip)
			continue
		}
		jobID, err := rp.StartRestart(ctx, pr.p.RPApp, r, reason, deployments)
		if err != nil {
			return fail("", err)
		}
		if err := rp.WaitJob(ctx, pr.p.RPApp, jobID); err != nil {
			return fail(jobID, err)
		}
		healthy, err := rp.RingHealthy(ctx, pr.p.RPApp, r)
		if err != nil {
			return fail(jobID, err)
		}
		if !healthy {
			return fail(jobID, fmt.Errorf("ring reported unhealthy after restart"))
		}
		msg := "restarted Ring Promoter default deployments"
		if len(deployments) > 0 {
			msg = "restarted " + strings.Join(deployments, ",")
		}
		pr.addTarget(model.SecretsRotationTarget{Kind: "ring", Name: name, Status: "completed", JobID: jobID, Message: msg})
		restartNotes = append(restartNotes, r+": "+msg+" (healthy)")
	}
	pr.set(PhaseRingRestart, "completed", pr.p.RPApp+" · "+strings.Join(restartNotes, " · "))
	return nil
}

// ringDeployments decides what to restart in a ring (namespace = ring name).
// rp_deployments wins; otherwise, with cluster access, only Deployments whose
// pod template references a Secret this run changed. Without cluster access
// Ring Promoter's default set is used. A non-empty skip means nothing in the
// ring consumes the rotated Secrets.
func (pr *propagator) ringDeployments(ctx context.Context, ring string) (deployments []string, skip string, err error) {
	if len(pr.p.RPDeployments) > 0 {
		return pr.p.RPDeployments, "", nil
	}
	if pr.kube == nil {
		return nil, "", nil
	}
	affected := map[string]bool{}
	for _, s := range pr.p.K8sSecrets {
		if s.Namespace == ring {
			affected[s.Name] = true
		}
	}
	for _, es := range pr.p.ExternalSecrets {
		if es.Namespace == ring {
			if t := pr.esoTarget[es.String()]; t != "" {
				affected[t] = true
			} else {
				affected[es.Name] = true
			}
		}
	}
	if len(affected) == 0 {
		return nil, "no rotated Secret in namespace " + ring + "; not restarted", nil
	}
	consumers, err := pr.kube.SecretConsumers(ctx, ring, affected)
	if err != nil {
		return nil, "", err
	}
	if len(consumers) == 0 {
		names := make([]string, 0, len(affected))
		for n := range affected {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, "no Deployment in " + ring + " references " + strings.Join(names, ",") + "; not restarted", nil
	}
	return consumers, "", nil
}

func joinNN(list []NamespacedName) string {
	s := make([]string, len(list))
	for i, n := range list {
		s[i] = n.String()
	}
	return strings.Join(s, ",")
}

func newPropagator(cfg Config, opt RunOptions, kube *KubeClient, out *RunOutcome, set func(string, string, string)) *propagator {
	pr := &propagator{cfg: cfg, opt: opt, p: opt.Propagate, kube: kube, out: out}
	pr.set = func(key, status, msg string) { set(key, status, pr.scrub(msg)) }
	return pr
}
