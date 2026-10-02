package course

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// Token budgets per stage. Chapter text is generous because non-English
// scripts cost more tokens per word.
const (
	maxOutlineTokens = 2000
	maxChapterTokens = 6000
	maxReviewTokens  = 1500
	maxQuizTokens    = 2500
	maxNoteFindings  = 40
	maxTags          = 6
	maxTagLen        = 40 // OpsAPI academy tag limit
	maxReviewIssues  = 8
)

// Researcher is satisfied by *research.Agent.
type Researcher interface {
	Research(ctx context.Context, req research.Request, progress research.ProgressFunc) (*research.Brief, error)
}

// Illustrator generates an image and returns a URL it can be served from.
type Illustrator interface {
	Illustrate(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID, prompt string, width, height int) (string, error)
}

// Job is one course to generate.
type Job struct {
	// RunID tags the phase timing logs; it is optional.
	RunID  uuid.UUID
	OrgID  uuid.UUID
	UserID *uuid.UUID
	Brief  model.CourseBrief
}

// Hooks let the caller persist progress as it happens, so a crash late in a
// long run does not lose the chapters already written.
type Hooks struct {
	Step    func(key, detail string)
	Planned func(outline *model.CourseOutline, sources []model.CourseSource) error
	Cover   func(url string) error
	Chapter func(ch *model.CourseChapter) error
	Warn    func(msg string)
}

// Generator runs the text pipeline: research → outline → per chapter
// (theory → review/revise → visuals → quiz) → render.
type Generator struct {
	llm         llm.Client
	researcher  Researcher
	illustrator Illustrator
	cfg         Config
	logger      *zap.Logger
}

// NewGenerator wires the pipeline. illustrator may be nil (no images).
func NewGenerator(client llm.Client, researcher Researcher, illustrator Illustrator, cfg Config, logger *zap.Logger) *Generator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Generator{llm: client, researcher: researcher, illustrator: illustrator, cfg: cfg, logger: logger}
}

// Ready reports whether the generator can run at all.
func (g *Generator) Ready() error {
	if g == nil || g.llm == nil {
		return errors.New("course generator: no LLM configured")
	}
	if g.researcher == nil {
		return errors.New("course generator: research is not configured")
	}
	return nil
}

func (g *Generator) imagesEnabled() bool {
	return g.cfg.Images && g.illustrator != nil
}

// Generate runs the whole pipeline for one course.
func (g *Generator) Generate(ctx context.Context, job Job, h Hooks) error {
	if err := g.Ready(); err != nil {
		return err
	}
	h = h.withDefaults()
	b := job.Brief

	total := g.phase(job, "total", zap.Int("chapters", b.ChapterCount))
	err := g.generate(ctx, job, h)
	total(err)
	return err
}

func (g *Generator) generate(ctx context.Context, job Job, h Hooks) error {
	b := job.Brief
	// Every call in the run — research included — goes to the brief's
	// provider through the routed client.
	if p := llm.NormalizeProvider(b.Provider); p != "" {
		ctx = llm.WithProvider(ctx, p)
	}

	h.Step(model.CourseStepResearching, b.Topic)
	done := g.phase(job, "research")
	brief, err := g.researcher.Research(ctx, research.Request{
		Topic:   b.Topic,
		Context: strings.TrimSpace(b.Audience + "\n" + b.Context),
		Focus:   b.Focus,
		Seeds:   b.SeedURLs,
		Model:   g.modelFor(b),
	}, nil)
	done(err)
	if err != nil {
		return fmt.Errorf("research: %w", err)
	}
	if !brief.IsUsable() {
		return errors.New("research found no verified sources for this topic; add source URLs or narrow the topic")
	}
	for _, w := range brief.Warnings {
		h.Warn("research: " + w)
	}
	notes := researchNotes(brief, maxNoteFindings)

	h.Step(model.CourseStepOutlining, "")
	done = g.phase(job, "outline")
	outline, err := g.outline(ctx, b, notes, h)
	done(err)
	if err != nil {
		return err
	}
	sources := make([]model.CourseSource, 0, len(brief.Sources))
	for _, s := range brief.Sources {
		sources = append(sources, model.CourseSource{URL: s.URL, Title: s.Title})
	}
	if err := h.Planned(outline, sources); err != nil {
		return fmt.Errorf("save outline: %w", err)
	}

	if g.imagesEnabled() {
		h.Step(model.CourseStepIllustrating, "course cover")
		done = g.phase(job, "cover")
		url, err := g.illustrator.Illustrate(ctx, job.OrgID, job.UserID, coverPrompt(outline), 1536, 864)
		done(err)
		if err != nil {
			h.Warn("cover image: " + err.Error())
		} else if err := h.Cover(url); err != nil {
			return fmt.Errorf("save cover: %w", err)
		}
	}

	for i := range outline.Chapters {
		if err := ctx.Err(); err != nil {
			return err
		}
		ch, err := g.chapter(ctx, job, outline, i, notes, h)
		if err != nil {
			return fmt.Errorf("chapter %d: %w", i+1, err)
		}
		if err := h.Chapter(ch); err != nil {
			return fmt.Errorf("save chapter %d: %w", i+1, err)
		}
	}
	h.Step(model.CourseStepSaved, "")
	return nil
}

