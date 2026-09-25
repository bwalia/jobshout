package course

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// scriptLLM answers by matching a substring of the last user message.
type scriptLLM struct {
	mu      sync.Mutex
	replies []scripted
	calls   []string
}

type scripted struct{ trigger, reply string }

func (s *scriptLLM) ProviderName() string { return "stub" }

func (s *scriptLLM) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	prompt := req.Messages[len(req.Messages)-1].Content
	s.mu.Lock()
	s.calls = append(s.calls, prompt)
	s.mu.Unlock()
	for _, r := range s.replies {
		if strings.Contains(prompt, r.trigger) {
			return &llm.GenerateResponse{Content: r.reply}, nil
		}
	}
	return nil, errors.New("stub: no scripted reply for prompt: " + prompt[:min(60, len(prompt))])
}

type stubResearch struct {
	brief *research.Brief
	err   error
	got   research.Request
}

func (s *stubResearch) Research(_ context.Context, req research.Request, _ research.ProgressFunc) (*research.Brief, error) {
	s.got = req
	return s.brief, s.err
}

type stubIllus struct {
	err   error
	count int
}

func (s *stubIllus) Illustrate(context.Context, uuid.UUID, *uuid.UUID, string, int, int) (string, error) {
	s.count++
	if s.err != nil {
		return "", s.err
	}
	return "/api/v1/images/file/org/2026/09/" + uuid.NewString() + ".png", nil
}

func usableBrief() *research.Brief {
	return &research.Brief{
		Topic:    "Kubernetes networking",
		Summary:  "How pods talk.",
		Findings: []research.Finding{{Claim: "Every pod gets its own IP.", SourceURL: "https://kubernetes.io/docs/concepts/services-networking/"}},
		Sources:  []research.Source{{URL: "https://kubernetes.io/docs/concepts/services-networking/", Title: "Services, Load Balancing, and Networking"}},
	}
}

const outlineJSON = `{"title":"Kubernetes Networking","description":"Learn how traffic flows.","level":"expert",
 "category":"DevOps","tags":["k8s","K8S","networking"],"learning_outcomes":"Explain pod IPs\nConfigure Services",
 "chapters":[{"title":"Pods and IPs","summary":"Pod addressing.","objectives":["Explain pod IPs"]},
             {"title":"Services","summary":"Stable endpoints.","objectives":["Create a Service"]}]}`

const quizJSON = `{"questions":[
 {"question":"What does each pod get?","options":["An IP","A VM","A disk","A node"],"correct_index":0,"explanation":"Each pod has its own IP."},
 {"question":"What gives a stable endpoint?","options":["Pod","Service","Node","Volume"],"correct_index":"1","explanation":"A Service is stable."},
 {"question":"Which is namespaced?","options":["Service","Node","PV","ClusterRole"],"correct_index":0,"explanation":"Services are namespaced."}]}`

func goodScript() []scripted {
	return []scripted{
		{"Plan a course.", outlineJSON},
		{"Review this course chapter", `{"issues":[]}`},
		{"Write a multiple-choice quiz", quizJSON},
		{"Write chapter", "# Should be dropped\n\n## Intro\n\nPods get IPs.<script>alert(1)</script>\n\n## Key takeaways\n\n- Pods have IPs"},
	}
}

func testBrief() model.CourseBrief {
	b, _ := NormalizeBrief(model.CourseBrief{Topic: "Kubernetes networking", ChapterCount: 2,
		SeedURLs: []string{"https://kubernetes.io/docs/"}, Focus: []string{"Services"}}, 8)
	return b
}

