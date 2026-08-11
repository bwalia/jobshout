package blog

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
)

// newTestRunner builds a Runner with the given GitHub token. An empty token is
// the interesting case: generation must still work, publishing must not.
func newTestRunner(token string, responses ...string) *Runner {
	return NewRunner(Config{
		GitHubToken: token,
		RepoOwner:   "acme",
		RepoName:    "site",
		BaseBranch:  "main",
		ContentDir:  "content/blogs",
	}, &stubLLM{responses: responses}, zap.NewNop())
}

func TestCanPublish(t *testing.T) {
	if newTestRunner("").CanPublish() {
		t.Error("CanPublish() = true with no token, want false")
	}
	if !newTestRunner("ghp_token").CanPublish() {
		t.Error("CanPublish() = false with a token, want true")
	}
}

// Generation must not require GitHub credentials — the whole point of the
// split is that an article can be written and read without a repository.
func TestGenerate_WithoutGitHubToken(t *testing.T) {
	r := newTestRunner("", "# Kubernetes\n\nBody.")

	arts, err := r.Generate(context.Background(), GenerateRequest{
		Topics: []string{"Kubernetes debugging"},
	}, nil)
	if err != nil {
		t.Fatalf("Generate without token: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("want 1 article, got %d", len(arts))
	}
	if !strings.Contains(arts[0].Markdown, "# Kubernetes") {
		t.Errorf("markdown not returned: %q", arts[0].Markdown)
	}
}

func TestPublish_WithoutTokenIsRefused(t *testing.T) {
	r := newTestRunner("")
	_, err := r.Publish(context.Background(), []GeneratedArticle{{Topic: "x", Markdown: "# x"}}, nil)
	if err == nil {
		t.Fatal("expected Publish to be refused without a token")
	}
	if !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("error should name the missing config, got: %v", err)
	}
}

func TestPublish_NothingToPublish(t *testing.T) {
	r := newTestRunner("ghp_token")
	if _, err := r.Publish(context.Background(), nil, nil); err == nil {
		t.Fatal("expected an error when publishing zero articles")
	}
}

func TestGenerate_NoTopics(t *testing.T) {
	r := newTestRunner("")
	if _, err := r.Generate(context.Background(), GenerateRequest{Topics: []string{" ", ""}}, nil); err == nil {
		t.Fatal("expected an error when every topic is blank")
	}
}

// The batch is capped regardless of what the caller asks for, so a typo cannot
// produce a 25-article pull request.
func TestGenerate_CapsTopics(t *testing.T) {
	responses := make([]string, HardMaxArticles)
	topics := make([]string, HardMaxArticles+5)
	for i := range responses {
		responses[i] = "# t\n\nbody"
	}
	for i := range topics {
		topics[i] = "topic " + string(rune('a'+i))
	}

	r := newTestRunner("", responses...)
	arts, err := r.Generate(context.Background(), GenerateRequest{Topics: topics}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(arts) != HardMaxArticles {
		t.Errorf("got %d articles, want the hard cap of %d", len(arts), HardMaxArticles)
	}
}

// Generation reports its final step so the UI can show "ready" rather than
// leaving the last per-topic label running forever.
func TestGenerate_ReportsGeneratedStep(t *testing.T) {
	var keys []string
	r := newTestRunner("", "# a\n\nbody")
	if _, err := r.Generate(context.Background(), GenerateRequest{Topics: []string{"a"}},
		func(key, _ string) { keys = append(keys, key) }); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(keys) < 2 {
		t.Fatalf("want at least a generating and a generated step, got %v", keys)
	}
	if keys[0] != model.BlogStepGenerating {
		t.Errorf("first step = %q, want %q", keys[0], model.BlogStepGenerating)
	}
	if keys[len(keys)-1] != model.BlogStepGenerated {
		t.Errorf("last step = %q, want %q", keys[len(keys)-1], model.BlogStepGenerated)
	}
}
