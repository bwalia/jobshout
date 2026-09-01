package model

import (
	"time"

	"github.com/google/uuid"
)

// Mail Agent thread / draft / connection vocabularies. Keep in step with the
// CHECKs in migration 000033.
const (
	MailConnDisconnected = "disconnected"
	MailConnConnected    = "connected"
	MailConnError        = "error"

	MailThreadNew         = "new"
	MailThreadClassifying = "classifying"
	MailThreadResearching = "researching"
	MailThreadDraftReady  = "draft_ready"
	MailThreadSent        = "sent"
	MailThreadRejected    = "rejected"
	MailThreadIgnored     = "ignored"
	MailThreadFailed      = "failed"

	MailDraftDraft    = "draft"
	MailDraftApproved = "approved"
	MailDraftSent     = "sent"
	MailDraftRejected = "rejected"

	AgentNameMail = "Mail Agent"
)

// MailConnection is the org's one shared Gmail mailbox. The refresh token is
// ciphertext only — never serialise RefreshTokenEnc on an API response.
type MailConnection struct {
	ID                    uuid.UUID  `json:"id"`
	OrgID                 uuid.UUID  `json:"org_id"`
	AgentID               *uuid.UUID `json:"agent_id,omitempty"`
	GoogleEmail           string     `json:"google_email"`
	RefreshTokenEnc       []byte     `json:"-"`
	TokenExpiry           *time.Time `json:"-"`
	Scopes                []string   `json:"scopes"`
	AllowMailboxMutations bool       `json:"allow_mailbox_mutations"`
	WatchLabels           []string   `json:"watch_labels"`
	WatchSenders          []string   `json:"watch_senders"`
	WatchSubjectPrefixes  []string   `json:"watch_subject_prefixes"`
	WatchKnowledgeURLs    []string   `json:"knowledge_urls"`
	KnowledgeNotes        string     `json:"knowledge_notes"`
	ResearchFocus         string     `json:"research_focus"`
	ReplyInstructions     string     `json:"reply_instructions"`
	Status                string     `json:"status"`
	StatusError           *string    `json:"status_error,omitempty"`
	LastSyncAt            *time.Time `json:"last_sync_at,omitempty"`
	NextSyncAt            *time.Time `json:"next_sync_at,omitempty"`
	SyncLeaseUntil        *time.Time `json:"-"`
	ConnectedAt           *time.Time `json:"connected_at,omitempty"`
	DisconnectedAt        *time.Time `json:"disconnected_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// MailWatchRules is the v1 filter set: only these labels / senders / subject
// prefixes are watched. Empty slices mean "unread INBOX, last 7 days".
type MailWatchRules struct {
	Labels          []string `json:"labels"`
	Senders         []string `json:"senders"`
	SubjectPrefixes []string `json:"subject_prefixes"`
}

// MailScopeDoc explains one OAuth scope in the UI. The values must match what
// StartOAuth actually requests.
type MailScopeDoc struct {
	Scope string `json:"scope"`
	Why   string `json:"why"`
}

// MailConnectionStatus is the safe API view of a connection: no tokens.
type MailConnectionStatus struct {
	Configured            bool           `json:"configured"`
	Connected             bool           `json:"connected"`
	Email                 string         `json:"email,omitempty"`
	Status                string         `json:"status"`
	StatusError           string         `json:"status_error,omitempty"`
	AllowMailboxMutations bool           `json:"allow_mailbox_mutations"`
	Rules                 MailWatchRules `json:"rules"`
	KnowledgeURLs         []string       `json:"knowledge_urls"`
	KnowledgeNotes        string         `json:"knowledge_notes"`
	ResearchFocus         string         `json:"research_focus"`
	ReplyInstructions     string         `json:"reply_instructions"`
	Scopes                []string       `json:"scopes,omitempty"`
	ScopesDocumented      []MailScopeDoc `json:"scopes_documented"`
	LastSyncAt            *time.Time     `json:"last_sync_at,omitempty"`
	ConnectedAt           *time.Time     `json:"connected_at,omitempty"`
	AgentID               *uuid.UUID     `json:"agent_id,omitempty"`
}

// MailClassification is the structured triage the Mail Agent stores per thread.
type MailClassification struct {
	Intent          string `json:"intent"`
	NeedsResearch   bool   `json:"needs_research"`
	Urgency         string `json:"urgency"`
	SuggestedAction string `json:"suggested_action"`
	Reason          string `json:"reason"`
	TriageLabel     string `json:"triage_label"`
}

// MailThread is a watched Gmail conversation snapshot.
type MailThread struct {
	ID               uuid.UUID           `json:"id"`
	OrgID            uuid.UUID           `json:"org_id"`
	AgentID          *uuid.UUID          `json:"agent_id,omitempty"`
	ConnectionID     uuid.UUID           `json:"connection_id"`
	GmailThreadID    string              `json:"gmail_thread_id"`
	GmailMessageID   string              `json:"gmail_message_id"`
	FromEmail        string              `json:"from_email"`
	FromName         string              `json:"from_name"`
	ToEmail          string              `json:"to_email"`
	Subject          string              `json:"subject"`
	Snippet          string              `json:"snippet"`
	BodyText         string              `json:"body_text"`
	MessageIDHeader  string              `json:"message_id_header,omitempty"`
	ReferencesHeader string              `json:"references_header,omitempty"`
	ReceivedAt       *time.Time          `json:"received_at,omitempty"`
	Status           string              `json:"status"`
	Classification   *MailClassification `json:"classification,omitempty"`
	NeedsResearch    bool                `json:"needs_research"`
	ResearchSummary  *string             `json:"research_summary,omitempty"`
	ResearchFindings []byte              `json:"-"`
	ResearchBriefID  *uuid.UUID          `json:"research_brief_id,omitempty"`
	ErrorMessage     *string             `json:"error_message,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

// MailDraft is a JobShout-side reply. Status draft/approved/sent/rejected.
type MailDraft struct {
	ID              uuid.UUID  `json:"id"`
	OrgID           uuid.UUID  `json:"org_id"`
	ThreadID        uuid.UUID  `json:"thread_id"`
	AgentID         *uuid.UUID `json:"agent_id,omitempty"`
	Status          string     `json:"status"`
	Subject         string     `json:"subject"`
	Body            string     `json:"body"`
	ToEmail         string     `json:"to_email"`
	CCEmail         string     `json:"cc_email,omitempty"`
	ResearchBriefID *uuid.UUID `json:"research_brief_id,omitempty"`
	ApprovedBy      *uuid.UUID `json:"approved_by,omitempty"`
	ApprovedAt      *time.Time `json:"approved_at,omitempty"`
	RejectedBy      *uuid.UUID `json:"rejected_by,omitempty"`
	RejectedAt      *time.Time `json:"rejected_at,omitempty"`
	GmailMessageID  *string    `json:"gmail_message_id,omitempty"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// MailThreadDetail is GET /threads/{id}: original, classification, research, draft.
type MailThreadDetail struct {
	Thread MailThread `json:"thread"`
	Draft  *MailDraft `json:"draft,omitempty"`
}

// MailOAuthStartResponse is POST /connection/oauth/start.
type MailOAuthStartResponse struct {
	AuthorizationURL string `json:"authorization_url"`
}

// UpdateMailDraftRequest is PATCH /drafts/{id}.
type UpdateMailDraftRequest struct {
	Body    *string `json:"body"`
	Subject *string `json:"subject"`
	CCEmail *string `json:"cc_email"`
}

// UpdateMailConnectionRequest is PATCH /connection (rules + knowledge playbook).
type UpdateMailConnectionRequest struct {
	AllowMailboxMutations *bool           `json:"allow_mailbox_mutations"`
	Rules                 *MailWatchRules `json:"rules"`
	KnowledgeURLs         *[]string       `json:"knowledge_urls"`
	KnowledgeNotes        *string         `json:"knowledge_notes"`
	ResearchFocus         *string         `json:"research_focus"`
	ReplyInstructions     *string         `json:"reply_instructions"`
}
