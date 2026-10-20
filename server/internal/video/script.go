package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

// Brief is a launch's values, resolved.
type Brief struct {
	Topic        string
	Kind         string // explainer | anime | product
	Seconds      int
	Format       string // landscape | portrait
	Voice        string
	Style        string
	Motion       bool
	Captions     bool
	Notes        string
	ProductName  string
	ProductImage string // URL of an uploaded product photo (product videos)
}

// Script is the film on paper: a title and the shots that make it up.
type Script struct {
	Title string
	Shots []Shot
}

// wordsPerSecond is Kokoro's pace at normal speed, measured: 26 words took
// 10.1 seconds. The narration's length is the video's length, so this is what
// turns "a 60-second video" into a word budget the writer can aim at.
const wordsPerSecond = 2.5

// secondsPerShot keeps cuts at a pace a viewer can follow and inside what one
// animated clip covers well (~5 s, held a little longer by a still frame).
const secondsPerShot = 6

// These match the workstation's request limits, which reject anything longer.
const (
	maxShots = 30 // VIDEO_MAX_SHOTS
	maxTitle = 200
	maxStyle = 500
)

// ShotCount is how many shots a video of this length is written in.
func ShotCount(seconds int) int {
	return min(max(seconds/secondsPerShot, 3), maxShots)
}

// Language is the narration language a Kokoro voice speaks: the first letter
// of its name is its language.
func Language(voice string) string {
	switch {
	case strings.HasPrefix(voice, "h"):
		return "Hindi (Devanagari script)"
	case strings.HasPrefix(voice, "b"):
		return "British English"
	default:
		return "American English"
	}
}

const systemPrompt = `You are a scriptwriter and director for short narrated videos.
You write the spoken script and, for each line of it, the one picture the viewer sees while it is spoken.
You reply with a single JSON object and nothing else.`

func scriptPrompt(b Brief) string {
	shots := ShotCount(b.Seconds)
	format := "landscape 16:9, for a desktop or TV screen"
	if b.Format == "portrait" {
		format = "portrait 9:16, for a phone held upright — frame subjects tall and centred"
	}
	style := b.Style
	if style == "" {
		style = DefaultStyle(b.Kind)
	}
	if style == "" {
		style = "choose one that suits the subject, and keep it identical in every shot"
	}
	var sb strings.Builder
	switch b.Kind {
	case model.VideoKindProduct:
		if b.ProductName != "" {
			fmt.Fprintf(&sb, "Write a short promotional video for a product called %q.\n", b.ProductName)
		} else {
			fmt.Fprintf(&sb, "Write a short promotional video for this product: %s\n", b.Topic)
		}
		fmt.Fprintf(&sb, "What it is / what to cover: %s\n\n", b.Topic)
	case model.VideoKindAnime:
		fmt.Fprintf(&sb, "Write a short anime-style animated video about: %s\n\n", b.Topic)
	default:
		fmt.Fprintf(&sb, "Write a narrated video about: %s\n\n", b.Topic)
	}
	fmt.Fprintf(&sb, "Length: about %d seconds of narration — about %d words in total, across %d shots.\n",
		b.Seconds, int(float64(b.Seconds)*wordsPerSecond), shots)
	fmt.Fprintf(&sb, "Format: %s.\n", format)
	fmt.Fprintf(&sb, "Narration language: %s.\n", Language(b.Voice))
	fmt.Fprintf(&sb, "Visual style: %s.\n", style)
	if b.Notes != "" {
		fmt.Fprintf(&sb, "Notes from the requester: %s\n", b.Notes)
	}
	sb.WriteString(kindGuidance(b.Kind))
	return sb.String()
}

