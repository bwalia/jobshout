package model

import (
	"time"

	"github.com/google/uuid"
)

// BlogRunStatus values.
const (
	BlogRunStatusPending   = "pending"
	BlogRunStatusRunning   = "running"
	BlogRunStatusCompleted = "completed"
	BlogRunStatusFailed    = "failed"
)

// Blog step keys. These name the phases of a run in order, and are what the
// agent board maps onto its columns — see BlogStepActivity.
const (
	BlogStepQueued     = "queued"
	BlogStepGenerating = "generating"
	BlogStepConverting = "converting"
	BlogStepGenerated  = "generated"
	BlogStepPublishing = "publishing"
	BlogStepPublished  = "published"
)

// Step statuses. A step is pending until it starts, running while it is the
// current step, then done or failed.
const (
	StepStatusPending = "pending"
	StepStatusRunning = "running"
	StepStatusDone    = "done"
	StepStatusFailed  = "failed"
)

// BlogStep is one entry in a run's progress trace. The shape mirrors PlanStep
// in goal.go so the UI can render both with the same timeline treatment.
type BlogStep struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Status is one of StepStatusPending / Running / Done / Failed.
	Status      string     `json:"status"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Error       *string    `json:"error,omitempty"`
}

// BlogRun records a single invocation of the article pipeline.
type BlogRun struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	AgentID     *uuid.UUID `json:"agent_id"`
	TriggeredBy *uuid.UUID `json:"triggered_by"`
	Source      string     `json:"source"` // api | schedule
	Status      string     `json:"status"`
	Topics      []string   `json:"topics"`
	Model       *string    `json:"model"`
	// CMSNamespace is the opsapi namespace the run's drafts were created in.
	// Nil until the run is published.
	CMSNamespace *string `json:"cms_namespace"`
	// Articles is the lightweight per-article summary. The markdown body lives
	// in blog_articles and is fetched separately so listing runs stays cheap.
	Articles     []BlogRunArticle `json:"articles"`
	Steps        []BlogStep       `json:"steps"`
	ErrorMessage *string          `json:"error_message"`
	StartedAt    *time.Time       `json:"started_at"`
	CompletedAt  *time.Time       `json:"completed_at"`
	PublishedAt  *time.Time       `json:"published_at"`
	CreatedAt    time.Time        `json:"created_at"`
}

// CurrentStep returns the step the run is on, or nil when nothing is running.
// The agent board uses this to label the agent's card.
func (r *BlogRun) CurrentStep() *BlogStep {
	for i := range r.Steps {
		if r.Steps[i].Status == StepStatusRunning {
			return &r.Steps[i]
		}
	}
	return nil
}

// BlogRunArticle is the per-article summary carried on the run itself.
type BlogRunArticle struct {
	ID    uuid.UUID `json:"id"`
	Topic string    `json:"topic"`
	Slug  string    `json:"slug"`
	Path  string    `json:"path"`
	// WordCount lets the list view show article size without loading the body.
	WordCount int `json:"word_count"`
}

// BlogArticle is a generated article: the markdown, the HTML it was converted
// to, and where it ended up in the CMS once published.
type BlogArticle struct {
	ID       uuid.UUID `json:"id"`
	RunID    uuid.UUID `json:"run_id"`
	OrgID    uuid.UUID `json:"org_id"`
	Topic    string    `json:"topic"`
	Slug     string    `json:"slug"`
	Path     string    `json:"path"`
	Markdown string    `json:"markdown"`
	// HTML is what gets sent to the CMS. Stored so the exact published body can
	// be inspected without re-running the conversion.
	HTML string `json:"html"`
	// PostUUID identifies the CMS draft this article was posted as. Nil until
	// the run is published.
	PostUUID   *string    `json:"post_uuid"`
	PostStatus *string    `json:"post_status"`
	PostedAt   *time.Time `json:"posted_at"`
	WordCount  int        `json:"word_count"`
	CreatedAt  time.Time  `json:"created_at"`
}

// BlogArticlePost is the result of posting one article, written back to
// blog_articles after a successful publish.
type BlogArticlePost struct {
	ArticleID uuid.UUID
	PostUUID  string
	Status    string
}

// GenerateBlogRequest is the HTTP request body for POST /api/v1/blogs/generate.
type GenerateBlogRequest struct {
	Topics      []string `json:"topics" validate:"required,min=1"`
	Model       string   `json:"model,omitempty"`
	MaxArticles int      `json:"max_articles,omitempty"`
}
