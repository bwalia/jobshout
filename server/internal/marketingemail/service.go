package marketingemail

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration/adapters/opsapi"

	"github.com/jobshout/server/internal/llmtrace"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// Illustrator draws the hero image. It mirrors blog.Illustrator so main.go can
// adapt the same image service; declared here so this package does not import
// blog. Enabled false (or a nil Illustrator) means no hero image.
type Illustrator interface {
	Generate(ctx context.Context, req IllustrationRequest) (*Illustration, error)
	Enabled() bool
}

// IllustrationRequest asks for one picture.
type IllustrationRequest struct {
	OrgID         uuid.UUID
	Prompt        string
	Width, Height int
	Source        string
}

// Illustration is what came back. URL must be an absolute https URL to be used.
type Illustration struct {
	URL string
}

// Tasks is the slice of the task service board updates need.
type Tasks interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.Task, error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateTaskRequest) (*model.Task, error)
	Transition(ctx context.Context, id uuid.UUID, status string, changedBy *uuid.UUID) error
}

// Runs records every run; satisfied by repository.MarketingEmailRunRepository.
type Runs = repository.MarketingEmailRunRepository

// StartRequest is a launch, resolved.
type StartRequest struct {
	OrgID   uuid.UUID
	UserID  uuid.UUID
	AgentID uuid.UUID
	TaskID  *uuid.UUID
	Brief   model.MarketingEmailBrief
}

// ErrNoRuns: the server keeps no run records (no database wired).
var ErrNoRuns = errors.New("marketing email runs are not recorded on this server")

// ErrBusy: the run is still being written or published.
var ErrBusy = errors.New("this email is still being written")

// ErrNotPublished: there is no CMS page to unpublish.
var ErrNotPublished = errors.New("this email has no published page")

const (
	// draftTimeout bounds the LLM writing step.
	draftTimeout = 6 * time.Minute
	// heroTimeout bounds the hero illustration.
	heroTimeout = 5 * time.Minute
	// publishTimeout bounds the CMS calls.
	publishTimeout = 2 * time.Minute
	// heroWidth/heroHeight are 2× the 600px column, 16:9.
	heroWidth, heroHeight = 1200, 675
)

// Service writes marketing emails and files them as OpsAPI CMS pages.
//
// Each launch is a run row, updated at every step so the agent's tab can show
// what it is doing. Re-generating a run upserts the same page by slug;
// Unpublish archives it. Nothing is ever filed to /cms/posts.
type Service struct {
	writer       *Writer
	destinations DestinationResolver
	illustrator  Illustrator
	tasks        Tasks
	runs         Runs
	chrome       Chrome
	logos        *LogoChecker
	// saveTemplates saves each email as an OpsAPI Marketing mail-merge
	// template as well (MARKETING_EMAIL_SAVE_TEMPLATE, default on).
	saveTemplates bool
	logger        *zap.Logger
	// sync runs the pipeline inline (tests).
	sync bool
}

// NewService wires the Marketing Email Writer. A nil writer or resolver leaves
// it "not configured" — Start says so.
func NewService(writer *Writer, destinations DestinationResolver, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{
		writer: writer, destinations: destinations, chrome: ChromeFromEnv(), logos: NewLogoChecker(),
		saveTemplates: SaveTemplatesFromEnv(), logger: logger,
	}
}

// EnvSaveTemplate turns saving the OpsAPI Marketing mail-merge template on or
// off. Default on; "false", "0", "off" or "no" turn it off.
const EnvSaveTemplate = "MARKETING_EMAIL_SAVE_TEMPLATE"

// SaveTemplatesFromEnv reads EnvSaveTemplate.
func SaveTemplatesFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvSaveTemplate))) {
	case "false", "0", "off", "no":
		return false
	}
	return true
}

// WithTemplateSave turns saving the mail-merge template on or off.
func (s *Service) WithTemplateSave(on bool) *Service { s.saveTemplates = on; return s }

