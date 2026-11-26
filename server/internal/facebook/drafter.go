package facebook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jobshout/server/internal/llm"
)

const maxDraftTokens = 12000

const systemPrompt = `You are an experienced Facebook social media marketing manager and copywriter.
You write original, natural, human-sounding Facebook posts in the Page's brand voice.
You never invent testimonials, customer names, statistics, certifications, awards, prices, discounts, offers, dates or any business claim the brief and notes do not state. If a post would need a fact you were not given, write it without that fact.
You do not use misleading urgency or engagement bait. You use emojis sparingly where they fit and at most three relevant hashtags per post.
You reply with a JSON array only: no preamble, no markdown fences.`

// Categories a post can have, so the calendar mixes them and analytics can
// compare them.
var Categories = []string{
	"promotional", "product", "service", "educational", "informational", "brand", "testimonial",
	"event", "greeting", "offer", "update", "question", "poll", "industry", "campaign",
}

// Draft is one drafted post.
type Draft struct {
	Message      string
	ImagePrompt  string
	Category     string
	Audience     string
	Objective    string
	SuggestedFor *time.Time
}

// DraftRequest asks for a calendar of posts for one Page.
type DraftRequest struct {
	Page      PageRecord
	Objective string
	Count     int
	// Days is the length of the calendar window starting tomorrow.
	Days  int
	Notes string
	// Recent are earlier posts' texts, so the new ones do not repeat them.
	Recent []string
	// Performance is a plain-language hint from earlier results.
	Performance string
	Now         time.Time
}

// Drafter writes calendars with an LLM.
type Drafter struct {
	LLM llm.Client
	Cfg Config
}

// Calendar drafts req.Count posts.
func (d *Drafter) Calendar(ctx context.Context, req DraftRequest) ([]Draft, error) {
	if d == nil || d.LLM == nil {
		return nil, errors.New("facebook: no LLM configured for drafting")
	}
	if req.Count <= 0 {
		return nil, errors.New("facebook: nothing to draft")
	}
	resp, err := d.LLM.Generate(llm.WithStage(ctx, "facebook_calendar"), llm.GenerateRequest{
		Model:     d.Cfg.Model,
		MaxTokens: maxDraftTokens,
		JSON:      true,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: systemPrompt},
			{Role: llm.RoleUser, Content: calendarPrompt(req)},
		},
	})
	if err != nil {
		return nil, err
	}
	return ParseCalendar(resp.Content, req)
}

func calendarPrompt(req DraftRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Plan and write %d Facebook posts for the Page %q", req.Count, req.Page.Name)
	if req.Page.Category != "" {
		fmt.Fprintf(&b, " (category: %s)", req.Page.Category)
	}
	fmt.Fprintf(&b, " spread over the next %d days.\n\n", req.Days)
	if req.Page.Brief != "" {
		b.WriteString("Business brief:\n" + req.Page.Brief + "\n\n")
	} else if req.Page.About != "" || req.Page.Description != "" {
		b.WriteString("What the Page says about itself:\n" + strings.TrimSpace(req.Page.About+"\n"+req.Page.Description) + "\n\n")
	}
	if req.Page.Voice != "" {
		b.WriteString("Brand voice: " + req.Page.Voice + "\n")
	}
	if req.Objective != "" {
		b.WriteString("Campaign objective: " + req.Objective + "\n")
	}
	if req.Notes != "" {
		b.WriteString("\nFacts, offers and events you may use (the only ones you may state):\n" + req.Notes + "\n")
	} else {
		b.WriteString("\nNo offers, prices, events or statistics were given: do not mention any.\n")
	}
	if req.Performance != "" {
		b.WriteString("\nWhat worked before: " + req.Performance + "\n")
	}
	if len(req.Recent) > 0 {
		b.WriteString("\nRecent posts — do not repeat their wording or angle:\n")
		for _, r := range req.Recent {
			b.WriteString("- " + oneLine(r, 160) + "\n")
		}
	}
	fmt.Fprintf(&b, `
Mix categories from: %s. Vary the opening and the call to action.

Return a JSON array of exactly %d objects, each with:
  "category": one category from the list,
  "audience": who it is for, in a few words,
  "objective": what the post should achieve (awareness, engagement, traffic, enquiries, ...),
  "opening": an attention-grabbing first line,
  "body": the useful body, short paragraphs separated by a blank line,
  "cta": one clear call to action,
  "hashtags": up to 3 relevant hashtags, each starting with #,
  "image_prompt": a description of a suitable photo or illustration with no text, letters, numbers, logos or signage in it,
  "day_offset": 1-%d, the day from today to publish,
  "time": "HH:MM" local publishing time (24-hour) when this audience is online.
`, strings.Join(Categories, ", "), req.Count, max(req.Days, 1))
	return b.String()
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// flexString decodes a JSON string, number, bool or array of those: models
// hand back hashtags as a string, a list, or a number now and then, and one
// odd shape must not fail a whole calendar.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*f = flexString(s)
		return nil
	}
	var arr []flexString
	if json.Unmarshal(b, &arr) == nil {
		parts := make([]string, 0, len(arr))
		for _, a := range arr {
			parts = append(parts, string(a))
		}
		*f = flexString(strings.Join(parts, " "))
		return nil
	}
	var n json.Number
	if json.Unmarshal(b, &n) == nil {
		*f = flexString(n.String())
		return nil
	}
	*f = ""
	return nil
}

