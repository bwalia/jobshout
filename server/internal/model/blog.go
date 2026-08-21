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
	// BlogStepDiscovering runs only on a run that was not given a subject: it
	// finds what is trending and turns it into topics before any writing.
	BlogStepDiscovering = "discovering"
	// The writing phases, in order. Each is its own step so a reader watching a
	// run can see the agent research before it writes, and can tell a failure
	// to find sources apart from a failure to draft from them.
	BlogStepResearching = "researching"
	BlogStepOutlining   = "outlining"
	BlogStepGenerating  = "generating"
	BlogStepReviewing   = "reviewing"
	BlogStepRevising    = "revising"
	// BlogStepExpanding runs only when the finished draft came in under the
	// target length. Its own step so a short article that had to be filled out
	// is visible rather than hidden inside "writing".
	BlogStepExpanding = "expanding"
	// BlogStepIllustrating runs only when image generation is switched on. Its
	// own step because it is the slowest thing in the pipeline after the writing
	// itself — a cover costs tens of seconds on a shared GPU — and a run that
	// looks stalled inside "converting" for a minute is a run someone cancels.
	BlogStepIllustrating = "illustrating"
	BlogStepConverting   = "converting"
	BlogStepGenerated    = "generated"
	BlogStepPublishing   = "publishing"
	BlogStepPublished    = "published"
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

// Built-in agent display names, used to attribute each step of a run to
// whichever agent performs it. These are the names the agents are seeded under,
// kept as constants so the trace and the Agents page cannot drift apart.
const (
	AgentNameArticleWriter = "Article Writer"
	AgentNameResearcher    = "Research Agent"
)

// BlogStep is one entry in a run's progress trace. The shape mirrors PlanStep
// in goal.go so the UI can render both with the same timeline treatment.
type BlogStep struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Agent names which agent performs this step. A run is a collaboration —
	// the Research Agent gathers and verifies sources, the Article Writer turns
	// them into a piece — and without this the trace reads as one opaque
	// process rather than showing the handover.
	//
	// Empty on runs written before this field existed; the UI omits it rather
	// than guessing.
	Agent string `json:"agent,omitempty"`
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
	// HeartbeatAt is refreshed while this process is still writing, so a
	// reconciler can fail a run whose writer died (deploy SIGKILL, OOM)
	// without false-positiving a healthy long Ollama call.
	HeartbeatAt *time.Time `json:"heartbeat_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at"`
	PublishedAt *time.Time `json:"published_at"`
	CreatedAt   time.Time  `json:"created_at"`
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

	// CoverImageURL is where the article's cover image is served from, empty
	// when the run generated none — cover images are opt-in per environment, and
	// a run that could not draw one still produces a publishable article.
	CoverImageURL string `json:"cover_image_url,omitempty"`
	// CoverImagePrompt is what the image was asked for, kept so a reader can see
	// why the picture looks the way it does and so it can be edited and redrawn.
	CoverImagePrompt string `json:"cover_image_prompt,omitempty"`
	// CoverImageMeta is the provider, model and seed behind the cover. The seed
	// is the part that matters: without it a cover that came out well can be
	// regenerated but never reproduced.
	CoverImageMeta CoverImageMeta `json:"cover_image_meta,omitempty"`
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
	// Focus narrows a trending run to particular subject areas. Meaningless
	// without Trending — a run given its topics outright is already focused —
	// and rejected in that combination rather than quietly ignored.
	Focus       []string `json:"focus,omitempty"`
	MaxArticles int      `json:"max_articles,omitempty"`
	// AutoPublish files the finished articles in the CMS without waiting for
	// someone to press the button.
	//
	// It exists for scheduled runs, where there is nobody at the keyboard at
	// 2am. It creates drafts, exactly as the manual action does — nothing goes
	// live without a human approving it in the CMS — so the worst case is a
	// draft somebody deletes rather than a bad article published to readers.
	AutoPublish bool `json:"auto_publish,omitempty"`
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

	// Focus areas arrive from a text box, so blanks and repeats are normal
	// input rather than caller error. Cleaning them here keeps Normalize
	// idempotent and means the discovery prompt never shows an empty bullet.
	if len(r.Focus) > 0 {
		seen := make(map[string]struct{}, len(r.Focus))
		focus := make([]string, 0, len(r.Focus))
		for _, f := range r.Focus {
			area := strings.TrimSpace(f)
			if area == "" {
				continue
			}
			key := strings.ToLower(area)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			focus = append(focus, area)
		}
		r.Focus = focus
	}
}

// Validate reports why a request cannot be run, after Normalize.
//
// It is a method rather than struct tags because the requirement is
// conditional: briefs are required unless the run is discovering its own
// topics, and `validate:"required"` cannot express that.
func (r *GenerateBlogRequest) Validate() error {
	if r.Trending {
		// The topics do not exist yet — discovery finds them when the run
		// starts — so there is nothing here to require.
		if r.TrendingCount < 0 {
			return fmt.Errorf("trending_count cannot be negative")
		}
		return nil
	}
	// Rejected rather than ignored: a focus area silently dropped would look
	// like it was applied, and the run would write about anything at all while
	// the caller believed it was steering.
	if len(r.Focus) > 0 {
		return fmt.Errorf("focus areas only apply to a trending run")
	}
	if len(r.Briefs) == 0 {
		return fmt.Errorf("at least one brief with a topic is required")
	}
	return nil
}

// DefaultTrendingCount is how many articles a trending run writes when the
// caller does not say. One, because a scheduled job that quietly produces five
// articles a day is a bigger surprise than one that produces too few.
const DefaultTrendingCount = 1

// ResolvedTrendingCount is how many articles this request should discover
// topics for, bounded by the same ceiling as an explicit batch.
func (r *GenerateBlogRequest) ResolvedTrendingCount(hardMax int) int {
	n := r.TrendingCount
	if n <= 0 {
		n = DefaultTrendingCount
	}
	if r.MaxArticles > 0 && n > r.MaxArticles {
		n = r.MaxArticles
	}
	if hardMax > 0 && n > hardMax {
		n = hardMax
	}
	return n
}
