package secretsrot

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner is the launch surface satisfied by the secrets rotation service.
type Runner interface {
	CreateRun(ctx context.Context, req model.CreateSecretsRotationRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.SecretsRotationRun, error)
}

// Module is the Secrets Rotation specialist (HC Vault + WSLVault).
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:   model.BuiltinSecretsRotation,
		Label:     "Secrets Rotation",
		Icon:      "key",
		TabSlug:   "secrets",
		Hint:      "Rotate secrets on HashiCorp Vault or WSLVault with a zero-downtime dual-version window.",
		ChatHint:  "To rotate secrets safely, call agent_execute on Secrets Rotation. Pass mount + path; prefer mode=plan first, then rotate. Works with WSLVault (vault-ui.workstation.co.uk) and HashiCorp Vault. Optional propagation: cluster (kubeconfig NAME, e.g. k3s1), k8s_secrets / external_secrets (namespace/name), github_repo, rp_rings. Credentials are server-side and named — never ask for or pass a kubeconfig, token or password.",
		Schema:    schema(),
		Seed:      Seed,
		Launch:    launch(run),
		StayOnTab: true,
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if vals["path"] != "" {
				return
			}
			for _, f := range strings.Fields(prompt) {
				f = strings.Trim(f, ".,;:\"'()")
				if strings.Contains(f, "/") && !strings.HasPrefix(f, "http") {
					vals["path"] = strings.TrimPrefix(f, "secret/data/")
					vals["path"] = strings.TrimPrefix(vals["path"], "secret/")
					return
				}
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinSecretsRotation,
		Hint:    "Zero-downtime secret rotation via KV v2 dual versions, transit key rotate, or DB lease cutover plan.",
		Fields: []agentschema.Field{
			{Key: "mode", Label: "Mode", Type: "select", Required: true, Default: "plan",
				Options: []model.ClarifyOption{
					{Label: "Plan only (no writes)", Value: "plan"},
					{Label: "Rotate (zero downtime)", Value: "rotate"},
					{Label: "Verify current version", Value: "verify"},
					{Label: "Rollback to previous", Value: "rollback"},
				},
				Question: "Plan, rotate, verify, or roll back?",
				Help:     "With Propagate targets set, verify re-pushes the current Vault version to them (recovery after a failed propagation)"},
			{Key: "provider", Label: "Vault product", Type: "select", Required: false, Default: "auto",
				Options: []model.ClarifyOption{
					{Label: "Auto-detect", Value: "auto"},
					{Label: "WSLVault (workstation)", Value: "wslvault"},
					{Label: "HashiCorp Vault", Value: "hashicorp"},
				},
				Help: "WSLVault UI: https://vault-ui.workstation.co.uk/login · docs: https://www.wslvault.org/"},
			{Key: "vault_addr", Label: "Vault API address", Type: "text", Required: false,
				Placeholder: DefaultWSLVaultAddr,
				Help:        "Defaults to SECRETS_VAULT_ADDR / VAULT_ADDR / WSLVault"},
			{Key: "engine", Label: "Engine", Type: "select", Required: false, Default: "kv2",
				Options: []model.ClarifyOption{
					{Label: "KV v2 (static secrets)", Value: "kv2"},
					{Label: "Transit (encryption keys)", Value: "transit"},
					{Label: "Database (dynamic leases — plan)", Value: "database"},
				}},
			{Key: "mount", Label: "Mount", Type: "text", Required: false, Default: "secret",
				Placeholder: "secret", Help: "KV/transit/database mount path"},
			{Key: "path", Label: "Secret path", Type: "text", Required: true, MinLength: 1,
				Placeholder: "prod/db/creds",
				Help:        "Path under the mount (no mount prefix)", Question: "Which secret path should I rotate?"},
			{Key: "keys", Label: "Keys to rotate", Type: "text", Required: false,
				Placeholder: "password, api_key",
				Help:        "Comma-separated; empty rotates all string fields (or password/token/api_key on create). Required when source=github"},
			{Key: "grace_seconds", Label: "Dual-window seconds", Type: "text", Required: false, Default: "300",
				Help: "How long both old and new KV versions stay valid before optional soft-delete"},
			{Key: "retire_old", Label: "Retire old version", Type: "select", Required: false, Default: "false",
				Options: []model.ClarifyOption{
					{Label: "Keep prior version", Value: "false"},
					{Label: "Soft-delete after grace", Value: "true"},
				}},
			{Key: "dry_run", Label: "Dry run", Type: "select", Required: false, Default: "false",
				Options: []model.ClarifyOption{
					{Label: "Live", Value: "false"},
					{Label: "Dry run (no writes)", Value: "true"},
				}},
			// Propagation — names only. Credentials (kubeconfig, GitHub token,
			// Ring Promoter token) are server-side because launch values are
			// persisted on the task.
			{Key: "source", Label: "Source of truth", Type: "select", Required: false, Default: "vault", Group: "Propagate",
				Options: []model.ClarifyOption{
					{Label: "Vault (write Vault, then propagate)", Value: "vault"},
					{Label: "GitHub Actions secrets (skip Vault)", Value: "github"},
				},
				Help: "github: generate new values for keys and write them to GitHub only (plan/rotate)"},
			{Key: "cluster", Label: "Cluster", Type: "text", Required: false, Group: "Propagate",
				Placeholder: "k3s1",
				Help:        "Name of a server-side kubeconfig (SECRETS_ROT_KUBECONFIG_DIR/<name>). Never paste a kubeconfig here"},
			{Key: "k8s_secrets", Label: "Kubernetes Secrets to patch", Type: "text", Required: false, Group: "Propagate",
				Placeholder: "int/jobshout-cms, test/jobshout-cms",
				Help:        "namespace/name, comma-separated. Only rotated keys are merged. Helm-managed Secrets are refused"},
			{Key: "external_secrets", Label: "ExternalSecrets to force-sync", Type: "text", Required: false, Group: "Propagate",
				Placeholder: "int/jobshout-secrets",
				Help:        "namespace/name, comma-separated; needs External Secrets Operator on the cluster"},
			{Key: "github_repo", Label: "GitHub repo", Type: "text", Required: false, Group: "Propagate",
				Placeholder: "owner/name", Help: "Update GitHub Actions secrets (token is server-side)"},
			{Key: "github_environment", Label: "GitHub environment", Type: "text", Required: false, Group: "Propagate",
				Placeholder: "production", Help: "Optional; empty writes repository secrets"},
			{Key: "github_secret_names", Label: "GitHub secret names", Type: "text", Required: false, Group: "Propagate",
				Placeholder: "password=DB_PASSWORD", Help: "key=SECRET_NAME, comma-separated; default is the key upper-cased"},
			{Key: "rp_rings", Label: "Restart rings", Type: "text", Required: false, Group: "Propagate",
				Placeholder: "int, test", Help: "Ring Promoter rings to restart in order after propagation (namespace = ring)"},
			{Key: "rp_app", Label: "Ring Promoter app", Type: "text", Required: false, Group: "Propagate",
				Placeholder: "jobshout", Help: "Defaults to jobshout when rings are set"},
			{Key: "rp_deployments", Label: "Deployments to restart", Type: "text", Required: false, Group: "Propagate",
				Placeholder: "jobshout-api",
				Help:        "Optional override; default is every Deployment in the ring that references a rotated Secret"},
			{Key: "instruction", Label: "Note (optional)", Type: "textarea",
				Placeholder: "e.g. ExternalSecret jobshout-secrets; rotate after deploy window"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Secrets: ", FromKey: "path", Fallback: "rotation"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "mode", Prefix: "Mode: "},
			{Key: "cluster", Prefix: "Cluster: "},
			{Key: "rp_rings", Prefix: "Restart rings: "},
			{Key: "provider", Prefix: "Provider: "},
			{Key: "engine", Prefix: "Engine: "},
			{Key: "instruction"},
		},
	}
}