type outlineWire struct {
	Title            looseString          `json:"title"`
	Description      looseString          `json:"description"`
	Category         looseString          `json:"category"`
	Tags             looseStrings         `json:"tags"`
	LearningOutcomes looseStrings         `json:"learning_outcomes"`
	Chapters         []outlineChapterWire `json:"chapters"`
}

type outlineChapterWire struct {
	Title      looseString  `json:"title"`
	Summary    looseString  `json:"summary"`
	Objectives looseStrings `json:"objectives"`
}

func (g *Generator) outline(ctx context.Context, b model.CourseBrief, notes string, h Hooks) (*model.CourseOutline, error) {
	prompt := outlinePrompt(b, notes)
	var w outlineWire
	if err := g.json(ctx, "outline", prompt, maxOutlineTokens, b, &w); err != nil {
		return nil, err
	}
	o := normalizeOutline(w, b)
	if len(o.Chapters) < b.ChapterCount {
		// One corrective retry: a short plan is the commonest outline miss.
		var retry outlineWire
		again := prompt + fmt.Sprintf("\n\nYour previous plan had %d chapters. It MUST have exactly %d.", len(o.Chapters), b.ChapterCount)
		if err := g.json(ctx, "outline", again, maxOutlineTokens, b, &retry); err == nil {
			if r := normalizeOutline(retry, b); len(r.Chapters) > len(o.Chapters) {
				o = r
			}
		}
	}
	if o.Title == "" || len(o.Chapters) == 0 {
		return nil, errors.New("outline: the model returned no usable course plan")
	}
	if len(o.Chapters) < b.ChapterCount {
		h.Warn(fmt.Sprintf("outline has %d chapters; %d were requested", len(o.Chapters), b.ChapterCount))
	}
	return o, nil
}

// normalizeOutline trims the model's plan to what the Academy accepts.
func normalizeOutline(w outlineWire, b model.CourseBrief) *model.CourseOutline {
	o := &model.CourseOutline{
		Title:            strings.TrimSpace(string(w.Title)),
		Description:      strings.TrimSpace(string(w.Description)),
		Level:            b.Level,
		Category:         strings.ToLower(strings.TrimSpace(string(w.Category))),
		LearningOutcomes: []string(w.LearningOutcomes),
	}
	if o.Category == "" {
		o.Category = "general"
	}
	seen := map[string]bool{}
	for _, t := range w.Tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || len(t) > maxTagLen || seen[t] {
			continue
		}
		seen[t] = true
		o.Tags = append(o.Tags, t)
		if len(o.Tags) == maxTags {
			break
		}
	}
	for _, c := range w.Chapters {
		title := strings.TrimSpace(string(c.Title))
		if title == "" {
			continue
		}
		o.Chapters = append(o.Chapters, model.CourseOutlineChapter{
			Title:      title,
			Summary:    strings.TrimSpace(string(c.Summary)),
			Objectives: []string(c.Objectives),
		})
		if len(o.Chapters) == b.ChapterCount {
			break
		}
	}
	return o
}

func (g *Generator) chapter(ctx context.Context, job Job, o *model.CourseOutline, idx int, notes string, h Hooks) (*model.CourseChapter, error) {
	b := job.Brief
	plan := o.Chapters[idx]
	label := fmt.Sprintf("%d/%d: %s", idx+1, len(o.Chapters), plan.Title)

	chapter := zap.Int("chapter", idx+1)

	h.Step(model.CourseStepWriting, label)
	done := g.phase(job, "write", chapter)
	md, err := g.text(ctx, chapterPrompt(b, o, idx, notes), maxChapterTokens, b)
	done(err)
	if err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	if strings.TrimSpace(md) == "" {
		return nil, errors.New("write: empty chapter")
	}

	h.Step(model.CourseStepReviewing, label)
	var review struct {
		Issues looseStrings `json:"issues"`
	}
	done = g.phase(job, "review", chapter)
	err = g.json(ctx, "review", reviewPrompt(b, plan.Title, md), maxReviewTokens, b, &review)
	done(err)
	if err != nil {
		h.Warn(fmt.Sprintf("chapter %d review skipped: %v", idx+1, err))
	} else if issues := capList(review.Issues, maxReviewIssues); len(issues) > 0 {
		done = g.phase(job, "revise", chapter, zap.Int("issues", len(issues)))
		revised, err := g.text(ctx, revisePrompt(b, plan.Title, md, issues), maxChapterTokens, b)
		done(err)
		switch {
		case err != nil:
			h.Warn(fmt.Sprintf("chapter %d revision failed, keeping draft: %v", idx+1, err))
		case strings.TrimSpace(revised) != "":
			md = revised
		}
	}
	md = stripFence(md)

	var images []model.CourseImage
	if g.imagesEnabled() {
		h.Step(model.CourseStepIllustrating, label)
		done = g.phase(job, "illustrate", chapter)
		url, err := g.illustrator.Illustrate(ctx, job.OrgID, job.UserID, illustrationPrompt(o.Title, plan.Title, plan.Summary), 1280, 720)
		done(err)
		if err != nil {
			h.Warn(fmt.Sprintf("chapter %d image: %v", idx+1, err))
		} else {
			images = append(images, model.CourseImage{URL: url, Alt: plan.Title, Caption: plan.Summary})
		}
	}

	h.Step(model.CourseStepQuizzing, label)
	var quiz *model.CourseQuiz
	var qw quizWire
	done = g.phase(job, "quiz", chapter)
	err = g.json(ctx, "quiz", quizPrompt(b, plan.Title, md), maxQuizTokens, b, &qw)
	done(err)
	if err != nil {
		h.Warn(fmt.Sprintf("chapter %d quiz: %v", idx+1, err))
	} else if q, err := validateQuiz(qw); err != nil {
		h.Warn(fmt.Sprintf("chapter %d %v", idx+1, err))
	} else {
		quiz = q
	}

	html, err := renderLesson(md, images)
	if err != nil {
		return nil, err
	}
	return &model.CourseChapter{
		Locale:     b.Locale,
		Position:   idx + 1,
		Title:      plan.Title,
		Summary:    plan.Summary,
		Objectives: plan.Objectives,
		Markdown:   md,
		HTML:       html,
		Images:     images,
		Quiz:       quiz,
	}, nil
}

