package mail

import (
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// ClassifyResult is the typed triage handoff. Alias of the stored JSON shape
// so the pipeline and the API cannot drift.
type ClassifyResult = model.MailClassification

// Draft is the typed reply the pipeline produces before it is persisted.
// Status is one of model.MailDraft*.
type Draft struct {
	ThreadID        string
	Body            string
	Subject         string
	To              string
	CC              string
	ResearchBriefID *uuid.UUID
	Status          string
}

// InboxMessage is one inbound Gmail message the monitor snapped for classify/draft.
type InboxMessage struct {
	GmailThreadID    string
	GmailMessageID   string
	FromEmail        string
	FromName         string
	ToEmail          string
	Subject          string
	Snippet          string
	Body             string
	MessageIDHeader  string
	ReferencesHeader string
	ReceivedAt       time.Time
}

// OutboundMessage is what Gmail send accepts after a human approves a draft.
type OutboundMessage struct {
	From       string
	To         string
	CC         string
	Subject    string
	Body       string
	InReplyTo  string
	References string
	ThreadID   string
}

// TokenSet is an OAuth token response. Never log these values.
type TokenSet struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}

// Scopes requested for the org mailbox. Minimal set for v1:
//
//	gmail.readonly  — list and read threads (monitor)
//	gmail.send      — send a message, only after human approve
//	userinfo.email  — show the connected account address in the UI
//
// Drafts are stored in JobShout, not Gmail, so gmail.compose is not requested.
const (
	ScopeGmailReadonly = "https://www.googleapis.com/auth/gmail.readonly"
	ScopeGmailSend     = "https://www.googleapis.com/auth/gmail.send"
	ScopeUserInfoEmail = "https://www.googleapis.com/auth/userinfo.email"
)

// RequestedScopes is the exact list passed to Google. Keep in step with ScopeDocs.
func RequestedScopes() []string {
	return []string{ScopeGmailReadonly, ScopeGmailSend, ScopeUserInfoEmail}
}

// ScopeDocs is what the UI shows next to "Connect Gmail".
func ScopeDocs() []model.MailScopeDoc {
	return []model.MailScopeDoc{
		{Scope: ScopeGmailReadonly, Why: "Read inbox threads so the Mail Agent can draft replies."},
		{Scope: ScopeGmailSend, Why: "Send a reply only after a human clicks Approve."},
		{Scope: ScopeUserInfoEmail, Why: "Show which Google account is connected."},
	}
}
