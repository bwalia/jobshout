package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	BuiltinMarketingEmailWriter = "marketing_email_writer"
	AgentNameMarketingEmail     = "Marketing Email Writer"
)

// Marketing email run statuses. writing → publishing → published|draft; any of
// the first two can end in failed. A published or draft page can later be
// archived (unpublished).
const (
	MarketingEmailStatusWriting    = "writing"
	MarketingEmailStatusPublishing = "publishing"
	MarketingEmailStatusPublished  = "published"
	MarketingEmailStatusDraft      = "draft"
	MarketingEmailStatusFailed     = "failed"
	MarketingEmailStatusArchived   = "archived"
)

// Mail-merge template outcomes.
const (
	MarketingTemplateSaved    = "saved"
	MarketingTemplateNotSaved = "not_saved"
	MarketingTemplateDisabled = "disabled"
)

// Publish modes: live files the CMS page as published at once; draft files it
// as a draft.
const (
	MarketingEmailPublishLive  = "live"
	MarketingEmailPublishDraft = "draft"
)

// Email goals offered in the launch form.
const (
	MarketingEmailGoalAnnounce   = "announce"
	MarketingEmailGoalPromote    = "promote"
	MarketingEmailGoalNurture    = "nurture"
	MarketingEmailGoalNewsletter = "newsletter"
)

// MarketingEmailBrief is what a launch asked for. It carries no credentials;
// the destination is a website name resolved server-side.
type MarketingEmailBrief struct {
	Brief           string `json:"brief"`
	Audience        string `json:"audience,omitempty"`
	Goal            string `json:"goal,omitempty"`
	PrimaryCTALabel string `json:"primary_cta_label,omitempty"`
	PrimaryCTAURL   string `json:"primary_cta_url,omitempty"`
	Products        string `json:"products,omitempty"`
	Tone            string `json:"tone,omitempty"`
	Destination     string `json:"destination,omitempty"`
	Publish         string `json:"publish,omitempty"`
	Model           string `json:"model,omitempty"`
}

// Email section types the model may use.
const (
	EmailSectionText    = "text"
	EmailSectionFeature = "feature"
	EmailSectionProduct = "product"
	EmailSectionQuote   = "quote"
	EmailSectionCTA     = "cta"
)

// MarketingEmailHero is the opening block of the email.
type MarketingEmailHero struct {
	Headline    string `json:"headline"`
	Subline     string `json:"subline,omitempty"`
	ImagePrompt string `json:"image_prompt,omitempty"`
}

// MarketingEmailSection is one block of the email body. The model never writes
// HTML: BodyMarkdown goes through an allowlist renderer.
type MarketingEmailSection struct {
	Type         string   `json:"type"`
	Heading      string   `json:"heading,omitempty"`
	BodyMarkdown string   `json:"body_markdown,omitempty"`
	Items        []string `json:"items,omitempty"`
	CTALabel     string   `json:"cta_label,omitempty"`
	CTAURL       string   `json:"cta_url,omitempty"`
}

// MarketingEmailDraft is the structured output of the writing step.
type MarketingEmailDraft struct {
	Subject    string                  `json:"subject"`
	Preheader  string                  `json:"preheader"`
	Title      string                  `json:"title"`
	SlugHint   string                  `json:"slug_hint,omitempty"`
	Hero       MarketingEmailHero      `json:"hero"`
	Sections   []MarketingEmailSection `json:"sections"`
	FooterNote string                  `json:"footer_note,omitempty"`
}

// MarketingEmailRun is one Marketing Email Writer launch and where it has got
// to. PageUUID/Slug/BrowserURL say where the CMS page lives once filed.
type MarketingEmailRun struct {
	ID           uuid.UUID           `json:"id"`
	OrgID        uuid.UUID           `json:"org_id"`
	TaskID       *uuid.UUID          `json:"task_id,omitempty"`
	AgentID      *uuid.UUID          `json:"agent_id,omitempty"`
	RequestedBy  *uuid.UUID          `json:"requested_by,omitempty"`
	Brief        MarketingEmailBrief `json:"brief"`
	Subject      string              `json:"subject"`
	Preheader    string              `json:"preheader"`
	Title        string              `json:"title"`
	Slug         string              `json:"slug"`
	Status       string              `json:"status"`
	Stage        string              `json:"stage"`
	Message      string              `json:"message"`
	Error        string              `json:"error,omitempty"`
	ContentHTML  string              `json:"content_html,omitempty"`
	Draft        MarketingEmailDraft `json:"draft"`
	HeroImageURL string              `json:"hero_image_url,omitempty"`
	PageUUID     string              `json:"page_uuid,omitempty"`
	PageStatus   string              `json:"page_status,omitempty"`
	BrowserURL   string              `json:"browser_url,omitempty"`
	// The OpsAPI Marketing mail-merge template saved from the same email
	// (real merge fields kept). TemplateOutcome is saved, not_saved or
	// disabled; TemplateMessage says why when it is not saved.
	TemplateUUID    string     `json:"template_uuid,omitempty"`
	TemplateName    string     `json:"template_name,omitempty"`
	TemplateOutcome string     `json:"template_outcome,omitempty"`
	TemplateMessage string     `json:"template_message,omitempty"`
	TemplateHTML    string     `json:"template_html,omitempty"`
	Destination     string     `json:"destination,omitempty"`
	Model           string     `json:"model,omitempty"`
	PublishMode     string     `json:"publish_mode"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Unfinished reports whether the run can still change by itself.
func (r MarketingEmailRun) Unfinished() bool {
	return r.Status == MarketingEmailStatusWriting || r.Status == MarketingEmailStatusPublishing
}