func (g *Generator) modelFor(b model.CourseBrief) string {
	if b.Model != "" {
		return b.Model
	}
	// COURSE_MODEL names a model on the server's provider; on any other the
	// provider's own default is used.
	if p := llm.NormalizeProvider(b.Provider); p != "" && p != llm.NormalizeProvider(g.cfg.Provider) {
		return ""
	}
	return g.cfg.Model
}

func (g *Generator) text(ctx context.Context, prompt string, maxTokens int, b model.CourseBrief) (string, error) {
	return g.generateText(ctx, prompt, maxTokens, b, false)
}

// generateText makes one call. jsonMode constrains the reply to JSON (see
// llm.GenerateRequest.JSON); the structured stages set it, chapter prose does
// not.
func (g *Generator) generateText(ctx context.Context, prompt string, maxTokens int, b model.CourseBrief, jsonMode bool) (string, error) {
	resp, err := g.llm.Generate(ctx, llm.GenerateRequest{
		Model:     g.modelFor(b),
		MaxTokens: maxTokens,
		JSON:      jsonMode,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: systemPrompt},
			{Role: llm.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

// json asks for a JSON reply. llm.GenerateJSON retries once with a corrective
// instruction when the reply cannot be decoded, and returns a DecodeError if
// the retry fails too — never more than two calls per stage.
func (g *Generator) json(ctx context.Context, stage, prompt string, maxTokens int, b model.CourseBrief, v any) error {
	return llm.GenerateJSON(ctx, stage, prompt, v,
		func(ctx context.Context, p string) (string, error) { return g.generateText(ctx, p, maxTokens, b, true) },
		func(_ string, err error) {
			g.logger.Warn("course: unparseable JSON, retrying once", zap.String("stage", stage), zap.Error(err))
		},
	)
}

// phase logs the start of a pipeline phase and returns a func that logs its
// outcome and duration. It logs identifiers and counts, never prompt or
// chapter text.
func (g *Generator) phase(job Job, name string, fields ...zap.Field) func(error) {
	fields = append(slices.Clip(fields), zap.String("phase", name))
	if job.RunID != uuid.Nil {
		fields = append(fields, zap.String("course_run_id", job.RunID.String()))
	}
	g.logger.Info("course: phase start", fields...)
	start := time.Now()
	return func(err error) {
		out := append(slices.Clip(fields), zap.Duration("duration", time.Since(start)))
		if err != nil {
			g.logger.Warn("course: phase failed", append(out, zap.Error(err))...)
			return
		}
		g.logger.Info("course: phase done", out...)
	}
}

// stripFence removes a ```markdown wrapper some models put around the reply.
func stripFence(md string) string {
	s := strings.TrimSpace(md)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	nl := strings.IndexByte(s, '\n')
	if nl < 0 || !strings.HasSuffix(s, "```") {
		return s
	}
	lang := strings.TrimSpace(s[3:nl])
	if lang != "" && lang != "markdown" && lang != "md" {
		return s
	}
	return strings.TrimSpace(s[nl+1 : len(s)-3])
}

func capList(items []string, n int) []string {
	out := make([]string, 0, min(len(items), n))
	for _, it := range items {
		if strings.TrimSpace(it) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(it))
		if len(out) == n {
			break
		}
	}
	return out
}

func (h Hooks) withDefaults() Hooks {
	if h.Step == nil {
		h.Step = func(string, string) {}
	}
	if h.Planned == nil {
		h.Planned = func(*model.CourseOutline, []model.CourseSource) error { return nil }
	}
	if h.Cover == nil {
		h.Cover = func(string) error { return nil }
	}
	if h.Chapter == nil {
		h.Chapter = func(*model.CourseChapter) error { return nil }
	}
	if h.Warn == nil {
		h.Warn = func(string) {}
	}
	return h
}
