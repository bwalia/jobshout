package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	BuiltinSecretsRotation   = "secrets_rotation"
	AgentNameSecretsRotation = "Secrets Rotation"
)

// CreateSecretsRotationRequest is the launch payload.
type CreateSecretsRotationRequest struct {
	AgentID       uuid.UUID  `json:"agent_id" validate:"required"`
	TaskID        *uuid.UUID `json:"task_id"`
	Mode          string     `json:"mode" validate:"required,oneof=plan rotate verify rollback"`
	Provider      string     `json:"provider" validate:"omitempty,oneof=auto wslvault hashicorp"`
	VaultAddr     string     `json:"vault_addr"` // override; else env / default WSLVault
	Mount         string     `json:"mount"`      // KV/transit/database mount, default secret
	Path          string     `json:"path" validate:"required"`
	Engine        string     `json:"engine" validate:"omitempty,oneof=kv2 transit database"`
	Keys          string     `json:"keys"`            // comma-separated KV keys to rotate (empty = all string fields)
	GraceSeconds  int        `json:"grace_seconds"`   // dual-window before soft-delete; default 300
	RetireOld     bool       `json:"retire_old"`      // soft-delete previous version after grace
	DryRun        bool       `json:"dry_run"`         // plan-like execute without writes
	NewSecretJSON string     `json:"new_secret_json"` // optional explicit new values (never logged)
	Instruction   string     `json:"instruction" validate:"omitempty,max=4000"`

	// Propagation targets. Names only — credentials are server-side and named
	// (kubeconfig by cluster name, SECRETS_ROT_GITHUB_TOKEN, RP_API_TOKEN).
	Source            string `json:"source" validate:"omitempty,oneof=vault github"`
	Cluster           string `json:"cluster" validate:"omitempty,max=63"`
	ExternalSecrets   string `json:"external_secrets" validate:"omitempty,max=2000"`
	K8sSecrets        string `json:"k8s_secrets" validate:"omitempty,max=2000"`
	GitHubRepo        string `json:"github_repo" validate:"omitempty,max=200"`
	GitHubEnvironment string `json:"github_environment" validate:"omitempty,max=255"`
	GitHubSecretNames string `json:"github_secret_names" validate:"omitempty,max=2000"`
	RPApp             string `json:"rp_app" validate:"omitempty,max=100"`
	RPRings           string `json:"rp_rings" validate:"omitempty,max=200"`
	RPDeployments     string `json:"rp_deployments" validate:"omitempty,max=1000"`
}

// SecretsRotationRun is one plan / rotate / verify / rollback execution.
type SecretsRotationRun struct {
	ID           uuid.UUID  `json:"id"`
	AgentID      uuid.UUID  `json:"agent_id"`
	TaskID       *uuid.UUID `json:"task_id"`
	OrgID        uuid.UUID  `json:"org_id"`
	Status       string     `json:"status"` // queued, running, completed, failed, cancelled
	Mode         string     `json:"mode"`
	Provider     string     `json:"provider"`
	DetectedProv string     `json:"detected_provider,omitempty"`
	VaultAddr    string     `json:"vault_addr"`
	Mount        string     `json:"mount"`
	Path         string     `json:"path"`
	Engine       string     `json:"engine"`
	Keys         string     `json:"keys,omitempty"`
	GraceSeconds int        `json:"grace_seconds"`
	RetireOld    bool       `json:"retire_old"`
	DryRun       bool       `json:"dry_run"`
	Instruction  *string    `json:"instruction,omitempty"`

	Source            string `json:"source"`
	Cluster           string `json:"cluster,omitempty"`
	ExternalSecrets   string `json:"external_secrets,omitempty"`
	K8sSecrets        string `json:"k8s_secrets,omitempty"`
	GitHubRepo        string `json:"github_repo,omitempty"`
	GitHubEnvironment string `json:"github_environment,omitempty"`
	GitHubSecretNames string `json:"github_secret_names,omitempty"`
	RPApp             string `json:"rp_app,omitempty"`
	RPRings           string `json:"rp_rings,omitempty"`
	RPDeployments     string `json:"rp_deployments,omitempty"`

	Phases       []SecretsRotationPhase `json:"phases,omitempty"`
	Result       *SecretsRotationResult `json:"result,omitempty"`
	ErrorMessage *string                `json:"error_message,omitempty"`
	RequestedBy  *uuid.UUID             `json:"requested_by"`
	StartedAt    *time.Time             `json:"started_at"`
	CompletedAt  *time.Time             `json:"completed_at"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// SecretsRotationPhase is one zero-downtime step.
type SecretsRotationPhase struct {
	Key       string     `json:"key"`
	Label     string     `json:"label"`
	Status    string     `json:"status"` // pending, active, completed, failed, skipped
	Message   string     `json:"message,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// SecretsRotationResult summarises the run without secret values.
type SecretsRotationResult struct {
	UILoginURL        string   `json:"ui_login_url,omitempty"`
	DocsURL           string   `json:"docs_url,omitempty"`
	Mount             string   `json:"mount"`
	Path              string   `json:"path"`
	Engine            string   `json:"engine"`
	PreviousVersion   int      `json:"previous_version,omitempty"`
	CurrentVersion    int      `json:"current_version,omitempty"`
	RolledBackTo      int      `json:"rolled_back_to,omitempty"`
	KeysRotated       []string `json:"keys_rotated,omitempty"`
	DualWindowSec     int      `json:"dual_window_seconds,omitempty"`
	OldVersionRetired bool     `json:"old_version_retired"`
	ZeroDowntime      bool     `json:"zero_downtime"`
	Strategy          string   `json:"strategy,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
	PlanSteps         []string `json:"plan_steps,omitempty"`

	Propagation *SecretsRotationPropagation `json:"propagation,omitempty"`
}

// SecretsRotationPropagation records where rotated values were pushed. It
// holds names, statuses, resourceVersions and job ids — never values.
type SecretsRotationPropagation struct {
	Source       string                  `json:"source"`
	Cluster      string                  `json:"cluster,omitempty"`
	VaultVersion int                     `json:"vault_version,omitempty"`
	Targets      []SecretsRotationTarget `json:"targets,omitempty"`
}

// SecretsRotationTarget is one propagation destination.
type SecretsRotationTarget struct {
	Kind            string `json:"kind"`   // k8s_secret, external_secret, github_secret, ring
	Name            string `json:"name"`   // ns/name, owner/repo[@env]/SECRET, app/ring
	Status          string `json:"status"` // pending, completed, failed, dry_run, skipped
	ResourceVersion string `json:"resource_version,omitempty"`
	JobID           string `json:"job_id,omitempty"`
	Message         string `json:"message,omitempty"`
}
