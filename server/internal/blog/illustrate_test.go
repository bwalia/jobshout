package blog

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// fakeIllustrator draws nothing and records what it was asked for.
type fakeIllustrator struct {
	enabled bool
	calls   []IllustrationRequest
	// failAfter makes generation fail once this many images have been drawn,
	// so the "a picture could not be drawn" path can be exercised.
	failAfter int
	// noURL simulates a generator that works but has nowhere to store output.
	noURL bool
	// provider overrides the reported provider. Empty means gemini, which is
	// the path that can letter labels.
	provider string
}

func (f *fakeIllustrator) Enabled() bool { return f.enabled }

func (f *fakeIllustrator) Generate(ctx context.Context, req IllustrationRequest) (*Illustration, error) {
	f.calls = append(f.calls, req)
	if f.failAfter > 0 && len(f.calls) > f.failAfter {
		return nil, fmt.Errorf("the GPU is busy")
	}
	url := fmt.Sprintf("/api/v1/images/file/img-%d.png", len(f.calls))
	if f.noURL {
		url = ""
	}
	model := req.Model
	if model == "" {
		model = "gemini-3.1-flash-lite-image"
	}
	provider := f.provider
	if provider == "" {
		provider = "gemini"
	}
	return &Illustration{
		URL: url, Provider: provider, Model: model,
		Seed: int64(len(f.calls)), Width: req.Width, Height: req.Height,
	}, nil
}

func testRunner(images Illustrator) *Runner {
	r := &Runner{logger: zap.NewNop()}
	return r.WithIllustrator(images)
}

func TestIllustrateBody_ReplacesBlocksWithImages(t *testing.T) {
	fake := &fakeIllustrator{enabled: true}
	r := testRunner(fake)

	markdown := "Intro paragraph.\n\n" +
		"```illustration\nA control plane reconciling desired state\n```\n\n" +
		"Closing paragraph.\n"

	out, notes := r.illustrateBody(context.Background(), uuid.New(), markdown)

	if strings.Contains(out, "```illustration") {
		t.Errorf("the fenced block survived into the output:\n%s", out)
	}
	if !strings.Contains(out, "![A control plane reconciling desired state](/api/v1/images/file/img-1.png)") {
		t.Errorf("image markdown missing:\n%s", out)
	}
	if !strings.Contains(out, "Intro paragraph.") || !strings.Contains(out, "Closing paragraph.") {
		t.Errorf("surrounding prose was damaged:\n%s", out)
	}
	if len(notes) != 0 {
		t.Errorf("unexpected notes: %v", notes)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("made %d generation calls, want 1", len(fake.calls))
	}
	if fake.calls[0].Source != "blog_inline" {
		t.Errorf("source = %q, want blog_inline", fake.calls[0].Source)
	}
	prompt := fake.calls[0].Prompt
	if strings.Contains(prompt, "Strictly no text") {
		t.Error("inline prompt still bans labels")
	}
	if !strings.Contains(prompt, "informational") && !strings.Contains(prompt, "labels") {
		t.Errorf("inline prompt is not asking for a labeled figure:\n%s", prompt)
	}
	if !fake.calls[0].NoFallback {
		t.Error("inline figures must not fall back to workstation diffusion")
	}
}

func TestIllustrateBody_TypedFenceSetsKindAndSize(t *testing.T) {
	fake := &fakeIllustrator{enabled: true}
	r := testRunner(fake)

	markdown := "```illustration comparison\nPolling vs webhooks: latency, cost, failure modes\n```\n"
	out, notes := r.illustrateBody(context.Background(), uuid.New(), markdown)

	if strings.Contains(out, "```illustration") {
		t.Errorf("typed fence survived:\n%s", out)
	}
	if !strings.Contains(out, "![Polling vs webhooks: latency, cost, failure modes]") {
		t.Errorf("alt text lost the facts:\n%s", out)
	}
	if len(notes) != 0 {
		t.Errorf("unexpected notes: %v", notes)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("made %d calls, want 1", len(fake.calls))
	}
	if fake.calls[0].Width != 1280 || fake.calls[0].Height != 720 {
		t.Errorf("comparison size = %dx%d, want 1280x720", fake.calls[0].Width, fake.calls[0].Height)
	}
	if !strings.Contains(fake.calls[0].Prompt, "comparison table") {
		t.Errorf("prompt did not ask for a comparison table:\n%s", fake.calls[0].Prompt)
	}
	if !strings.Contains(fake.calls[0].Prompt, "latency") {
		t.Errorf("prompt lost the article facts:\n%s", fake.calls[0].Prompt)
	}
}

func TestIllustrateBody_DropsWorkstationLettering(t *testing.T) {
	fake := &fakeIllustrator{enabled: true, provider: "mflux"}
	r := testRunner(fake)
	out, notes := r.illustrateBody(context.Background(), uuid.New(),
		"```illustration comparison\nPolling vs webhooks: latency\n```\n")
	if strings.Contains(out, "![") {
		t.Errorf("mflux output must not land in the article:\n%s", out)
	}
	if len(notes) != 1 {
		t.Errorf("got %d notes, want 1: %v", len(notes), notes)
	}
}

