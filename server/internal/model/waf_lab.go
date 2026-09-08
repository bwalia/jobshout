package model

import (
	"time"

	"github.com/google/uuid"
)

const AgentNameWAFLab = "WAF Efficacy Lab"

// CreateWAFLabRunRequest is the launch payload for a WAF Efficacy Lab run.
type CreateWAFLabRunRequest struct {
	AgentID         uuid.UUID  `json:"agent_id" validate:"required"`
	TaskID          *uuid.UUID `json:"task_id"`
	WSLProxyBaseURL string     `json:"wslproxy_base_url" validate:"required"`
	SecureHost      string     `json:"secure_host" validate:"required"`
	OpenHost        string     `json:"open_host" validate:"required"`
	OriginUpstream  string     `json:"origin_upstream"`
	PolicyID        string     `json:"policy_id"`
	Mode            string     `json:"mode" validate:"required,oneof=provision_and_test provision_only test_only"`
	ManageDNS       string     `json:"manage_dns" validate:"omitempty,oneof=off cloudflare"`
	DNSZone         string     `json:"dns_zone"`
	AttackSet       string     `json:"attack_set" validate:"omitempty,oneof=full owasp_core modern_api stages_only"`
	Instruction     string     `json:"instruction" validate:"omitempty,max=4000"`
}

// WAFLabRun is one efficacy-lab execution.
type WAFLabRun struct {
	ID              uuid.UUID  `json:"id"`
	AgentID         uuid.UUID  `json:"agent_id"`
	TaskID          *uuid.UUID `json:"task_id"`
	OrgID           uuid.UUID  `json:"org_id"`
	Status          string     `json:"status"` // queued, running, completed, failed, cancelled
	Mode            string     `json:"mode"`
	SecureHost      string     `json:"secure_host"`
	OpenHost        string     `json:"open_host"`
	OriginUpstream  string     `json:"origin_upstream"`
	PolicyID        string     `json:"policy_id"`
	ManageDNS       string     `json:"manage_dns"`
	DNSZone         string     `json:"dns_zone"`
	AttackSet       string     `json:"attack_set"`
	Instruction     *string    `json:"instruction,omitempty"`
	WSLProxyBaseURL string     `json:"wslproxy_base_url"`
	Score           *WAFLabScore `json:"score,omitempty"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
	RequestedBy     *uuid.UUID `json:"requested_by"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// WAFLabStep is one phase of a lab run.
type WAFLabStep struct {
	ID        uuid.UUID  `json:"id"`
	RunID     uuid.UUID  `json:"run_id"`
	Phase     string     `json:"phase"` // preflight, rules_policy, hosts, dns, efficacy
	Status    string     `json:"status"` // pending, running, completed, failed, skipped
	Message   string     `json:"message"`
	StartedAt *time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// WAFLabResult is one attack × host outcome.
type WAFLabResult struct {
	ID           uuid.UUID `json:"id"`
	RunID        uuid.UUID `json:"run_id"`
	HostRole     string    `json:"host_role"` // secure | open
	Host         string    `json:"host"`
	AttackID     string    `json:"attack_id"`
	AttackName   string    `json:"attack_name"`
	Category     string    `json:"category"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	Payload      string    `json:"payload,omitempty"`
	StatusCode   int       `json:"status_code"`
	Blocked      bool      `json:"blocked"`
	ExpectBlock  bool      `json:"expect_block"`
	WAFRule      string    `json:"waf_rule,omitempty"`
	WAFViolation string    `json:"waf_violation,omitempty"`
	SupportID    string    `json:"support_id,omitempty"`
	LatencyMS    int       `json:"latency_ms"`
	Verdict      string    `json:"verdict"` // blocked, leaked, false_positive, not_blocked_expected, error
	Notes        string    `json:"notes,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// WAFLabScore summarises a before/after matrix.
type WAFLabScore struct {
	AttacksTotal       int            `json:"attacks_total"`
	SecureBlocked      int            `json:"secure_blocked"`
	SecureExpected     int            `json:"secure_expected"`
	OpenLeaked         int            `json:"open_leaked"`
	OpenExpectedLeak   int            `json:"open_expected_leak"`
	FalsePositives     int            `json:"false_positives"`
	NotBlockedExpected int            `json:"not_blocked_expected"` // e.g. BOLA
	ByCategory         map[string]WAFLabCategoryScore `json:"by_category"`
	Suspicious         bool           `json:"suspicious"`
	SuspiciousReason   string         `json:"suspicious_reason,omitempty"`
	BOLANote           string         `json:"bola_note,omitempty"`
}

// WAFLabCategoryScore is per-category coverage.
type WAFLabCategoryScore struct {
	Total          int `json:"total"`
	SecureBlocked  int `json:"secure_blocked"`
	OpenLeaked     int `json:"open_leaked"`
	FalsePositives int `json:"false_positives"`
}
