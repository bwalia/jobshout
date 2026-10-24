package marketingemail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

// Limits the structured draft must meet.
const (
	maxSubject      = 70
	minPreheader    = 40
	maxPreheader    = 110
	minSections     = 2
	maxSections     = 6
	draftMaxTokens  = 6000
	maxHeadline     = 120
	maxSectionItems = 8
)

// Completer is the one LLM call the writer needs: a prompt in, text out.
// jsonMode asks the provider for a JSON-constrained reply.
type Completer interface {
	Complete(ctx context.Context, model, prompt string, maxTokens int, jsonMode bool) (string, error)
}

// clientCompleter adapts an llm.Client (which has Generate, not Complete).
type clientCompleter struct {
	client       llm.Client
	defaultModel string
}

// NewClientCompleter adapts an llm.Client to Completer. defaultModel is used
// when a call names none.
func NewClientCompleter(c llm.Client, defaultModel string) Completer {
	if c == nil {
		return nil
	}
	return &clientCompleter{client: c, defaultModel: defaultModel}
}

func (c *clientCompleter) Complete(ctx context.Context, modelName, prompt string, maxTokens int, jsonMode bool) (string, error) {
	if strings.TrimSpace(modelName) == "" {
		modelName = c.defaultModel
	}
	resp, err := c.client.Generate(ctx, llm.GenerateRequest{
		Model:       modelName,
		MaxTokens:   maxTokens,
		Temperature: 0.7,
		JSON:        jsonMode,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: systemPrompt},
			{Role: llm.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// Writer turns a brief into a validated structured draft. The model never
// writes HTML: Render does that from the draft.
type Writer struct {
	LLM   Completer
	Model string
}

// ValidationError lists why a draft was rejected.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return "the email draft is invalid: " + strings.Join(e.Problems, "; ")
}

const systemPrompt = `You are a senior B2B email copywriter. You write one marketing email as structured JSON — never HTML.
You never invent prices, specifications, discounts, dates or customer quotes: use only facts in the brief.
You reply with a single JSON object and nothing else.`

// tradeFacts are the confirmed Workstation Trade membership facts. They are
// offered to the model only when the brief is about Trade, so it never has to
// invent a price or a benefit.
const tradeFacts = `Confirmed Workstation Trade membership facts (use only these for Trade; do not add others):
- Founding price from £75/month or £750/year ex VAT; 100 founding places.
- Unlimited AI engineer time by phone, chat and email.
- Money off qualifying orders.
- A 60-day quote price lock.
- 24-hour early access to new stock.
- Cancel in two clicks.
- Engineer phone: +44 7515 356 146.
- Landing page: https://shop.workstation.co.uk/trade
- Chat: https://shop.workstation.co.uk/chat
`

var tradeRe = regexp.MustCompile(`(?i)\btrade\b`)

// aboutTrade reports whether the brief is about Trade membership.
func aboutTrade(b model.MarketingEmailBrief) bool {
	return tradeRe.MatchString(b.Brief + " " + b.Products + " " + b.PrimaryCTAURL + " " + b.PrimaryCTALabel)
}

func draftPrompt(b model.MarketingEmailBrief) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Write a marketing email about:\n%s\n\n", strings.TrimSpace(b.Brief))
	if aboutTrade(b) {
		sb.WriteString(tradeFacts + "\n")
	}
	if b.Audience != "" {
		fmt.Fprintf(&sb, "Audience: %s\n", b.Audience)
	}
	if b.Goal != "" {
		fmt.Fprintf(&sb, "Goal: %s\n", b.Goal)
	}
	if b.Tone != "" {
		fmt.Fprintf(&sb, "Tone: %s\n", b.Tone)
	}
	if b.Products != "" {
		fmt.Fprintf(&sb, "Products to feature (shop URLs or slugs): %s\n", b.Products)
	}
	if b.PrimaryCTALabel != "" || b.PrimaryCTAURL != "" {
		fmt.Fprintf(&sb, "Primary call to action: label %q, URL %s — use it exactly in a \"cta\" section.\n", b.PrimaryCTALabel, b.PrimaryCTAURL)
	}
	fmt.Fprintf(&sb, `
Reply with JSON of exactly this shape:
{
  "subject": "at most %d characters, specific, no ALL CAPS",
  "preheader": "%d-%d characters, continues the subject, does not repeat it",
  "title": "short internal title",
  "slug_hint": "kebab-case-slug",
  "hero": {"headline": "...", "subline": "...", "image_prompt": "a concrete picture to draw: subject, setting, light, no text or logos"},
  "sections": [
    {"type": "text|feature|product|quote|cta", "heading": "...", "body_markdown": "...", "items": ["..."], "cta_label": "...", "cta_url": "https://..."}
  ],
  "footer_note": "optional one short line"
}

Rules:
- %d to %d sections. End with one "cta" section that has cta_label and cta_url.
- body_markdown allows only paragraphs, **bold**, *italic*, [links](https://...) and "- " lists. No HTML.
- Every URL must start with https://. Never use merge tags such as {{name}}.
- A "feature" lists its points in items; a "product" names one product and may link to it; a "quote" is only a quote the brief supplies.
- Do not open with a greeting such as "Hi" or "Dear": the email adds "Hi <first name>," itself.
- Plain, concrete sentences. One idea per section.
`, maxSubject, minPreheader, maxPreheader, minSections, maxSections)
	return sb.String()
}

// Draft asks the model for the structured draft, validates it, and — when it
// is invalid — retries once with the problems named in the prompt.
func (w *Writer) Draft(ctx context.Context, b model.MarketingEmailBrief) (*model.MarketingEmailDraft, error) {
	if w == nil || w.LLM == nil {
		return nil, errors.New("marketingemail: no LLM configured to write the email")
	}
	modelName := firstNonEmpty(b.Model, w.Model)
	prompt := draftPrompt(b)

	d, err := w.generate(ctx, modelName, prompt)
	if err != nil {
		return nil, err
	}
	problems := Validate(d)
	if len(problems) == 0 {
		return d, nil
	}

	retryPrompt := prompt + "\n\nYour previous draft was rejected for these reasons — fix every one and reply with the full corrected JSON:\n- " +
		strings.Join(problems, "\n- ")
	d2, err := w.generate(llm.WithStage(ctx, "marketing_email_retry"), modelName, retryPrompt)
	if err != nil {
		return nil, err
	}
	if problems := Validate(d2); len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	return d2, nil
}

func (w *Writer) generate(ctx context.Context, modelName, prompt string) (*model.MarketingEmailDraft, error) {
	var raw rawDraft
	err := llm.GenerateJSON(ctx, "marketing_email_draft", prompt, &raw,
		func(ctx context.Context, p string) (string, error) {
			return w.LLM.Complete(ctx, modelName, p, draftMaxTokens, true)
		}, nil)
	if err != nil {
		return nil, err
	}
	return raw.draft(), nil
}

// flexibleString accepts whatever shape a model gives a text field: a string,
// a number or bool, a list (joined with ", "), or an object (its string values
// joined). Anything else is empty — an odd shape must not fail a whole run.
type flexibleString string

func (s *flexibleString) UnmarshalJSON(b []byte) error {
	*s = flexibleString(strings.TrimSpace(flexText(b)))
	return nil
}

func flexText(b []byte) string {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return ""
	}
	return flexValue(v)
}

func flexValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64, bool:
		return fmt.Sprint(t)
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			if p := strings.TrimSpace(flexValue(x)); p != "" {
				parts = append(parts, p)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		parts := make([]string, 0, len(t))
		for _, k := range sortedKeys(t) {
			if p := strings.TrimSpace(flexValue(t[k])); p != "" {
				parts = append(parts, p)
			}
		}
		return strings.Join(parts, " — ")
	}
	return ""
}

// flexibleList accepts a list of strings, a list of objects, or one string.
type flexibleList []string

func (l *flexibleList) UnmarshalJSON(b []byte) error {
	*l = nil
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if s := strings.TrimSpace(flexValue(x)); s != "" {
				*l = append(*l, s)
			}
		}
	case string:
		for _, line := range strings.Split(t, "\n") {
			if s := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*• ")); s != "" {
				*l = append(*l, s)
			}
		}
	}
	return nil
}

type rawHero struct {
	Headline    flexibleString `json:"headline"`
	Subline     flexibleString `json:"subline"`
	ImagePrompt flexibleString `json:"image_prompt"`
}

type rawSection struct {
	Type         flexibleString `json:"type"`
	Heading      flexibleString `json:"heading"`
	BodyMarkdown flexibleString `json:"body_markdown"`
	Items        flexibleList   `json:"items"`
	CTALabel     flexibleString `json:"cta_label"`
	CTAURL       flexibleString `json:"cta_url"`
}

type rawDraft struct {
	Subject    flexibleString `json:"subject"`
	Preheader  flexibleString `json:"preheader"`
	Title      flexibleString `json:"title"`
	SlugHint   flexibleString `json:"slug_hint"`
	Hero       rawHero        `json:"hero"`
	Sections   []rawSection   `json:"sections"`
	FooterNote flexibleString `json:"footer_note"`
}