// WithLogoChecker replaces the logo check; nil always uses the text wordmark.
func (s *Service) WithLogoChecker(c *LogoChecker) *Service { s.logos = c; return s }

// chromeFor is the chrome for one render: the logo only when it answers 200
// (cached), else the text wordmark — never a broken image.
func (s *Service) chromeFor(ctx context.Context) Chrome {
	c := s.chrome
	if c.LogoURL != "" && !s.logos.Available(ctx, c.LogoURL) {
		c.LogoURL = ""
	}
	return c
}

// BindRuns lets runs be recorded for the agent's tab.
func (s *Service) BindRuns(runs Runs) { s.runs = runs }

// BindTasks lets runs report to the board.
func (s *Service) BindTasks(tasks Tasks) { s.tasks = tasks }

// WithIllustrator enables hero images.
func (s *Service) WithIllustrator(i Illustrator) *Service { s.illustrator = i; return s }

// WithChrome overrides the email chrome (logo, footer links).
func (s *Service) WithChrome(c Chrome) *Service { s.chrome = c; return s }

// Configured reports whether a launch can run at all.
func (s *Service) Configured() bool {
	return s != nil && s.writer != nil && s.writer.LLM != nil && s.destinations != nil
}

// List returns an org's runs, newest first.
func (s *Service) List(ctx context.Context, orgID uuid.UUID, limit int) ([]model.MarketingEmailRun, error) {
	if s == nil || s.runs == nil {
		return nil, ErrNoRuns
	}
	return s.runs.List(ctx, orgID, limit)
}

// Get returns one of an org's runs.
func (s *Service) Get(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error) {
	if s == nil || s.runs == nil {
		return nil, ErrNoRuns
	}
	return s.runs.Get(ctx, orgID, id)
}

// normaliseBrief trims, defaults and validates a brief.
func normaliseBrief(b model.MarketingEmailBrief) (model.MarketingEmailBrief, error) {
	b.Brief = strings.TrimSpace(b.Brief)
	b.Audience = strings.TrimSpace(b.Audience)
	b.Goal = strings.ToLower(strings.TrimSpace(b.Goal))
	b.PrimaryCTALabel = strings.TrimSpace(b.PrimaryCTALabel)
	b.PrimaryCTAURL = strings.TrimSpace(b.PrimaryCTAURL)
	b.Products = strings.TrimSpace(b.Products)
	b.Tone = strings.TrimSpace(b.Tone)
	b.Destination = strings.TrimSpace(b.Destination)
	b.Model = strings.TrimSpace(b.Model)
	b.Publish = strings.ToLower(strings.TrimSpace(b.Publish))

	if b.Brief == "" {
		return b, errors.New("tell the Marketing Email Writer what the email is about")
	}
	switch b.Goal {
	case "", model.MarketingEmailGoalAnnounce, model.MarketingEmailGoalPromote,
		model.MarketingEmailGoalNurture, model.MarketingEmailGoalNewsletter:
	default:
		return b, fmt.Errorf("goal %q is not one of announce, promote, nurture, newsletter", b.Goal)
	}
	switch b.Publish {
	case "":
		b.Publish = model.MarketingEmailPublishLive
	case model.MarketingEmailPublishLive, model.MarketingEmailPublishDraft:
	default:
		return b, fmt.Errorf("publish %q is not live or draft", b.Publish)
	}
	if b.PrimaryCTAURL != "" && !IsHTTPS(b.PrimaryCTAURL) {
		return b, errors.New("primary_cta_url must be an https URL")
	}
	if b.PrimaryCTAURL != "" && b.PrimaryCTALabel == "" {
		b.PrimaryCTALabel = "Learn more"
	}
	return b, nil
}