// kindGuidance is the JSON shape and the rules, tuned to the video's kind. The
// shared rules differ only where the kind needs them to: product videos sell
// and tag their product shots; anime describes animated scenes; the default is
// the original narrated documentary.
func kindGuidance(kind string) string {
	switch kind {
	case model.VideoKindProduct:
		return `
Return JSON in exactly this shape:
{"title": "a short title", "shots": [{"narration": "...", "visual": "...", "product": true}]}

Rules for "narration":
- Spoken ad copy for that shot: one or two short sentences, 8 to 20 words, benefit-led and energetic.
- The first shot hooks with the single biggest reason to care. The last shot ends on a clear call to action.
- Plain spoken language. No stage directions, shot numbers, emojis or hashtags.
- State only claims you are confident are true. Never invent statistics, prices, awards or quotes.

Rules for "visual":
- A self-contained description an image model can draw on its own, 25 to 60 words, always in English: subject, setting, composition, camera angle, lighting and colour palette.
- Set "product": true for every shot whose main subject IS the product (a hero shot). Set "product": false for supporting or lifestyle shots that set a mood or show it in use without being a clean shot of the product itself.
- Describe the product the same distinctive way in every "product": true shot so it looks consistent.
- No written words, letters, numbers, captions, logos or signs in the picture, and no real, identifiable people.
- Vary the framing between wide, medium and close-up shots.`
	case model.VideoKindAnime:
		return `
Return JSON in exactly this shape:
{"title": "a short title", "shots": [{"narration": "...", "visual": "..."}]}

Rules for "narration":
- What the narrator says during that shot: one or two short spoken sentences, 8 to 20 words.
- The first shot hooks the viewer in its first sentence. The last shot lands a clear ending.
- Plain spoken language. No stage directions, shot numbers, emojis or hashtags.
- State only facts you are confident are true. Never invent statistics, quotes or names.

Rules for "visual":
- A self-contained description of an anime-style animated scene an image model can draw, 25 to 60 words, always in English: character or subject, action, setting, composition, camera angle, lighting and colour palette.
- Recurring characters are described with the same distinctive hair, outfit, colours and features every time, so they look the same in every shot.
- No written words, letters, numbers, captions, logos or signs in the picture, and no real, identifiable people.
- Vary the framing between wide establishing shots, medium shots and close-ups.`
	default:
		return `
Return JSON in exactly this shape:
{"title": "a short title", "shots": [{"narration": "...", "visual": "..."}]}

Rules for "narration":
- What the narrator says during that shot: one or two short spoken sentences, 8 to 20 words.
- The first shot hooks the viewer in its first sentence. The last shot lands a clear ending.
- Plain spoken language. No stage directions, shot numbers, emojis or hashtags.
- State only facts you are confident are true. Never invent statistics, quotes or names.

Rules for "visual":
- A self-contained description an image model can draw on its own: subject, action, setting, composition, camera angle, lighting and colour palette, in 25 to 60 words. Always in English.
- Show concretely what the narration is about. Prefer a scene over an abstract concept.
- A recurring person, animal or place is described with the same distinctive details every time it appears, so it looks the same in every shot.
- No written words, letters, numbers, captions, logos or signs in the picture, and no real, identifiable people.
- Vary the framing between wide, medium and close-up shots.`
	}
}

// DefaultStyle is the visual style a kind falls back to when the requester
// leaves the style field blank. Explainer keeps the old "let the writer choose"
// behaviour (empty string).
func DefaultStyle(kind string) string {
	switch kind {
	case model.VideoKindAnime:
		return "anime style, cel-shaded, vibrant saturated colours, clean linework, detailed backgrounds, dramatic cinematic lighting"
	case model.VideoKindProduct:
		return "clean studio product photography, soft even lighting, shallow depth of field, seamless neutral background, crisp focus on the product"
	default:
		return ""
	}
}

// writeRetryWait is the base pause between script attempts. A variable for tests.
var writeRetryWait = time.Minute

// Writer turns a brief into a script with an LLM.
type Writer struct {
	LLM   llm.Client
	Model string
}