func TestGenerate_WritesEveryChapterWithQuizAndImages(t *testing.T) {
	lm := &scriptLLM{replies: goodScript()}
	rs := &stubResearch{brief: usableBrief()}
	il := &stubIllus{}
	g := NewGenerator(lm, rs, il, Config{Images: true}, nil)

	var outline *model.CourseOutline
	var chapters []*model.CourseChapter
	var cover string
	var steps []string
	err := g.Generate(context.Background(), Job{OrgID: uuid.New(), Brief: testBrief()}, Hooks{
		Step:    func(k, _ string) { steps = append(steps, k) },
		Planned: func(o *model.CourseOutline, _ []model.CourseSource) error { outline = o; return nil },
		Cover:   func(u string) error { cover = u; return nil },
		Chapter: func(ch *model.CourseChapter) error { chapters = append(chapters, ch); return nil },
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(rs.got.Seeds) != 1 || len(rs.got.Focus) != 1 {
		t.Errorf("seed URLs and focus must reach research, got %+v", rs.got)
	}
	if outline == nil || outline.Title != "Kubernetes Networking" {
		t.Fatalf("outline not saved: %+v", outline)
	}
	if outline.Level != "beginner" {
		t.Errorf("level must come from the brief, not the model: %q", outline.Level)
	}
	if len(outline.Tags) != 2 {
		t.Errorf("tags must be de-duplicated case-insensitively: %v", outline.Tags)
	}
	if len(outline.LearningOutcomes) != 2 {
		t.Errorf("newline-separated outcomes should split: %v", outline.LearningOutcomes)
	}
	if cover == "" {
		t.Error("cover image not saved")
	}
	if len(chapters) != 2 {
		t.Fatalf("want 2 chapters, got %d", len(chapters))
	}
	for i, ch := range chapters {
		if ch.Position != i+1 || ch.Locale != "en" {
			t.Errorf("chapter %d position/locale wrong: %d %q", i, ch.Position, ch.Locale)
		}
		if ch.Quiz == nil || len(ch.Quiz.Questions) != 3 {
			t.Errorf("chapter %d quiz missing or wrong size: %+v", i, ch.Quiz)
		}
		if len(ch.Images) != 1 || !strings.Contains(ch.HTML, "<figure>") {
			t.Errorf("chapter %d image not embedded", i)
		}
		if strings.Contains(ch.HTML, "<script") || strings.Contains(ch.HTML, "Should be dropped") {
			t.Errorf("chapter %d HTML leaked raw HTML or the H1: %s", i, ch.HTML)
		}
		for _, q := range ch.Quiz.Questions {
			if strings.Contains(ch.HTML, q.Explanation) || strings.Contains(ch.HTML, q.Question) {
				t.Errorf("chapter %d HTML must never contain quiz content", i)
			}
		}
	}
	if il.count != 3 {
		t.Errorf("want 1 cover + 2 chapter images, got %d", il.count)
	}
	if steps[len(steps)-1] != model.CourseStepSaved {
		t.Errorf("last step should be saved, got %v", steps)
	}
}

func TestGenerate_UnusableResearchFails(t *testing.T) {
	g := NewGenerator(&scriptLLM{replies: goodScript()}, &stubResearch{brief: &research.Brief{}}, nil, Config{}, nil)
	err := g.Generate(context.Background(), Job{Brief: testBrief()}, Hooks{})
	if err == nil || !strings.Contains(err.Error(), "no verified sources") {
		t.Fatalf("want unusable-research error, got %v", err)
	}
}

func TestGenerate_ImageAndQuizFailuresWarnButKeepChapter(t *testing.T) {
	script := goodScript()
	script[2] = scripted{"Write a multiple-choice quiz", `{"questions":[{"question":"only one","options":["a","b"],"correct_index":0}]}`}
	g := NewGenerator(&scriptLLM{replies: script}, &stubResearch{brief: usableBrief()}, &stubIllus{err: errors.New("gpu busy")}, Config{Images: true}, nil)

	var warnings []string
	var chapters int
	err := g.Generate(context.Background(), Job{Brief: testBrief()}, Hooks{
		Warn:    func(m string) { warnings = append(warnings, m) },
		Chapter: func(ch *model.CourseChapter) error { chapters++; return nil },
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if chapters != 2 {
		t.Errorf("chapters must still be saved, got %d", chapters)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "gpu busy") || !strings.Contains(joined, "quiz") {
		t.Errorf("want image and quiz warnings, got %q", joined)
	}
}

func TestGenerate_NotReadyWithoutResearch(t *testing.T) {
	g := NewGenerator(&scriptLLM{}, nil, nil, Config{}, nil)
	if err := g.Ready(); err == nil {
		t.Fatal("generator without research must not be ready")
	}
}

func TestGenerate_NonEnglishLocaleReachesPrompts(t *testing.T) {
	lm := &scriptLLM{replies: goodScript()}
	b := testBrief()
	b.Locale = "hi"
	g := NewGenerator(lm, &stubResearch{brief: usableBrief()}, nil, Config{}, nil)
	if err := g.Generate(context.Background(), Job{Brief: b}, Hooks{}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, c := range lm.calls {
		if !strings.Contains(c, `tag "hi"`) {
			t.Fatalf("every stage must carry the language, missing in: %.80s", c)
		}
	}
}

func TestStripFence(t *testing.T) {
	if got := stripFence("```markdown\n## A\n```"); got != "## A" {
		t.Errorf("got %q", got)
	}
	if got := stripFence("```go\nx := 1\n```"); !strings.HasPrefix(got, "```go") {
		t.Errorf("a code fence must be kept: %q", got)
	}
}