// Start validates the launch, records a run and writes and publishes the email
// in the background. Problems the caller can fix — an empty brief, a bad URL, a
// website this org cannot file to — are an immediate error, not a failed run.
func (s *Service) Start(ctx context.Context, req StartRequest) (uuid.UUID, error) {
	switch {
	case s == nil || s.writer == nil || s.writer.LLM == nil:
		return uuid.Nil, errors.New("the Marketing Email Writer has no LLM to write emails with")
	case s.destinations == nil:
		return uuid.Nil, errors.New("the Marketing Email Writer has no website to publish to")
	}
	brief, err := normaliseBrief(req.Brief)
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := s.destinations.Resolve(ctx, req.OrgID, brief.Destination); err != nil {
		return uuid.Nil, err
	}

	now := time.Now()
	userID := req.UserID
	run := &model.MarketingEmailRun{
		ID:          uuid.New(),
		OrgID:       req.OrgID,
		TaskID:      req.TaskID,
		RequestedBy: &userID,
		Brief:       brief,
		Status:      model.MarketingEmailStatusWriting,
		Stage:       "draft",
		Message:     "Writing the email",
		Destination: brief.Destination,
		Model:       brief.Model,
		PublishMode: brief.Publish,
		StartedAt:   &now,
		Draft:       model.MarketingEmailDraft{Sections: []model.MarketingEmailSection{}},
	}
	if req.AgentID != uuid.Nil {
		id := req.AgentID
		run.AgentID = &id
	}
	if s.runs != nil {
		if err := s.runs.Create(ctx, run); err != nil {
			return uuid.Nil, fmt.Errorf("marketingemail: record the run: %w", err)
		}
	}
	s.dispatch(run, false)
	return run.ID, nil
}

// ReGenerate writes the email again from the same brief and updates the same
// CMS page (upsert by slug).
func (s *Service) ReGenerate(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error) {
	run, err := s.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if run.Unfinished() {
		return run, ErrBusy
	}
	if _, err := s.destinations.Resolve(ctx, orgID, run.Brief.Destination); err != nil {
		return nil, err
	}
	now := time.Now()
	run.Status, run.Stage, run.Message, run.Error = model.MarketingEmailStatusWriting, "draft", "Writing the email again", ""
	run.StartedAt, run.FinishedAt = &now, nil
	s.save(run)
	s.dispatch(run, true)
	return run, nil
}

// Unpublish archives the run's CMS page so the browser version stops serving.
func (s *Service) Unpublish(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error) {
	run, err := s.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if run.Unfinished() {
		return run, ErrBusy
	}
	if run.PageUUID == "" {
		return run, ErrNotPublished
	}
	dest, err := s.destinations.Resolve(ctx, orgID, run.Brief.Destination)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	// Only the status is sent: nothing else about the page changes.
	if _, err := dest.Pages.UpdatePage(cctx, run.PageUUID, map[string]string{"status": "archived"}); err != nil {
		return run, fmt.Errorf("unpublish: %w", err)
	}
	now := time.Now()
	run.Status, run.PageStatus, run.Message, run.FinishedAt = model.MarketingEmailStatusArchived, "archived", "Unpublished: the page is archived", &now
	s.save(run)
	s.note(run.TaskID, "Email unpublished (page archived): "+run.Subject, "")
	return run, nil
}

func (s *Service) dispatch(run *model.MarketingEmailRun, upsert bool) {
	if s.sync {
		s.run(context.Background(), run, upsert)
		return
	}
	go s.run(context.Background(), run, upsert)
}

