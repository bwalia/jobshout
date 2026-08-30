package blog

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
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
		model = "z-image-turbo"
	}
	return &Illustration{
		URL: url, Provider: "mflux", Model: model,
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
	got := coverPrompt("What the Gateway API Actually Changes", "kubernetes networking")
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
	// Placement is a varying axis now, so the invariant is that the prompt
	// commits to one of the curated placements — not that it picks a
	// particular one, which would only hold for topics that hash that way.
	if !strings.Contains(got, "LEFT") && !strings.Contains(got, "RIGHT") && !strings.Contains(got, "LOWER THIRD") {
		t.Errorf("prompt should place the title explicitly: %q", got)
	}

	fallback := coverPrompt("", "kubernetes networking")
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
	err := r.generateCover(context.Background(), GenerateRequest{OrgID: uuid.New()}, article)

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
	if err := r.generateCover(context.Background(), GenerateRequest{OrgID: uuid.New()}, article); err != nil {
		t.Fatalf("generateCover: %v", err)
	}

	if article.CoverImageURL == "" || article.CoverImagePrompt == "" {
		t.Fatalf("cover not recorded: %+v", article)
	}
	if article.CoverImageSeed == 0 {
		t.Error("the seed must be stored — it is the only way to reproduce the cover")
	}
	if article.CoverImageProvider != "mflux" || article.CoverImageModel != coverModel {
		t.Errorf("provider/model not recorded: %+v", article)
	}
	if len(r.images.(*fakeIllustrator).calls) != 1 {
		t.Fatalf("want one generation call")
	}
	call := r.images.(*fakeIllustrator).calls[0]
	if call.Model != coverModel {
		t.Errorf("cover must pin %q, got %q", coverModel, call.Model)
	}
	if call.Width != coverWidth || call.Height != coverHeight {
		t.Errorf("cover size = %dx%d, want %dx%d", call.Width, call.Height, coverWidth, coverHeight)
	}
	if call.Steps != coverSteps {
		t.Errorf("cover steps = %d, want %d", call.Steps, coverSteps)
	}
}

