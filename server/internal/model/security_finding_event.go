package model

import (
	"time"

	"github.com/google/uuid"
)

// SecurityFindingEvent records detect / still_open / fixed across versioned reports.
type SecurityFindingEvent struct {
	ID         uuid.UUID `json:"id"`
	OrgID      uuid.UUID `json:"org_id"`
	AgentKind  string    `json:"agent_kind"` // pentest | waf_lab
	SubjectKey string    `json:"subject_key"`
	FindingKey string    `json:"finding_key"`
	Event      string    `json:"event"` // detected | still_open | fixed
	RunID      uuid.UUID `json:"run_id"`
	ReportSeq  *int      `json:"report_seq,omitempty"`
	AppVersion *string   `json:"app_version,omitempty"`
	Severity   string    `json:"severity,omitempty"`
	Title      string    `json:"title"`
	Detail     string    `json:"detail,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