// run is the pipeline: draft → hero → render → upsert page.
func (s *Service) run(parent context.Context, run *model.MarketingEmailRun, upsert bool) {
	log := s.logger.With(zap.String("marketing_email_run", run.ID.String()))
	ctx := parent
	trace := llmtrace.TraceInfo{
		TraceName: "go-marketing-email",
		SessionID: run.ID.String(),
		OrgID:     run.OrgID.String(),
	}
	if run.AgentID != nil {
		trace.AgentID = run.AgentID.String()
	}
	if run.TaskID != nil {
		trace.TaskID = run.TaskID.String()
	}
	ctx = llmtrace.WithTrace(ctx, trace)

	dest, err := s.destinations.Resolve(ctx, run.OrgID, run.Brief.Destination)
	if err != nil {
		s.fail(run, err)
		return
	}

	// 1. Structured draft.
	dctx, cancel := context.WithTimeout(ctx, draftTimeout)
	draft, err := s.writer.Draft(dctx, run.Brief)
	cancel()
	if err != nil {
		log.Warn("marketingemail: draft failed", zap.Error(err))
		s.fail(run, fmt.Errorf("writing the email failed: %w", err))
		return
	}
	run.Draft = *draft
	run.Subject, run.Preheader, run.Title = strings.TrimSpace(draft.Subject), strings.TrimSpace(draft.Preheader), strings.TrimSpace(draft.Title)
	if run.Title == "" {
		run.Title = run.Subject
	}
	run.Stage, run.Message = "render", "Rendering the email"
	s.save(run)

	// 2. Hero image (optional; never a broken one).
	run.HeroImageURL = s.hero(ctx, run, draft, log)

	// 3. Render.
	chrome := s.chromeFor(ctx)
	html, err := Render(draft, run.HeroImageURL, chrome)
	if err != nil {
		s.fail(run, err)
		return
	}
	run.ContentHTML = html

	// 4. Publish: upsert the CMS page by slug.
	run.Status, run.Stage, run.Message = model.MarketingEmailStatusPublishing, "publish", "Filing the page in the CMS"
	s.save(run)

	pctx, pcancel := context.WithTimeout(ctx, publishTimeout)
	defer pcancel()
	if run.Slug == "" || !upsert {
		base := Slugify(firstNonEmpty(draft.SlugHint, draft.Title, draft.Subject))
		if base == "" {
			s.fail(run, errors.New("could not make a slug from the email"))
			return
		}
		slug, err := uniqueSlug(pctx, dest.Pages, base)
		if err != nil {
			s.fail(run, err)
			return
		}
		run.Slug = slug
	}
	live := run.PublishMode != model.MarketingEmailPublishDraft
	page, err := upsertPage(pctx, dest.Pages, pageInput{
		Title: run.Title, Slug: run.Slug, Excerpt: run.Preheader, HTML: run.ContentHTML,
		HeroImageURL: run.HeroImageURL, SEOTitle: run.Subject, Live: live,
	})
	if err != nil {
		log.Warn("marketingemail: publish failed", zap.Error(err))
		s.fail(run, err)
		return
	}
	run.PageUUID, run.PageStatus = page.UUID, page.Status
	if run.PageStatus == "" {
		run.PageStatus = map[bool]string{true: "published", false: "draft"}[live]
	}
	run.BrowserURL = dest.BrowserURL(run.Slug)
	if live {
		run.Status, run.Message = model.MarketingEmailStatusPublished, "Published"
	} else {
		run.Status, run.Message = model.MarketingEmailStatusDraft, "Filed as a draft"
	}
	if run.BrowserURL != "" {
		run.Message += ": " + run.BrowserURL
	}

	// 5. Save the same email as an OpsAPI Marketing mail-merge template. A
	// failure here is reported on the run; it never fails the CMS publish.
	run.Stage = "template"
	s.save(run)
	s.saveTemplate(ctx, run, dest, draft, chrome, log)

	s.finish(run, "")
	s.note(run.TaskID, doneNote(run), "done")
}

// templateTimeout bounds the Marketing template calls.
const templateTimeout = time.Minute

// TemplateName is the stable name a run's template is upserted by: the slug
// does not change when an email is re-generated.
func TemplateName(slug string) string { return "Email: " + slug }

