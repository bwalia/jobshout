package model

import (
	"fmt"
	"strings"
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
	BlogStepQueued = "queued"
	// The writing phases, in order. Each is its own step so a reader watching a
	// run can see the agent research before it writes, and can tell a failure
	// to find sources apart from a failure to draft from them.
	BlogStepResearching = "researching"
	BlogStepOutlining   = "outlining"
	BlogStepGenerating  = "generating"
	BlogStepReviewing   = "reviewing"
	BlogStepRevising    = "revising"
	BlogStepConverting  = "converting"
	BlogStepGenerated   = "generated"
	BlogStepPublishing  = "publishing"
	BlogStepPublished   = "published"
)

// Step statuses. A step is pending until it starts, running while it is the
// current step, then done or failed.
const (
	StepStatusPending = "pending"
	StepStatusRunning = "running"
	StepStatusDone    = "done"
	StepStatusFailed  = "failed"
	// StepStatusSkipped marks a step that was never needed. Revision is the
	// case: a draft the reviewer had no complaints about is never revised, and
	// leaving that step "pending" on a finished run reads as work that stalled
	// rather than work that turned out to be unnecessary.
	StepStatusSkipped = "skipped"
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

// BlogBrief is one article's instructions: what to write about, and the
// guidance that shapes how.
//
// The topic is a subject, not a title — the agent researches the subject and
// derives a title from what it finds. Context is what makes two articles on the
// same topic different: who is reading, what angle to take, what to avoid.
type BlogBrief struct {
	Topic string `json:"topic"`
	// Context is free text and optional. It is passed to the research planner
	// and to the writer verbatim rather than being parsed into fields — the
	// useful guidance people actually give ("assume they know Kubernetes",
	// "don't compare vendors") does not decompose into a schema.
	Context string `json:"context,omitempty"`
}

// BlogRun records a single invocation of the article pipeline.
type BlogRun struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	AgentID     *uuid.UUID `json:"agent_id"`
	TriggeredBy *uuid.UUID `json:"triggered_by"`
	Source      string     `json:"source"` // api | schedule
	Status      string     `json:"status"`
	// Briefs is what the run was asked to write. Topics is kept in step with
	// it — same order, topics only — because runs created before briefs existed
	// have only that, and because it stays the cheapest way to ask "what has
	// this org written about".
	Briefs []BlogBrief `json:"briefs"`
	Topics []string    `json:"topics"`
	Model  *string     `json:"model"`
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
	// Title is what the agent called the piece, which is what a list should
	// show — the topic is what it was asked to write about.
	Title string `json:"title"`
	Slug  string `json:"slug"`
	Path  string `json:"path"`
	// WordCount lets the list view show article size without loading the body.
	WordCount int `json:"word_count"`
	// ReferenceCount surfaces how well-sourced an article is without loading
	// its references, so a thinly-sourced piece is visible in the list.
	ReferenceCount int `json:"reference_count"`
}

// BlogReference is one source an article was written from, carried alongside
// the article rather than buried in its markdown.
//
// Every reference here was retrieved and had at least one claim verified
// against it — these are not "further reading" suggestions, they are the
// sources the piece actually rests on.
type BlogReference struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Site  string `json:"site,omitempty"`
	// PublishedAt is nil when the source did not report one; an unknown date is
	// left unknown rather than defaulted.
	PublishedAt *time.Time `json:"published_at,omitempty"`
}

// BlogArticle is a generated article: the markdown, the HTML it was converted
// to, and where it ended up in the CMS once published.
type BlogArticle struct {
	ID    uuid.UUID `json:"id"`
	RunID uuid.UUID `json:"run_id"`
	OrgID uuid.UUID `json:"org_id"`
	Topic string    `json:"topic"`
	// Title is the headline the agent chose after researching the topic. It is
	// stored rather than re-derived from the markdown H1 on every read, so the
	// title the agent decided on is the title that gets published.
	Title string `json:"title"`
	Slug  string `json:"slug"`
	Path  string `json:"path"`
	// References are the verified sources behind this article.
	References []BlogReference `json:"references"`
	Markdown   string          `json:"markdown"`
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
//
// Briefs is the current shape. Topics is still accepted so existing callers,
// stored scheduled tasks and the retry path keep working — Normalize folds one
// into the other, and every consumer reads Briefs.
type GenerateBlogRequest struct {
	Briefs []BlogBrief `json:"briefs,omitempty"`
	// Topics is the legacy form: a bare list of subjects with no context.
	Topics []string `json:"topics,omitempty"`
	Model  string   `json:"model,omitempty"`
	// Trending, when set, tells the pipeline to discover what to write about
	// instead of taking it from the request.
	//
	// Reserved here in Phase 1 and not yet honoured by the generator, so that
	// the API contract does not have to change a second time when topic
	// discovery lands. A request setting it today is rejected rather than
	// quietly ignored — see Validate.
	Trending bool `json:"trending,omitempty"`
	// TrendingCount is how many articles to write when Trending is set.
	TrendingCount int `json:"trending_count,omitempty"`
	MaxArticles   int `json:"max_articles,omitempty"`
}

// Normalize folds the legacy Topics field into Briefs and trims empties, so
// every consumer can read Briefs alone.
func (r *GenerateBlogRequest) Normalize() {
	briefs := make([]BlogBrief, 0, len(r.Briefs)+len(r.Topics))
	for _, b := range r.Briefs {
		topic := strings.TrimSpace(b.Topic)
		if topic == "" {
			continue
		}
		briefs = append(briefs, BlogBrief{Topic: topic, Context: strings.TrimSpace(b.Context)})
	}
	// A legacy topic is folded in only when no brief already covers it.
	//
	// The check is what makes Normalize idempotent, which it has to be: it
	// writes Topics back from Briefs below, and it is called on the way in at
	// the handler and again on the retry path. Without this, a second pass
	// would append every brief's topic a second time and double the run.
	//
	// It also means a caller that sends the same subject in both fields gets
	// one article rather than two, which is the more useful reading of an
	// ambiguous request.
	covered := make(map[string]struct{}, len(briefs))
	for _, b := range briefs {
		covered[b.Topic] = struct{}{}
	}
	for _, t := range r.Topics {
		topic := strings.TrimSpace(t)
		if topic == "" {
			continue
		}
		if _, dup := covered[topic]; dup {
			continue
		}
		covered[topic] = struct{}{}
		briefs = append(briefs, BlogBrief{Topic: topic})
	}
	r.Briefs = briefs

	// Keep Topics in step so the column, the retry path and anything reading
	// the legacy field see the same list in the same order.
	topics := make([]string, 0, len(briefs))
	for _, b := range briefs {
		topics = append(topics, b.Topic)
	}
	r.Topics = topics
}

// Validate reports why a request cannot be run, after Normalize.
//
// It is a method rather than struct tags because the requirement is
// conditional: briefs are required unless the run is discovering its own
// topics, and `validate:"required"` cannot express that.
func (r *GenerateBlogRequest) Validate() error {
	if r.Trending {
		// Accepting this now would mean silently writing about whatever the
		// caller last sent, or nothing at all. Refusing is the honest answer
		// until discovery is wired up.
		return fmt.Errorf("trending topic discovery is not available yet; supply briefs instead")
	}
	if len(r.Briefs) == 0 {
		return fmt.Errorf("at least one brief with a topic is required")
	}
	return nil
}