// Each image costs tens of seconds on one shared GPU, so an article that asks
// for nine must not get nine.
func TestIllustrateBody_EnforcesTheLimit(t *testing.T) {
	fake := &fakeIllustrator{enabled: true}
	r := testRunner(fake)

	var b strings.Builder
	for i := 0; i < maxInlineIllustrations+3; i++ {
		fmt.Fprintf(&b, "```illustration\npicture %d\n```\n\n", i)
	}

	out, notes := r.illustrateBody(context.Background(), uuid.New(), b.String())

	if len(fake.calls) != maxInlineIllustrations {
		t.Errorf("drew %d images, want the cap of %d", len(fake.calls), maxInlineIllustrations)
	}
	if strings.Contains(out, "```illustration") {
		t.Errorf("a dropped block was left in the output as raw syntax:\n%s", out)
	}
	if len(notes) != 3 {
		t.Errorf("got %d notes, want 3 (one per dropped block): %v", len(notes), notes)
	}
}

// A picture that cannot be drawn must leave no trace: the reader should not be
// shown a fenced description of a picture that is not there.
func TestIllustrateBody_DropsWhatItCannotDraw(t *testing.T) {
	fake := &fakeIllustrator{enabled: true, failAfter: 1}
	r := testRunner(fake)

	markdown := "```illustration\nfirst\n```\n\n```illustration\nsecond\n```\n"

	out, notes := r.illustrateBody(context.Background(), uuid.New(), markdown)

	if strings.Contains(out, "```illustration") || strings.Contains(out, "second") {
		t.Errorf("the failed block leaked into the output:\n%s", out)
	}
	if !strings.Contains(out, "img-1.png") {
		t.Errorf("the successful image was lost:\n%s", out)
	}
	if len(notes) != 1 {
		t.Errorf("got %d notes, want 1: %v", len(notes), notes)
	}
}

// Markdown with no illustration blocks must come back byte-identical, and must
// not touch the generator at all.
func TestIllustrateBody_LeavesPlainMarkdownAlone(t *testing.T) {
	fake := &fakeIllustrator{enabled: true}
	r := testRunner(fake)

	markdown := "# Title\n\nSome prose.\n\n```mermaid\nflowchart TD\n A --> B\n```\n"
	out, notes := r.illustrateBody(context.Background(), uuid.New(), markdown)

	if out != markdown {
		t.Errorf("markdown was modified:\n%s", out)
	}
	if len(notes) != 0 || len(fake.calls) != 0 {
		t.Errorf("notes=%v calls=%d — nothing should have happened", notes, len(fake.calls))
	}
}

// A description containing brackets must not break out of its own alt text.
func TestEscapeAlt_NeutralisesBracketsAndNewlines(t *testing.T) {
	got := escapeAlt("a [diagram] of\nthe control loop")
	if strings.ContainsAny(got, "[]\n") {
		t.Errorf("alt text still contains breaking characters: %q", got)
	}
	if got != "a (diagram) of the control loop" {
		t.Errorf("alt text = %q", got)
	}
}