// draft converts the tolerant decode to the model type, normalising section
// types (unknown types become "text").
func (r rawDraft) draft() *model.MarketingEmailDraft {
	d := &model.MarketingEmailDraft{
		Subject:    string(r.Subject),
		Preheader:  string(r.Preheader),
		Title:      string(r.Title),
		SlugHint:   string(r.SlugHint),
		FooterNote: string(r.FooterNote),
		Hero: model.MarketingEmailHero{
			Headline:    string(r.Hero.Headline),
			Subline:     string(r.Hero.Subline),
			ImagePrompt: string(r.Hero.ImagePrompt),
		},
		Sections: []model.MarketingEmailSection{},
	}
	if d.Title == "" {
		d.Title = d.Subject
	}
	for _, s := range r.Sections {
		typ := strings.ToLower(strings.TrimSpace(string(s.Type)))
		switch typ {
		case model.EmailSectionText, model.EmailSectionFeature, model.EmailSectionProduct,
			model.EmailSectionQuote, model.EmailSectionCTA:
		default:
			typ = model.EmailSectionText
		}
		d.Sections = append(d.Sections, model.MarketingEmailSection{
			Type:         typ,
			Heading:      string(s.Heading),
			BodyMarkdown: string(s.BodyMarkdown),
			Items:        []string(s.Items),
			CTALabel:     string(s.CTALabel),
			CTAURL:       string(s.CTAURL),
		})
	}
	return d
}

// Validate lists everything wrong with a draft; empty means it is good to
// render. Rules: subject ≤ 70, preheader 40–110, every URL https, 2–6
// sections, no merge tags.
func Validate(d *model.MarketingEmailDraft) []string {
	if d == nil {
		return []string{"no draft"}
	}
	var p []string
	add := func(format string, a ...any) { p = append(p, fmt.Sprintf(format, a...)) }

	subject := strings.TrimSpace(d.Subject)
	switch n := utf8.RuneCountInString(subject); {
	case n == 0:
		add("subject is empty")
	case n > maxSubject:
		add("subject is %d characters; the limit is %d", n, maxSubject)
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(d.Preheader)); n < minPreheader || n > maxPreheader {
		add("preheader is %d characters; it must be %d to %d", n, minPreheader, maxPreheader)
	}
	if strings.TrimSpace(d.Hero.Headline) == "" {
		add("hero.headline is empty")
	} else if utf8.RuneCountInString(d.Hero.Headline) > maxHeadline {
		add("hero.headline is longer than %d characters", maxHeadline)
	}
	if n := len(d.Sections); n < minSections || n > maxSections {
		add("there are %d sections; there must be %d to %d", n, minSections, maxSections)
	}

	for i, s := range d.Sections {
		where := fmt.Sprintf("sections[%d]", i)
		if strings.TrimSpace(s.Heading) == "" && strings.TrimSpace(s.BodyMarkdown) == "" && len(s.Items) == 0 {
			add("%s has no heading, body or items", where)
		}
		if len(s.Items) > maxSectionItems {
			add("%s has %d items; the limit is %d", where, len(s.Items), maxSectionItems)
		}
		if cta, u := strings.TrimSpace(s.CTALabel), strings.TrimSpace(s.CTAURL); cta != "" || u != "" || s.Type == model.EmailSectionCTA {
			if cta == "" {
				add("%s needs a cta_label", where)
			}
			if u == "" {
				add("%s needs a cta_url", where)
			} else if !IsHTTPS(u) {
				add("%s cta_url %q must be an https URL", where, u)
			}
		}
		for _, bad := range nonHTTPSLinks(s.BodyMarkdown) {
			add("%s links to %q; every URL must be https", where, bad)
		}
	}

	for _, f := range []struct{ name, v string }{
		{"subject", d.Subject}, {"preheader", d.Preheader}, {"hero", d.Hero.Headline + d.Hero.Subline},
		{"footer_note", d.FooterNote}, {"sections", sectionsText(d.Sections)},
	} {
		if strings.Contains(f.v, "{{") || strings.Contains(f.v, "{%") {
			add("%s contains a merge tag or template marker ({{ or {%%); remove it", f.name)
		}
	}
	return p
}

func sectionsText(ss []model.MarketingEmailSection) string {
	var sb strings.Builder
	for _, s := range ss {
		sb.WriteString(s.Heading)
		sb.WriteByte(' ')
		sb.WriteString(s.BodyMarkdown)
		sb.WriteByte(' ')
		sb.WriteString(strings.Join(s.Items, " "))
		sb.WriteByte(' ')
		sb.WriteString(s.CTALabel)
		sb.WriteByte(' ')
	}
	return sb.String()
}

// IsHTTPS reports whether raw is an absolute https URL with a host.
func IsHTTPS(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.Scheme == "https" && u.Host != ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
