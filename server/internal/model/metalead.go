package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Meta Lead Analyst pulls Instagram/Facebook Lead Ads Instant Form
// submissions via Graph API and runs LLM analysis (per-lead briefs + rollup).
const (
	BuiltinMetaLead   = "meta_lead_analyst"
	AgentNameMetaLead = "Meta Lead Analyst"
)

// Meta lead / run statuses.
const (
	MetaLeadStatusPending    = "pending"
	MetaLeadStatusAnalysing  = "analysing"
	MetaLeadStatusAnalysed   = "analysed"
	MetaLeadStatusFailed     = "failed"
	MetaLeadRunStatusRunning = "running"
	MetaLeadRunStatusDone    = "completed"
	MetaLeadRunStatusFailed  = "failed"
)

// Meta lead sources: a Lead Ads form, or an organic interaction on Facebook or
// Instagram.
const (
	MetaLeadSourceForm      = "lead_form"
	MetaLeadSourceFBDM      = "facebook_dm"
	MetaLeadSourceFBComment = "facebook_comment"
	MetaLeadSourceFBMention = "facebook_mention"
	MetaLeadSourceIGDM      = "instagram_dm"
	MetaLeadSourceIGComment = "instagram_comment"
	MetaLeadSourceIGMention = "instagram_mention"
	// MetaLeadSourceIGTag is a post the account is photo-tagged in. Meta sends
	// no webhook for those, so only a launch's backfill finds them.
	MetaLeadSourceIGTag = "instagram_tag"
)

// MetaLeadConnection is a named Page connection for an org. The access token
// is never part of the JSON.
type MetaLeadConnection struct {
	ID       uuid.UUID `json:"id"`
	OrgID    uuid.UUID `json:"org_id"`
	Name     string    `json:"name"`
	PageID   string    `json:"page_id"`
	PageName string    `json:"page_name"`
	// IGAccountID is the Page's linked Instagram professional account, which
	// Instagram webhooks name instead of the Page. Empty: Facebook only.
	IGAccountID string     `json:"ig_account_id,omitempty"`
	ConnectedBy *uuid.UUID `json:"connected_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// MetaLeadRun is one launch or webhook batch of lead analysis.
type MetaLeadRun struct {
	ID           uuid.UUID       `json:"id"`
	OrgID        uuid.UUID       `json:"org_id"`
	TaskID       *uuid.UUID      `json:"task_id,omitempty"`
	ConnectionID uuid.UUID       `json:"connection_id"`
	FormID       string          `json:"form_id,omitempty"`
	FormName     string          `json:"form_name,omitempty"`
	Focus        string          `json:"focus,omitempty"`
	Status       string          `json:"status"`
	LeadCount    int             `json:"lead_count"`
	Rollup       json.RawMessage `json:"rollup,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	Source       string          `json:"source"` // launch | webhook
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// MetaLead is one Lead Ads form submission, DM thread, comment or mention,
// plus optional LLM analysis.
type MetaLead struct {
	ID           uuid.UUID  `json:"id"`
	OrgID        uuid.UUID  `json:"org_id"`
	ConnectionID uuid.UUID  `json:"connection_id"`
	RunID        *uuid.UUID `json:"run_id,omitempty"`
	// LeadgenID is the lead's key: the leadgen id for a form, the thread for a
	// DM ("dm:<account>:<sender>"), otherwise the comment, post or media id.
	LeadgenID  string `json:"leadgen_id"`
	Source     string `json:"source"`
	AuthorID   string `json:"author_id,omitempty"`
	AuthorName string `json:"author_name,omitempty"`
	// Body is the interaction's text; for a DM thread, the sender's messages
	// so far, newest last.
	Body         string          `json:"body,omitempty"`
	Permalink    string          `json:"permalink,omitempty"`
	FormID       string          `json:"form_id,omitempty"`
	FieldData    json.RawMessage `json:"field_data,omitempty"`
	CreatedTime  *time.Time      `json:"created_time,omitempty"`
	Analysis     json.RawMessage `json:"analysis,omitempty"`
	Status       string          `json:"status"`
	ErrorMessage string          `json:"error_message,omitempty"`
	// CRM sync into OpsAPI /api/v2/crm/contacts (Marketing Manager workspace).
	CRMContactID string     `json:"crm_contact_id,omitempty"`
	CRMWebsite   string     `json:"crm_website,omitempty"`
	CRMSyncedAt  *time.Time `json:"crm_synced_at,omitempty"`
	CRMError     string     `json:"crm_error,omitempty"`
	// LabelIsLead is a person's verdict, for measuring models; nil: unlabelled.
	LabelIsLead *bool     `json:"label_is_lead,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MetaLeadEval runs the labelled DMs, comments and mentions through each
// chosen model, Runs times each, and scores the verdicts against the labels.
type MetaLeadEval struct {
	ID           uuid.UUID         `json:"id"`
	OrgID        uuid.UUID         `json:"org_id"`
	Status       string            `json:"status"` // running | completed | failed
	Models       []string          `json:"models"` // provider:model
	Runs         int               `json:"runs"`
	CaseCount    int               `json:"case_count"`
	Report       []MetaLeadEvalRow `json:"report,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
	CreatedBy    *uuid.UUID        `json:"created_by,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// MetaLeadEvalRow is one model's score. Accuracy, precision and recall use
// each case's majority verdict; Consistency is the share of cases where every
// run agreed; Grounded is the share of "is a lead" answers whose evidence
// quote is really in the message.
type MetaLeadEvalRow struct {
	Model        string             `json:"model"`
	Error        string             `json:"error,omitempty"`
	Accuracy     float64            `json:"accuracy"`
	Precision    float64            `json:"precision"`
	Recall       float64            `json:"recall"`
	Consistency  float64            `json:"consistency"`
	Grounded     float64            `json:"grounded"`
	Failures     int                `json:"failures"`
	AvgLatencyMS int64              `json:"avg_latency_ms"`
	Misses       []MetaLeadEvalMiss `json:"misses,omitempty"`
}

// MetaLeadEvalMiss is a case the model got wrong, for a person to review.
type MetaLeadEvalMiss struct {
	LeadID uuid.UUID `json:"lead_id"`
	Body   string    `json:"body"`
	Label  bool      `json:"label"`
	Votes  string    `json:"votes"` // e.g. "lead 1/3"
}

// MetaLeadBrief is the per-lead LLM analysis shape. IsLead, Category and
// Evidence are set for DMs, comments and mentions, where most messages are not
// leads; a form submission is one by definition.
type MetaLeadBrief struct {
	IsLead   *bool  `json:"is_lead,omitempty"`
	Category string `json:"category,omitempty"`
	// Evidence is the message's own words that show the intent, checked
	// against the message after the model returns it.
	Evidence   string   `json:"evidence,omitempty"`
	Intent     string   `json:"intent"`
	FitScore   int      `json:"fit_score"`
	NextAction string   `json:"next_action"`
	SpamRisk   string   `json:"spam_risk"`
	Flags      []string `json:"flags"`
	Summary    string   `json:"summary"`
}

// MetaLeadRollup is the run-level LLM summary.
type MetaLeadRollup struct {
	Themes               []string `json:"themes"`
	QualityMix           string   `json:"quality_mix"`
	RecommendedFollowups []string `json:"recommended_followups"`
	LeadCount            int      `json:"lead_count"`
}
