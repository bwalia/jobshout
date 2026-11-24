package blog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/imagestore"
	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
)

// websiteCMS is a fakeCMS that also lists categories and takes uploads, as
// the opsapi client does.
type websiteCMS struct {
	fakeCMS
	categories []opsapi.Category
	uploads    []string // file names, in order
	noStorage  bool
}

func (w *websiteCMS) ListCategories(context.Context) ([]opsapi.Category, error) {
	return w.categories, nil
}

func (w *websiteCMS) UploadMedia(_ context.Context, filename, _ string, data []byte, _, _ string) (*opsapi.Media, error) {
	if w.noStorage {
		return nil, fmt.Errorf("%w in namespace %q", opsapi.ErrMediaNotConfigured, "acme")
	}
	if len(data) == 0 {
		return nil, errors.New("empty upload")
	}
	w.uploads = append(w.uploads, filename)
	return &opsapi.Media{URL: fmt.Sprintf("https://site.test/media/%d-%s", len(w.uploads), filename)}, nil
}

// The bug this guards: the writer's style was saved on the agent and recorded
// on the run, then dropped when Generate copied the briefs, so no house style
// ever reached a prompt. Content rules ride the same way and are also checked
// by the review and handed to the revision.
func TestWriterStyleAndRulesReachThePrompts(t *testing.T) {
	stub := &stubLLM{responses: []scriptedResponse{
		{trigger: promptPlan, content: `{"title":"T","angle":"a","sections":["One"]}`},
		{trigger: promptDraft, content: "# T\n\nBody [1]."},
		{trigger: promptReview, content: `{"issues":["Paragraph 2 promises a return."]}`},
		{trigger: promptRevise, content: "# T\n\nRevised body [1]."},
	}}
	r := newRunnerWithStub(stub)

	_, err := r.Generate(context.Background(), GenerateRequest{
		Briefs: []model.BlogBrief{{
			Topic: "Buy-to-let in 2027",
			Style: "Plain British English, no hype.",
			Rules: "Never promise or imply a return.",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	for _, phase := range []string{promptPlan, promptDraft} {
		p := promptFor(t, stub, phase)
		for _, want := range []string{"Plain British English, no hype.", "CONTENT RULES", "Never promise or imply a return."} {
			if !strings.Contains(p, want) {
				t.Errorf("%s prompt is missing %q", phase, want)
			}
		}
	}
	review := promptFor(t, stub, promptReview)
	if !strings.Contains(review, "\n- BREAKS OF THE WRITER'S CONTENT RULES") {
		t.Errorf("the rules check is not on a line of its own:\n%s", review)
	}
	if !strings.Contains(review, "BREAKS OF THE WRITER'S CONTENT RULES") || !strings.Contains(review, "Never promise or imply a return.") {
		t.Errorf("the review does not check the content rules:\n%s", review)
	}
	revise := promptFor(t, stub, promptRevise)
	if !strings.Contains(revise, "Never promise or imply a return.") {
		t.Errorf("the revision is not given the content rules:\n%s", revise)
	}
}

// A writer without rules gets no rules block and no rules check.
func TestNoRulesNoRulesCheck(t *testing.T) {
	if got := rulesBlock(model.BlogBrief{}); got != "" {
		t.Errorf("rulesBlock with no rules = %q", got)
	}
	if got := rulesCheck(model.BlogBrief{Rules: "  "}); got != "" {
		t.Errorf("rulesCheck with blank rules = %q", got)
	}
}

func TestPublishTo_FilesWithTheWritersPostSettings(t *testing.T) {
	cms := &websiteCMS{categories: []opsapi.Category{
		{UUID: "c-btl", Name: "Buy-to-let", Slug: "buy-to-let"},
		{UUID: "c-mkt", Name: "Market insights", Slug: "market-insights"},
	}}
	r := newTestRunner(nil)
	cover, _ := r.covers.PutFile(context.Background(), uuid.Nil, []byte("png"), "png", "image/png")

	_, err := r.PublishTo(context.Background(), uuid.Nil, cms, []GeneratedArticle{{
		Topic: "t", Slug: "t", Title: "T", Excerpt: "e", HTML: "<p>Body</p>",
		Audience: "business", CoverImageURL: cover,
	}}, model.PostSettings{
		Author:     "Portfolio Lab",
		Categories: []string{"market-insights", "Buy-to-let", "MARKET INSIGHTS"},
		Tags:       []string{"UK property"},
	}, nil)
	if err != nil {
		t.Fatalf("PublishTo: %v", err)
	}
	got := cms.posts[0]
	if got.AuthorName != "Portfolio Lab" {
		t.Errorf("byline = %q, want the writer's", got.AuthorName)
	}
	if strings.Join(got.CategoryUUIDs, ",") != "c-mkt,c-btl" {
		t.Errorf("categories = %v, want [c-mkt c-btl] (order kept, duplicates dropped)", got.CategoryUUIDs)
	}
	if strings.Join(got.Tags, ",") != "UK property" {
		t.Errorf("tags = %v, want only the writer's", got.Tags)
	}
	if !strings.HasPrefix(got.FeaturedImageURL, "https://jobshout.test/") {
		t.Errorf("without cms images the cover stays on this ring, got %q", got.FeaturedImageURL)
	}
	if len(cms.uploads) != 0 {
		t.Errorf("uploaded %v without cms images on", cms.uploads)
	}
}

// No settings is the behaviour from before they existed.
func TestPublishTo_ZeroSettingsFileAsBefore(t *testing.T) {
	cms := &websiteCMS{}
	r := newTestRunner(nil)
	cover, _ := r.covers.PutFile(context.Background(), uuid.Nil, []byte("png"), "png", "image/png")
	if _, err := r.PublishTo(context.Background(), uuid.Nil, cms, []GeneratedArticle{{
		Topic: "t", Slug: "t", Title: "T", HTML: "<p>x</p>", Audience: "business", CoverImageURL: cover,
	}}, model.PostSettings{}, nil); err != nil {
		t.Fatalf("PublishTo: %v", err)
	}
	got := cms.posts[0]
	if got.AuthorName != "Test Writer" || len(got.CategoryUUIDs) != 0 {
		t.Errorf("zero settings changed the post: author %q, categories %v", got.AuthorName, got.CategoryUUIDs)
	}
	if want := articleTags(GeneratedArticle{Audience: "business"}); strings.Join(got.Tags, ",") != strings.Join(want, ",") {
		t.Errorf("tags = %v, want the default %v", got.Tags, want)
	}
}

// A category the website does not have stops the publish before any draft is
// filed, and says which categories exist.
func TestPublishTo_UnknownCategoryFilesNothing(t *testing.T) {
	cms := &websiteCMS{categories: []opsapi.Category{{UUID: "c1", Name: "Buy-to-let", Slug: "buy-to-let"}}}
	r := newTestRunner(nil)
	_, err := r.PublishTo(context.Background(), uuid.Nil, cms, []GeneratedArticle{{
		Topic: "t", Slug: "t", Title: "T", HTML: "<p>x</p>",
	}}, model.PostSettings{Categories: []string{"Crypto"}}, nil)
	if err == nil || !strings.Contains(err.Error(), `"Crypto"`) || !strings.Contains(err.Error(), "Buy-to-let") {
		t.Fatalf("err = %v, want one naming the missing category and the existing ones", err)
	}
	if len(cms.posts) != 0 {
		t.Fatalf("filed %d draft(s) despite the bad category", len(cms.posts))
	}
}

func TestPublishTo_CopiesImagesToTheWebsite(t *testing.T) {
	cms := &websiteCMS{}
	r := newTestRunner(nil)
	ctx := context.Background()
	cover, _ := r.covers.PutFile(ctx, uuid.Nil, []byte("cover"), "png", "image/png")
	fig, _ := r.covers.PutFile(ctx, uuid.Nil, []byte("figure"), "png", "image/png")
	html := `<p>One</p><img alt="a" src="https://jobshout.test` + fig + `" /><img src="https://elsewhere.test/x.png">` +
		`<img src="https://jobshout.test` + fig + `">`

	_, err := r.PublishTo(ctx, uuid.Nil, cms, []GeneratedArticle{{
		Topic: "t", Slug: "t", Title: "Gulf investors", HTML: html, CoverImageURL: cover,
	}}, model.PostSettings{CMSImages: true}, nil)
	if err != nil {
		t.Fatalf("PublishTo: %v", err)
	}
	got := cms.posts[0]
	if !strings.HasPrefix(got.FeaturedImageURL, "https://site.test/media/") {
		t.Errorf("featured image = %q, want the website's copy", got.FeaturedImageURL)
	}
	if strings.Contains(got.ContentHTML, "jobshout.test") {
		t.Errorf("body still loads images from this ring:\n%s", got.ContentHTML)
	}
	if !strings.Contains(got.ContentHTML, "https://elsewhere.test/x.png") {
		t.Errorf("an image that is not ours was changed:\n%s", got.ContentHTML)
	}
	// The cover and the one figure, once each — the repeated figure reuses
	// its first copy.
	if len(cms.uploads) != 2 || !strings.HasPrefix(cms.uploads[0], "gulf-investors-cover") {
		t.Errorf("uploads = %v", cms.uploads)
	}
}

// A website without image storage fails the publish rather than quietly
// leaving the images on this ring.
func TestPublishTo_ImagesWithoutStorageFail(t *testing.T) {
	cms := &websiteCMS{noStorage: true}
	r := newTestRunner(nil)
	cover, _ := r.covers.PutFile(context.Background(), uuid.Nil, []byte("cover"), "png", "image/png")
	_, err := r.PublishTo(context.Background(), uuid.Nil, cms, []GeneratedArticle{{
		Topic: "t", Slug: "t", Title: "T", HTML: "<p>x</p>", CoverImageURL: cover,
	}}, model.PostSettings{CMSImages: true}, nil)
	if !errors.Is(err, opsapi.ErrMediaNotConfigured) {
		t.Fatalf("err = %v, want ErrMediaNotConfigured", err)
	}
	if len(cms.posts) != 0 {
		t.Fatal("filed a draft whose images could not be copied")
	}
}

func TestCopyable(t *testing.T) {
	r := newTestRunner(nil)
	for src, want := range map[string]bool{
		"https://jobshout.test" + imagestore.URLPrefix + "org/1.png": true,
		imagestore.URLPrefix + "org/1.png":                           true,
		"https://mermaid.ink/svg/abc":                                true,
		"http://mermaid.ink/svg/abc":                                 false,
		"https://evil.test/api/v1/images/file/x.png":                 false,
		"https://169.254.169.254/latest":                             false,
	} {
		if got := r.copyable(src); got != want {
			t.Errorf("copyable(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestUploadableURL_MermaidUploadsAsPNG(t *testing.T) {
	if got, want := uploadableURL("https://mermaid.ink/svg/abc"), "https://mermaid.ink/img/abc?type=png&width=1200"; got != want {
		t.Errorf("mermaid svg: got %q, want %q", got, want)
	}
	if src := "https://jobshout.test/api/v1/images/file/x.png"; uploadableURL(src) != src {
		t.Errorf("stored image changed: %q", uploadableURL(src))
	}
}