// Write asks for the script, once more if the first answer is unusable.
func (w *Writer) Write(ctx context.Context, b Brief) (*Script, error) {
	if w == nil || w.LLM == nil {
		return nil, errors.New("video: no LLM configured to write the script")
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			// A shared Ollama at capacity answers "server busy" for minutes at
			// a time; the client's own single quick retry does not outlast it.
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("video: script writing failed: %w", lastErr)
			case <-time.After(time.Duration(attempt) * writeRetryWait):
			}
		}
		resp, err := w.LLM.Generate(llm.WithStage(ctx, "video_script"), llm.GenerateRequest{
			Model:       w.Model,
			MaxTokens:   8000,
			Temperature: 0.7,
			JSON:        true,
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: systemPrompt},
				{Role: llm.RoleUser, Content: scriptPrompt(b)},
			},
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			lastErr = err
			continue
		}
		script, err := ParseScript(resp.Content)
		if err != nil {
			lastErr = err
			continue
		}
		if script.Title == "" {
			script.Title = truncate(b.Topic, 80)
		}
		return script, nil
	}
	return nil, fmt.Errorf("video: script writing failed: %w", lastErr)
}

// text decodes a string-ish JSON value tolerantly: a model that answers a
// string field with a list or a number should not cost a whole run.
type text string

func (t *text) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	var parts []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			if s := strings.TrimSpace(x); s != "" {
				parts = append(parts, s)
			}
		case float64:
			parts = append(parts, fmt.Sprint(x))
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(v)
	*t = text(strings.Join(parts, " "))
	return nil
}

type rawShot struct {
	Narration    text  `json:"narration"`
	Voiceover    text  `json:"voiceover"`
	Text         text  `json:"text"`
	Visual       text  `json:"visual"`
	VisualPrompt text  `json:"visual_prompt"`
	ImagePrompt  text  `json:"image_prompt"`
	Description  text  `json:"description"`
	Product      *bool `json:"product"`
	ShowsProduct *bool `json:"shows_product"`
	IsProduct    *bool `json:"is_product"`
}

// showsProduct reads whichever of the product flags the model used.
func (s rawShot) showsProduct() bool {
	for _, v := range []*bool{s.Product, s.ShowsProduct, s.IsProduct} {
		if v != nil {
			return *v
		}
	}
	return false
}

// ParseScript reads the writer's JSON, accepting the field names models drift
// to, and keeps only shots that have both something to say and something to
// show.
func ParseScript(content string) (*Script, error) {
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, errors.New("the reply held no JSON object")
	}
	var raw struct {
		Title  text      `json:"title"`
		Shots  []rawShot `json:"shots"`
		Scenes []rawShot `json:"scenes"`
	}
	if err := json.Unmarshal([]byte(content[start:end+1]), &raw); err != nil {
		return nil, fmt.Errorf("the reply was not valid JSON: %w", err)
	}
	if len(raw.Shots) == 0 {
		raw.Shots = raw.Scenes
	}
	script := &Script{Title: truncate(string(raw.Title), maxTitle)}
	for _, s := range raw.Shots {
		narration := firstText(s.Narration, s.Voiceover, s.Text)
		visual := firstText(s.Visual, s.VisualPrompt, s.ImagePrompt, s.Description)
		if narration == "" || visual == "" {
			continue
		}
		script.Shots = append(script.Shots, Shot{Narration: truncate(narration, 800), Visual: truncate(visual, 2000), ShowsProduct: s.showsProduct()})
	}
	if len(script.Shots) == 0 {
		return nil, errors.New("the script had no shot with both narration and a visual")
	}
	if len(script.Shots) > maxShots {
		script.Shots = script.Shots[:maxShots]
	}
	return script, nil
}

func firstText(values ...text) string {
	for _, v := range values {
		if s := strings.TrimSpace(string(v)); s != "" {
			return s
		}
	}
	return ""
}

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// model is the brief as a run records it.
func (b Brief) model() model.VideoBrief {
	return model.VideoBrief{Kind: b.Kind, Seconds: b.Seconds, Format: b.Format, Voice: b.Voice, Style: b.Style,
		Motion: b.Motion, Captions: b.Captions, Notes: b.Notes, ProductName: b.ProductName, ProductImage: b.ProductImage}
}