// Seed is the built-in Secrets Rotation agent.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Rotates secrets on HashiCorp Vault and WSLVault without downtime: KV v2 dual-version windows, transit key rotation, and database lease cutover plans. Optionally propagates to Kubernetes Secrets, ExternalSecrets, GitHub Actions secrets and a Ring Promoter restart. Integrates with vault-ui.workstation.co.uk."
	prompt := "You are the Secrets Rotation agent. Prefer mode=plan before rotate. Use Vault-compatible APIs against WSLVault (https://vault.workstation.co.uk, UI https://vault-ui.workstation.co.uk/login, docs https://www.wslvault.org/) or HashiCorp Vault. Never echo secret values in chat or logs—only paths, versions, and keys. For KV v2: write N+1, keep N readable through a grace window, then optionally soft-delete N. For transit: rotate key versions so old ciphertext still decrypts. For database: produce a cutover plan (new lease → consumers → revoke old). Propagation is opt-in: after the Vault write and verify, patch plain Kubernetes Secrets (never Helm-managed ones), force-sync ExternalSecrets, update GitHub Actions secrets, then restart Ring Promoter rings whose Deployments consume the rotated Secrets. source=github makes GitHub Actions the source of truth and skips Vault. Clusters, GitHub and Ring Promoter credentials are named server-side (cluster=k3s1 is a kubeconfig name) — never ask for or accept a kubeconfig, token or password in chat or launch fields."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameSecretsRotation,
		Role:         "Secrets Rotation Specialist",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinSecretsRotation},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("Secrets Rotation is not configured")
		}
		path := strings.TrimSpace(in.Values["path"])
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		mode := strings.TrimSpace(in.Values["mode"])
		if mode == "" {
			mode = "plan"
		}
		grace := 300
		if g := strings.TrimSpace(in.Values["grace_seconds"]); g != "" {
			fmt.Sscanf(g, "%d", &grace)
		}
		payload := model.CreateSecretsRotationRequest{
			AgentID:      in.Agent.ID,
			TaskID:       &in.Task.ID,
			Mode:         mode,
			Provider:     strings.TrimSpace(in.Values["provider"]),
			VaultAddr:    strings.TrimSpace(in.Values["vault_addr"]),
			Mount:        strings.TrimSpace(in.Values["mount"]),
			Path:         path,
			Engine:       strings.TrimSpace(in.Values["engine"]),
			Keys:         strings.TrimSpace(in.Values["keys"]),
			GraceSeconds: grace,
			RetireOld:    strings.EqualFold(strings.TrimSpace(in.Values["retire_old"]), "true"),
			DryRun:       strings.EqualFold(strings.TrimSpace(in.Values["dry_run"]), "true"),
			Instruction:  strings.TrimSpace(in.Values["instruction"]),

			Source:            strings.TrimSpace(in.Values["source"]),
			Cluster:           strings.TrimSpace(in.Values["cluster"]),
			ExternalSecrets:   strings.TrimSpace(in.Values["external_secrets"]),
			K8sSecrets:        strings.TrimSpace(in.Values["k8s_secrets"]),
			GitHubRepo:        strings.TrimSpace(in.Values["github_repo"]),
			GitHubEnvironment: strings.TrimSpace(in.Values["github_environment"]),
			GitHubSecretNames: strings.TrimSpace(in.Values["github_secret_names"]),
			RPApp:             strings.TrimSpace(in.Values["rp_app"]),
			RPRings:           strings.TrimSpace(in.Values["rp_rings"]),
			RPDeployments:     strings.TrimSpace(in.Values["rp_deployments"]),
		}
		created, err := run.CreateRun(ctx, payload, in.OrgID, &in.UserID)
		if err != nil {
			return nil, err
		}
		id := created.ID
		return &agentmodule.LaunchOutput{
			RunID:     &id,
			Message:   "Secrets Rotation run queued",
			ExtraMeta: map[string]any{model.TaskMetaRunID: id.String()},
		}, nil
	}
}