type rawDraft struct {
	Category    flexString `json:"category"`
	Audience    flexString `json:"audience"`
	Objective   flexString `json:"objective"`
	Opening     flexString `json:"opening"`
	Body        flexString `json:"body"`
	CTA         flexString `json:"cta"`
	Hashtags    flexString `json:"hashtags"`
	ImagePrompt flexString `json:"image_prompt"`
	DayOffset   flexString `json:"day_offset"`
	Time        flexString `json:"time"`
}

var (
	thinkBlock = regexp.MustCompile(`(?s)<think>.*?</think>`)
	codeFence  = regexp.MustCompile("(?m)^```[a-zA-Z]*\\s*$")
	hashtagRe  = regexp.MustCompile(`#[\p{L}\p{N}_]+`)
	hhmm       = regexp.MustCompile(`^(\d{1,2}):(\d{2})`)
)

// ParseCalendar decodes a model reply into drafts. Posts with no usable text
// are dropped; the caller marks the rows left over as failed.
func ParseCalendar(content string, req DraftRequest) ([]Draft, error) {
	s := thinkBlock.ReplaceAllString(content, "")
	s = codeFence.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	start, end := strings.Index(s, "["), strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		// Some models wrap the array: {"posts":[...]}.
		var wrap map[string]json.RawMessage
		if json.Unmarshal([]byte(s), &wrap) == nil {
			for _, v := range wrap {
				if t := strings.TrimSpace(string(v)); strings.HasPrefix(t, "[") {
					s, start, end = t, 0, len(t)-1
					break
				}
			}
		}
	}
	if start < 0 || end <= start {
		return nil, errors.New("facebook: the model did not return a list of posts")
	}
	var raws []rawDraft
	if err := json.Unmarshal([]byte(s[start:end+1]), &raws); err != nil {
		return nil, fmt.Errorf("facebook: could not read the model's posts: %w", err)
	}
	loc := time.UTC
	if l, err := time.LoadLocation(req.Page.Timezone); err == nil && req.Page.Timezone != "" {
		loc = l
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	out := make([]Draft, 0, len(raws))
	for i, r := range raws {
		msg := compose(r)
		if msg == "" {
			continue
		}
		cat := strings.ToLower(strings.TrimSpace(string(r.Category)))
		if !validCategory(cat) {
			cat = "informational"
		}
		at := suggestTime(now, loc, string(r.DayOffset), string(r.Time), i, len(raws), req.Days)
		out = append(out, Draft{
			Message:      FitMessage(msg),
			ImagePrompt:  strings.TrimSpace(string(r.ImagePrompt)),
			Category:     cat,
			Audience:     oneLine(string(r.Audience), 200),
			Objective:    oneLine(string(r.Objective), 200),
			SuggestedFor: &at,
		})
	}
	if len(out) == 0 {
		return nil, errors.New("facebook: the model returned no usable posts")
	}
	return out, nil
}

func validCategory(c string) bool {
	for _, k := range Categories {
		if k == c {
			return true
		}
	}
	return false
}

// compose joins the parts into the post text: opening, body, call to action,
// then at most three hashtags.
func compose(r rawDraft) string {
	var parts []string
	for _, p := range []flexString{r.Opening, r.Body, r.CTA} {
		if t := strings.TrimSpace(string(p)); t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	tags := hashtagRe.FindAllString(string(r.Hashtags), -1)
	if len(tags) > 3 {
		tags = tags[:3]
	}
	if len(tags) > 0 {
		parts = append(parts, strings.Join(tags, " "))
	}
	return strings.Join(parts, "\n\n")
}

// suggestTime turns the model's day offset and clock time into a future
// instant on the Page's clock. A missing or silly value falls back to an even
// spread at 10:00, so a calendar is never left unscheduled or in the past.
func suggestTime(now time.Time, loc *time.Location, dayStr, clock string, i, n, days int) time.Time {
	if days <= 0 {
		days = 7
	}
	day, err := strconv.Atoi(strings.TrimSpace(dayStr))
	if err != nil || day < 1 || day > days {
		day = 1 + i*days/max(n, 1)
	}
	h, m := 10, 0
	if mm := hhmm.FindStringSubmatch(strings.TrimSpace(clock)); mm != nil {
		hh, _ := strconv.Atoi(mm[1])
		mi, _ := strconv.Atoi(mm[2])
		if hh < 24 && mi < 60 {
			h, m = hh, mi
		}
	}
	local := now.In(loc)
	t := time.Date(local.Year(), local.Month(), local.Day()+day, h, m, 0, 0, loc)
	for !t.After(now.Add(15 * time.Minute)) {
		t = t.AddDate(0, 0, 1)
	}
	return t.UTC()
}

// FitMessage keeps text within MessageMax at a paragraph boundary.
func FitMessage(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	rs := []rune(s)
	if len(rs) <= MessageMax {
		return s
	}
	cut := string(rs[:MessageMax])
	if i := strings.LastIndex(cut, "\n\n"); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut)
}

// ImagePrompt wraps a post's image idea with the house rules for creatives:
// on-brand colours, no text or logos drawn by the model (generated lettering
// is unreliable and distorts logos), square feed format.
func ImagePrompt(idea string, p PageRecord) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(idea))
	b.WriteString(". Professional social media photograph or clean illustration, square 1:1 composition, uncluttered, good contrast.")
	if p.Voice != "" {
		b.WriteString(" Mood to match this brand voice: " + oneLine(p.Voice, 120) + ".")
	}
	b.WriteString(" No text, letters, numbers, captions, logos, watermarks or signage anywhere in the image.")
	return b.String()
}
