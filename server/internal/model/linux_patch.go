package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	BuiltinLinuxPatch   = "linux_patch"
	AgentNameLinuxPatch = "Linux Patch"
)

// CreateLinuxPatchRunRequest is the launch payload.
type CreateLinuxPatchRunRequest struct {
	AgentID        uuid.UUID  `json:"agent_id" validate:"required"`
	TaskID         *uuid.UUID `json:"task_id"`
	Mode           string     `json:"mode" validate:"required,oneof=plan patch verify"`
	Workload       string     `json:"workload" validate:"omitempty,oneof=generic couchbase hashicorp_vault wslvault"`
	Hosts          string     `json:"hosts" validate:"required"` // host or user@host:port, comma/newline
	SSHUser        string     `json:"ssh_user"`
	PreScript      string     `json:"pre_script"`
	PostScript     string     `json:"post_script"`
	Services       string     `json:"services"` // comma-separated systemd units to ensure running after
	RebootPolicy   string     `json:"reboot_policy" validate:"omitempty,oneof=never if_needed always"`
	DryRun         bool       `json:"dry_run"`
	UseLLM         bool       `json:"use_llm"`
	ScheduleAfter  string     `json:"schedule_after"` // RFC3339 optional advisory
	VaultMount     string     `json:"vault_mount"`    // KV mount for SSH key (default secret)
	VaultPath      string     `json:"vault_path"`     // e.g. ops/ssh/patch-fleet
	VaultKeyField  string     `json:"vault_key_field"` // default ssh_private_key
	Instruction    string     `json:"instruction" validate:"omitempty,max=4000"`
}

// LinuxPatchRun is one plan/patch/verify execution across hosts.
type LinuxPatchRun struct {
	ID            uuid.UUID              `json:"id"`
	AgentID       uuid.UUID              `json:"agent_id"`
	TaskID        *uuid.UUID             `json:"task_id"`
	OrgID         uuid.UUID              `json:"org_id"`
	Status        string                 `json:"status"`
	Mode          string                 `json:"mode"`
	Workload      string                 `json:"workload"`
	Hosts         string                 `json:"hosts"`
	SSHUser       string                 `json:"ssh_user,omitempty"`
	PreScript     string                 `json:"pre_script,omitempty"`
	PostScript    string                 `json:"post_script,omitempty"`
	Services      string                 `json:"services,omitempty"`
	RebootPolicy  string                 `json:"reboot_policy"`
	DryRun        bool                   `json:"dry_run"`
	UseLLM        bool                   `json:"use_llm"`
	ScheduleAfter *string                `json:"schedule_after,omitempty"`
	VaultMount    string                 `json:"vault_mount,omitempty"`
	VaultPath     string                 `json:"vault_path,omitempty"`
	VaultKeyField string                 `json:"vault_key_field,omitempty"`
	Instruction   *string                `json:"instruction,omitempty"`
	Phases        []LinuxPatchPhase      `json:"phases,omitempty"`
	HostResults   []LinuxPatchHostResult `json:"host_results,omitempty"`
	Plan          *LinuxPatchPlan        `json:"plan,omitempty"`
	ErrorMessage  *string                `json:"error_message,omitempty"`
	RequestedBy   *uuid.UUID             `json:"requested_by"`
	StartedAt     *time.Time             `json:"started_at"`
	CompletedAt   *time.Time             `json:"completed_at"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// LinuxPatchPhase is a run-level step.
type LinuxPatchPhase struct {
	Key       string     `json:"key"`
	Label     string     `json:"label"`
	Status    string     `json:"status"`
	Message   string     `json:"message,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// LinuxPatchHostResult is per-host outcome (no secrets).
type LinuxPatchHostResult struct {
	Host       string   `json:"host"`
	OSFlavor   string   `json:"os_flavor,omitempty"`
	Status     string   `json:"status"`
	Rebooted   bool     `json:"rebooted"`
	Packages   []string `json:"packages,omitempty"`
	ServicesOK []string `json:"services_ok,omitempty"`
	ServicesFix []string `json:"services_fixed,omitempty"`
	Messages   []string `json:"messages,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// LinuxPatchPlan is the LLM/heuristic rolling plan.
type LinuxPatchPlan struct {
	Strategy       string   `json:"strategy"`
	ZeroDowntime   bool     `json:"zero_downtime"`
	Order          []string `json:"order"` // hosts in patch order
	Steps          []string `json:"steps"`
	Warnings       []string `json:"warnings,omitempty"`
	LLMSummary     string   `json:"llm_summary,omitempty"`
	WorkloadHints  []string `json:"workload_hints,omitempty"`
	DocsURL        string   `json:"docs_url,omitempty"`
}