// saveTemplate upserts the run's mail-merge template by name (or by the uuid
// a previous run stored) as a draft email template, and records the outcome.
func (s *Service) saveTemplate(ctx context.Context, run *model.MarketingEmailRun, dest *Destination,
	d *model.MarketingEmailDraft, chrome Chrome, log *zap.Logger) {
	notSaved := func(msg string) {
		run.TemplateOutcome, run.TemplateMessage = model.MarketingTemplateNotSaved, "Template not saved: "+msg
	}
	if !s.saveTemplates {
		run.TemplateOutcome = model.MarketingTemplateDisabled
		run.TemplateMessage = "Template not saved: saving mail-merge templates is turned off (" + EnvSaveTemplate + ")"
		return
	}
	if dest == nil || dest.Templates == nil {
		notSaved("this website has no OpsAPI Marketing connection")
		return
	}
	html, err := RenderMailMerge(d, run.HeroImageURL, chrome)
	if err != nil {
		notSaved(err.Error())
		return
	}
	run.TemplateHTML = html
	run.TemplateName = TemplateName(run.Slug)
	req := opsapi.MarketingTemplateRequest{
		Name: run.TemplateName, Channel: "email", Subject: run.Subject, PreviewText: run.Preheader,
		BodyHTML: html, BodyText: RenderMailMergeText(d, chrome),
	}

	tctx, cancel := context.WithTimeout(ctx, templateTimeout)
	defer cancel()
	tpl, analysis, err := upsertTemplate(tctx, dest.Templates, run.TemplateUUID, req)
	if err != nil {
		log.Warn("marketingemail: mail-merge template not saved", zap.Error(err))
		switch {
		case errors.Is(err, opsapi.ErrMarketingScope):
			notSaved("missing " + opsapi.MarketingTemplatesScope + " scope")
		case errors.Is(err, opsapi.ErrMarketingDisabled):
			notSaved("the OpsAPI Marketing plugin is not enabled for namespace " + dest.Namespace)
		default:
			notSaved(err.Error())
		}
		return
	}
	run.TemplateUUID, run.TemplateOutcome = tpl.UUID, model.MarketingTemplateSaved
	status := firstNonEmpty(tpl.Status, opsapi.StatusDraft)
	run.TemplateMessage = fmt.Sprintf("Saved as mail-merge template %q (%s)", run.TemplateName, status)
	if analysis != nil {
		var notes []string
		for _, k := range analysis.UnknownMerge {
			notes = append(notes, "unknown merge field "+k)
		}
		notes = append(notes, analysis.Warnings...)
		if len(notes) > 0 {
			run.TemplateMessage += "; OpsAPI warnings: " + strings.Join(notes, ", ")
		}
	}
}

// upsertTemplate saves req to the template at knownUUID, else to the one
// named req.Name, else to a new draft template. Content always goes through
// the studio save so OpsAPI sanitises and analyses it.
func upsertTemplate(ctx context.Context, t MarketingTemplates, knownUUID string, req opsapi.MarketingTemplateRequest) (
	*opsapi.MarketingTemplate, *opsapi.TemplateAnalysis, error) {
	id := strings.TrimSpace(knownUUID)
	if id == "" {
		found, err := t.FindMarketingTemplateByName(ctx, req.Name)
		if err != nil {
			return nil, nil, err
		}
		if found != nil {
			id = found.UUID
		}
	}
	if id == "" {
		create := req
		create.Status = opsapi.StatusDraft
		created, err := t.CreateMarketingTemplate(ctx, create)
		if err != nil {
			return nil, nil, err
		}
		id = created.UUID
	}
	req.Status = "" // keep whatever status the template has now
	tpl, analysis, err := t.SaveMarketingTemplate(ctx, id, req)
	if err != nil && knownUUID != "" && !errors.Is(err, opsapi.ErrMarketingScope) && !errors.Is(err, opsapi.ErrMarketingDisabled) {
		// The stored template may have been deleted: find or create by name.
		return upsertTemplate(ctx, t, "", req)
	}
	return tpl, analysis, err
}

// SaveTemplate saves (or re-saves) a finished run's mail-merge template from
// its stored draft — e.g. after the key gains the Marketing scope.
func (s *Service) SaveTemplate(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error) {
	run, err := s.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if run.Unfinished() {
		return run, ErrBusy
	}
	if run.Slug == "" || len(run.Draft.Sections) == 0 {
		return run, ErrNotPublished
	}
	dest, err := s.destinations.Resolve(ctx, orgID, run.Brief.Destination)
	if err != nil {
		return nil, err
	}
	draft := run.Draft
	s.saveTemplate(ctx, run, dest, &draft, s.chromeFor(ctx), s.logger.With(zap.String("marketing_email_run", run.ID.String())))
	s.save(run)
	return run, nil
}