// A transient 502 must be retried against qwen — never silently swapped for a
// faster model.
func TestGenerateCover_RetriesQwenUntilItSucceeds(t *testing.T) {
	prev := coverRetryWaitFn
	coverRetryWaitFn = func(int) time.Duration { return 0 }
	t.Cleanup(func() { coverRetryWaitFn = prev })

	fake := &failNThenSucceedIllustrator{failures: 2}
	r := testRunner(fake)

	article := &GeneratedArticle{Title: "A Title", Topic: "ai agents for tax return"}
	if err := r.generateCover(context.Background(), GenerateRequest{OrgID: uuid.New()}, article); err != nil {
		t.Fatalf("generateCover: %v", err)
	}
	if article.CoverImageModel != coverModel {
		t.Errorf("cover model = %q, want %q", article.CoverImageModel, coverModel)
	}
	if fake.calls != 3 {
		t.Errorf("got %d calls, want 3 (2 failures + success)", fake.calls)
	}
	for i, m := range fake.models {
		if m != coverModel {
			t.Errorf("call %d used %q, want %q every time", i, m, coverModel)
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

// The article the writer handed over has no pictures in it. A runner that can
// draw asks once where they should go, and puts the fences in itself.
func TestEnsureIllustrations_AddsFencesToAPictureLessArticle(t *testing.T) {
	llmStub := &stubLLM{responses: []scriptedResponse{{
		trigger: promptIllustrate,
		content: `{"illustrations":[
			{"after_heading":"How spindles fail","scene":"A machinist watching a spindle spin down in a quiet workshop"},
			{"after_heading":"What to measure","scene":"A vibration sensor clamped to a spindle housing, cables running away"}
		]}`,
	}}}
	r := testRunnerWithLLM(llmStub, &fakeIllustrator{enabled: true})

	markdown := "# Spindles\n\nIntro.\n\n## How spindles fail\n\nBearings.\n\n## What to measure\n\nVibration.\n"
	out, notes := r.ensureIllustrations(context.Background(), "m", &writePlan{Title: "Spindles"}, markdown)

	if got := len(illustrationFence.FindAllString(out, -1)); got != 2 {
		t.Fatalf("want 2 illustration fences added, got %d\n%s", got, out)
	}
	if !strings.Contains(out, "## How spindles fail\n\n```illustration\nA machinist") {
		t.Errorf("the fence should open the section it was placed under, got:\n%s", out)
	}
	if !strings.Contains(strings.Join(notes, " "), "added 2 illustration request(s)") {
		t.Errorf("the run should say what it added, got %v", notes)
	}
	// Nothing but the fences: an article that came back with its prose rewritten
	// would have had its citations and diagrams put at risk to gain a picture.
	if !strings.Contains(out, "Bearings.") || !strings.Contains(out, "Vibration.") {
		t.Errorf("the prose must survive untouched, got:\n%s", out)
	}
}

// A writer that already asked for a picture is left alone — the second opinion
// is only for an article that has none.
func TestEnsureIllustrations_LeavesAnIllustratedArticleAlone(t *testing.T) {
	llmStub := &stubLLM{}
	r := testRunnerWithLLM(llmStub, &fakeIllustrator{enabled: true})

	markdown := "# T\n\n## S\n\n```illustration\nA lighthouse at dawn\n```\n\nBody.\n"
	out, notes := r.ensureIllustrations(context.Background(), "m", &writePlan{Title: "T"}, markdown)

	if out != markdown || notes != nil {
		t.Errorf("an illustrated article should come back unchanged, got %q %v", out, notes)
	}
	if len(llmStub.calls) != 0 {
		t.Errorf("no model call should be made, got %d", len(llmStub.calls))
	}
}

// A runner with no image generator must not ask, because it could not draw the
// answer — the same reason the prompt does not offer illustrations either.
func TestEnsureIllustrations_SilentWithoutAnIllustrator(t *testing.T) {
	llmStub := &stubLLM{}
	r := testRunnerWithLLM(llmStub, &fakeIllustrator{enabled: false})

	markdown := "# T\n\n## S\n\nBody.\n"
	out, _ := r.ensureIllustrations(context.Background(), "m", &writePlan{Title: "T"}, markdown)

	if out != markdown || len(llmStub.calls) != 0 {
		t.Errorf("a runner that cannot draw must not ask where to draw")
	}
	if r.illustrationRequirement() != "" {
		t.Error("a runner that cannot draw must not require an illustration either")
	}
}

// The prompt's rules are advice; these are the enforcement. A model that names
// a heading the article does not have, repeats itself, or asks for more
// pictures than the budget allows gets those placements dropped rather than
// obeyed.
func TestInsertIllustrations_EnforcesTheRulesThePromptStates(t *testing.T) {
	markdown := "# T\n\n## One\n\na\n\n## Two\n\nb\n\n## Three\n\nc\n\n## Four\n\nd\n"
	out, notes := insertIllustrations(markdown, []illustrationPlacement{
		{AfterHeading: "One", Scene: "a workshop at dawn"},
		{AfterHeading: "One", Scene: "a second picture in the same section"},
		{AfterHeading: "Two", Scene: "A WORKSHOP AT DAWN"},
		{AfterHeading: "Nowhere", Scene: "a heading that does not exist"},
		{AfterHeading: "Two", Scene: ""},
		{AfterHeading: "Two", Scene: "a lathe"},
		{AfterHeading: "Three", Scene: "a milling machine"},
		{AfterHeading: "Four", Scene: "one picture too many"},
	})

	if got := len(illustrationFence.FindAllString(out, -1)); got != maxInlineIllustrations {
		t.Fatalf("want %d fences, got %d\n%s", maxInlineIllustrations, got, out)
	}
	joined := strings.Join(notes, "\n")
	for _, want := range []string{
		"dropped a second illustration under \"One\"",
		"repeats an earlier scene",
		"not a heading in the article",
		"no description",
		"beyond the limit of 3",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("want a note saying %q, got:\n%s", want, joined)
		}
	}
}

// The H1 is already covered by the cover image and the reference list is not a
// place for a picture, so neither is offered to the model.
func TestBodyHeadings_SkipsTheTitleAndTheReferences(t *testing.T) {
	got := bodyHeadings("# Title\n\n## Real\n\n### Also real\n\n## References\n\n1. x\n")
	want := []string{"Real", "Also real"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want %v, got %v", want, got)
		}
	}
}

// testRunnerWithLLM is testRunner for the phases that talk to a model.
func testRunnerWithLLM(client llm.Client, images Illustrator) *Runner {
	r := &Runner{llm: client, logger: zap.NewNop()}
	return r.WithIllustrator(images)
}