func TestCoverPrompt_NamesASubjectAndPinsTheStyle(t *testing.T) {
	got := coverPrompt("What the Gateway API Actually Changes", "kubernetes networking", "", "", "")
	if !strings.Contains(got, "kubernetes networking") {
		t.Errorf("prompt lost the topic subject: %q", got)
	}
	// Short title lettering is intentional for z-image-turbo dark covers.
	if !strings.Contains(got, `"WHAT THE GATEWAY API ACTUALLY"`) {
		t.Errorf("prompt should letter a ≤5-word title: %q", got)
	}
	if !strings.Contains(got, "charcoal navy") || !strings.Contains(got, "dark-mode") {
		t.Errorf("prompt should pin the dark cover template: %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "flat vector") {
		t.Errorf("prompt lost the flat vector style: %q", got)
	}
	if !strings.Contains(got, "16:9") {
		t.Errorf("prompt should ask for a wide cover: %q", got)
	}
	if !strings.Contains(got, "LEFT") {
		t.Errorf("prompt should place title text on the left: %q", got)
	}

	fallback := coverPrompt("", "kubernetes networking", "", "", "")
	if !strings.Contains(fallback, "kubernetes networking") {
		t.Errorf("empty title should fall back to the topic: %q", fallback)
	}
	if !strings.Contains(fallback, `"KUBERNETES NETWORKING"`) {
		t.Errorf("empty title should letter the topic as the title: %q", fallback)
	}
}

func TestCoverTitleText_CapsAtFiveWords(t *testing.T) {
	got := coverTitleText("What the Gateway API Actually Changes Today", "ignored")
	if got != "WHAT THE GATEWAY API ACTUALLY" {
		t.Errorf("title text = %q", got)
	}
}

// A cover that was drawn but has nowhere to live is not a cover: the CMS would
// receive an empty src.
func TestGenerateCover_TreatsAnUnstorableImageAsFailure(t *testing.T) {
	r := testRunner(&fakeIllustrator{enabled: true, noURL: true})

	article := &GeneratedArticle{Title: "A Title", Topic: "a topic"}
	err := r.generateCover(context.Background(), uuid.New(), article)

	if err == nil {
		t.Fatal("expected an error when the image cannot be stored")
	}
	if article.CoverImageURL != "" {
		t.Errorf("cover URL was set to %q despite the failure", article.CoverImageURL)
	}
}

func TestGenerateCover_RecordsTheSeedForReproduction(t *testing.T) {
	r := testRunner(&fakeIllustrator{enabled: true})

	article := &GeneratedArticle{Title: "A Title", Topic: "a topic"}
	if err := r.generateCover(context.Background(), uuid.New(), article); err != nil {
		t.Fatalf("generateCover: %v", err)
	}

	if article.CoverImageURL == "" || article.CoverImagePrompt == "" {
		t.Fatalf("cover not recorded: %+v", article)
	}
	if article.CoverImageSeed == 0 {
		t.Error("the seed must be stored — it is the only way to reproduce the cover")
	}
	if article.CoverImageProvider == "" || article.CoverImageModel == "" {
		t.Errorf("provider/model not recorded: %+v", article)
	}
	if len(r.images.(*fakeIllustrator).calls) != 1 {
		t.Fatalf("want one generation call")
	}
	call := r.images.(*fakeIllustrator).calls[0]
	if call.Model != "" {
		t.Errorf("cover must not pin a model (so Gemini is tried first), got %q", call.Model)
	}
	if call.Width != coverWidth || call.Height != coverHeight {
		t.Errorf("cover size = %dx%d, want %dx%d", call.Width, call.Height, coverWidth, coverHeight)
	}
	if call.Steps != coverSteps {
		t.Errorf("cover steps = %d, want %d", call.Steps, coverSteps)
	}
}

// A transient 502 must be retried without pinning a workstation model — that
// pin is what used to skip Gemini.
func TestGenerateCover_RetriesUntilItSucceeds(t *testing.T) {
	prev := coverRetryWaitFn
	coverRetryWaitFn = func(int) time.Duration { return 0 }
	t.Cleanup(func() { coverRetryWaitFn = prev })

	fake := &failNThenSucceedIllustrator{failures: 2}
	r := testRunner(fake)

	article := &GeneratedArticle{Title: "A Title", Topic: "ai agents for tax return"}
	if err := r.generateCover(context.Background(), uuid.New(), article); err != nil {
		t.Fatalf("generateCover: %v", err)
	}
	if fake.calls != 3 {
		t.Errorf("got %d calls, want 3 (2 failures + success)", fake.calls)
	}
	for i, m := range fake.models {
		if m != "" {
			t.Errorf("call %d pinned %q, want no model so Gemini is tried first", i, m)
		}
	}
}

func TestTransientImageErr(t *testing.T) {
	if !transientImageErr(fmt.Errorf("imagegen: image service returned 502: Server Error | WSL Proxy")) {
		t.Error("502 from the WSL proxy should be transient")
	}
	if transientImageErr(fmt.Errorf("imagegen: the image gateway rejected the request (status 401)")) {
		t.Error("auth rejection must not be retried")
	}
}

// failNThenSucceedIllustrator fails the first N calls with a transient error,
// then draws successfully — always on whatever model was requested.
type failNThenSucceedIllustrator struct {
	failures int
	calls    int
	models   []string
}

func (f *failNThenSucceedIllustrator) Enabled() bool { return true }

func (f *failNThenSucceedIllustrator) Generate(_ context.Context, req IllustrationRequest) (*Illustration, error) {
	f.calls++
	f.models = append(f.models, req.Model)
	if f.calls <= f.failures {
		return nil, fmt.Errorf("imagegen: image service returned 502: Server Error | WSL Proxy")
	}
	return &Illustration{
		URL: "/api/v1/images/file/ok.png", Provider: "mflux", Model: req.Model,
		Seed: 42, Width: req.Width, Height: req.Height,
	}, nil
}

// A runner with no illustrator, or a disabled one, must not try to draw.
func TestCanIllustrate(t *testing.T) {
	if (&Runner{}).canIllustrate() {
		t.Error("a runner with no illustrator must not report itself able to draw")
	}
	if testRunner(&fakeIllustrator{enabled: false}).canIllustrate() {
		t.Error("a disabled illustrator must not report itself able to draw")
	}
	if !testRunner(&fakeIllustrator{enabled: true}).canIllustrate() {
		t.Error("an enabled illustrator should be usable")
	}
}
