package model

import (
	"time"

	"github.com/google/uuid"
)

// The LinkedIn Poster turns published articles into LinkedIn posts. Each
// article gets two drafts — one for engineers, one for a business reader — and
// nothing reaches LinkedIn until a person approves a draft on the agent's tab.
const (
	BuiltinLinkedIn   = "linkedin_poster"
	AgentNameLinkedIn = "LinkedIn Poster"
)

// LinkedIn post variants: who the draft is written for.
const (
	LinkedInVariantTechnical = "technical"
	LinkedInVariantBusiness  = "business"
)

// LinkedInVariants is every variant, in the order a run drafts them.
var LinkedInVariants = []string{LinkedInVariantTechnical, LinkedInVariantBusiness}

// LinkedIn post statuses.
//
//	drafting → draft → posting → posted
//	    ↘ failed ↙         ↘ draft (post rejected; the error says why)
//
// posting is held while the LinkedIn call is in flight, so a double click or a
// second replica cannot post the same draft twice.
const (
	LinkedInPostDrafting = "drafting"
	LinkedInPostDraft    = "draft"
	LinkedInPostPosting  = "posting"
	LinkedInPostPosted   = "posted"
	LinkedInPostFailed   = "failed"
)

// LinkedInCommentaryMax is LinkedIn's limit on a post's text.
const LinkedInCommentaryMax = 3000

// LinkedInPost is one drafted (and perhaps posted) LinkedIn post for one
// article and variant.
type LinkedInPost struct {
	ID           uuid.UUID  `json:"id"`
	OrgID        uuid.UUID  `json:"org_id"`
	ArticleID    uuid.UUID  `json:"article_id"`
	ArticleTitle string     `json:"article_title"`
	TaskID       *uuid.UUID `json:"task_id,omitempty"`
	Variant      string     `json:"variant"`
	Status       string     `json:"status"`
	// Commentary is the post text, editable while the post is a draft.
	Commentary string `json:"commentary"`
	// LinkURL is the article's public URL, attached as a link card. Empty
	// when the article has no public URL yet; the post then goes out as text.
	LinkURL string `json:"link_url"`
	// Notes is the angle the person asked for when launching, kept so a
	// redraft writes to the same brief.
	Notes        string     `json:"notes,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	PostURN      string     `json:"post_urn,omitempty"`
	PostURL      string     `json:"post_url,omitempty"`
	PostedAt     *time.Time `json:"posted_at,omitempty"`
	PostedBy     *uuid.UUID `json:"posted_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// LinkedInConnection is the LinkedIn member an org posts as. The access token
// is never part of it.
type LinkedInConnection struct {
	OrgID       uuid.UUID  `json:"org_id"`
	MemberURN   string     `json:"member_urn"`
	Name        string     `json:"name"`
	ConnectedBy *uuid.UUID `json:"connected_by,omitempty"`
	ExpiresAt   time.Time  `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// LinkedInStatus is what the agent's tab shows about the connection.
type LinkedInStatus struct {
	// Configured is false when the deployment has no LinkedIn app
	// (LINKEDIN_CLIENT_ID / LINKEDIN_CLIENT_SECRET / LINKEDIN_TOKEN_KEY).
	Configured bool `json:"configured"`
	Connected  bool `json:"connected"`
	// Expired: LinkedIn issues members a 60-day token with no refresh, so the
	// connection has to be renewed by connecting again.
	Expired   bool       `json:"expired"`
	Name      string     `json:"name,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	DaysLeft  int        `json:"days_left"`
	AutoDraft bool       `json:"auto_draft"`
}

// LinkedInArticle is the slice of a published article a post is drafted from.
type LinkedInArticle struct {
	ID       uuid.UUID
	OrgID    uuid.UUID
	Title    string
	Markdown string
	Audience string
	// InsightsSlug is set when the article is on jobshout.com Insights; with
	// the site URL it is the article's public URL.
	InsightsSlug string
	PublishedAt  *time.Time
}