// hero draws the hero image when an illustrator is on and the prompt exists.
// It returns "" on any problem or when the URL is not an absolute https one.
func (s *Service) hero(ctx context.Context, run *model.MarketingEmailRun, d *model.MarketingEmailDraft, log *zap.Logger) string {
	prompt := strings.TrimSpace(d.Hero.ImagePrompt)
	if s.illustrator == nil || !s.illustrator.Enabled() || prompt == "" {
		return ""
	}
	hctx, cancel := context.WithTimeout(ctx, heroTimeout)
	defer cancel()
	ill, err := s.illustrator.Generate(hctx, IllustrationRequest{
		OrgID: run.OrgID, Prompt: prompt, Width: heroWidth, Height: heroHeight, Source: "marketing_email",
	})
	if err != nil || ill == nil {
		log.Warn("marketingemail: hero image failed; sending without one", zap.Error(err))
		return ""
	}
	if !IsHTTPS(ill.URL) {
		log.Warn("marketingemail: hero image is not an absolute https URL; sending without one", zap.String("url", ill.URL))
		return ""
	}
	return ill.URL
}

func (s *Service) fail(run *model.MarketingEmailRun, err error) {
	run.Status, run.Message = model.MarketingEmailStatusFailed, err.Error()
	s.finish(run, err.Error())
	s.note(run.TaskID, "Marketing email failed: "+err.Error(), "")
}

func (s *Service) finish(run *model.MarketingEmailRun, errMsg string) {
	now := time.Now()
	run.Error, run.FinishedAt = errMsg, &now
	if errMsg == "" {
		run.Stage = "done"
	}
	s.save(run)
}

func (s *Service) save(run *model.MarketingEmailRun) {
	if s.runs == nil {
		return
	}
	if err := s.runs.Save(context.Background(), run); err != nil {
		s.logger.Warn("marketingemail: could not record run progress", zap.String("marketing_email_run", run.ID.String()), zap.Error(err))
	}
}

func doneNote(run *model.MarketingEmailRun) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Marketing email %s: %s\nPreheader: %s", run.Status, run.Subject, run.Preheader)
	if run.BrowserURL != "" {
		fmt.Fprintf(&sb, "\nBrowser version: %s", run.BrowserURL)
	}
	if run.TemplateMessage != "" {
		sb.WriteString("\n" + run.TemplateMessage)
		if run.TemplateUUID != "" {
			fmt.Fprintf(&sb, " — template %s", run.TemplateUUID)
		}
	}
	return sb.String()
}

// note appends to the board task's description and optionally moves it.
func (s *Service) note(taskID *uuid.UUID, note, status string) {
	if s.tasks == nil || taskID == nil {
		return
	}
	ctx := context.Background()
	text := note
	if task, err := s.tasks.GetByID(ctx, *taskID); err == nil && task != nil && task.Description != nil && *task.Description != "" {
		text = *task.Description + "\n\n" + note
	}
	_, _ = s.tasks.Update(ctx, *taskID, model.UpdateTaskRequest{Description: &text})
	if status != "" {
		_ = s.tasks.Transition(ctx, *taskID, status, nil)
	}
}

// FailStale fails runs whose goroutine died with a previous server, so they do
// not sit at "writing" forever. Call it once at boot.
func (s *Service) FailStale(ctx context.Context, olderThan time.Duration) {
	if s == nil || s.runs == nil {
		return
	}
	n, err := s.runs.FailStale(ctx, time.Now().Add(-olderThan), "Interrupted by a server restart; re-generate to try again.")
	if err != nil {
		s.logger.Warn("marketingemail: could not fail stale runs", zap.Error(err))
	} else if n > 0 {
		s.logger.Info("marketingemail: failed stale runs", zap.Int("count", n))
	}
}
