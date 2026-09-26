package course

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/model"
)

func TestNormalizeBrief(t *testing.T) {
	b, err := NormalizeBrief(model.CourseBrief{Topic: "  Go testing "}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if b.Topic != "Go testing" || b.Level != "beginner" || b.ChapterCount != 5 || b.Locale != "en" {
		t.Errorf("defaults not applied: %+v", b)
	}

	bad := []model.CourseBrief{
		{},
		{Topic: "x", Level: "expert"},
		{Topic: "x", ChapterCount: 9},
		{Topic: "x", Locale: "english; drop table"},
		{Topic: "x", SeedURLs: []string{"javascript:alert(1)"}},
		{Topic: "x", SeedURLs: []string{"file:///etc/passwd"}},
		{Topic: strings.Repeat("a", maxTopicLen+1)},
	}
	for _, in := range bad {
		if _, err := NormalizeBrief(in, 8); err == nil {
			t.Errorf("want error for %+v", in)
		}
	}
	if b, err := NormalizeBrief(model.CourseBrief{Topic: "x", Locale: "pt-BR", Level: "Advanced"}, 8); err != nil || b.Level != "advanced" {
		t.Errorf("pt-BR / Advanced should be accepted: %v %+v", err, b)
	}
}

func TestBriefFromValues(t *testing.T) {
	b := BriefFromValues(map[string]string{
		"topic": "SQL", "chapters": "3", "seed_urls": "https://a.example/x\n\nhttps://b.example/y",
		"focus": "joins, indexes ,",
	})
	if b.ChapterCount != 3 || len(b.SeedURLs) != 2 || len(b.Focus) != 2 {
		t.Errorf("parsed wrong: %+v", b)
	}
}

func TestValidateQuiz(t *testing.T) {
	var w quizWire
	raw := `{"questions":[
	 {"question":"Q1","options":["a","b","c","d"],"correct_index":2,"explanation":"e"},
	 {"question":"Q1","options":["a","b","c","d"],"correct_index":2,"explanation":"duplicate question"},
	 {"question":"Q2","options":["a","a","c","d"],"correct_index":0,"explanation":"duplicate option"},
	 {"question":"Q3","options":["a","b","c","d"],"correct_index":4,"explanation":"out of range"},
	 {"question":"Q4","options":["a","b","c","d"],"correct_index":"x","explanation":"not a number"},
	 {"question":"Q5","options":["a","b","c","d"],"correct_index":1,"explanation":""},
	 {"question":"Q6","options":["a","b","c","d"],"correct_index":"3","explanation":"string index ok"},
	 {"question":"Q7","options":["a","b","c","d"],"correct_index":0,"explanation":"ok"}]}`
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatal(err)
	}
	q, err := validateQuiz(w)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Questions) != 3 {
		t.Fatalf("want Q1, Q6, Q7; got %+v", q.Questions)
	}
	if q.Questions[1].CorrectIndex != 3 {
		t.Errorf("string index should decode: %+v", q.Questions[1])
	}

	var few quizWire
	_ = json.Unmarshal([]byte(`{"questions":[{"question":"Q","options":["a","b","c","d"],"correct_index":0,"explanation":"e"}]}`), &few)
	if _, err := validateQuiz(few); err == nil {
		t.Error("one question is not a quiz")
	}
}

func TestRenderLesson_IsSafe(t *testing.T) {
	md := "# Title\n\n## Part\n\nText <img src=x onerror=alert(1)> and [bad](javascript:alert(1)).\n\n<iframe src=\"https://evil.example\"></iframe>"
	html, err := renderLesson(md, []model.CourseImage{
		{URL: "/api/v1/images/file/a.png", Alt: `"><script>x</script>`, Caption: "cap"},
		{URL: "javascript:alert(1)", Alt: "bad"},
		{URL: "http://insecure.example/a.png", Alt: "http"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"<h1", "onerror", "javascript:", "<iframe", "<script", "insecure.example"} {
		if strings.Contains(html, banned) {
			t.Errorf("lesson HTML contains %q:\n%s", banned, html)
		}
	}
	if !strings.Contains(html, "<h2>Part</h2>") || strings.Count(html, "<figure>") != 1 {
		t.Errorf("expected heading and exactly one safe figure:\n%s", html)
	}
}

func TestLooseStrings(t *testing.T) {
	var s struct {
		A looseStrings `json:"a"`
		B looseStrings `json:"b"`
		C looseString  `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"a":"- one\n- two","b":[{"text":"x"},"y",""],"c":["p","q"]}`), &s); err != nil {
		t.Fatal(err)
	}
	if len(s.A) != 2 || s.A[0] != "one" || len(s.B) != 2 || s.C != "p, q" {
		t.Errorf("got %+v", s)
	}
}

type fakeRunner struct{ got model.CreateCourseRunRequest }

func (f *fakeRunner) CreateRun(_ context.Context, req model.CreateCourseRunRequest, _ uuid.UUID, _ *uuid.UUID) (*model.CourseRun, error) {
	f.got = req
	return &model.CourseRun{ID: uuid.New()}, nil
}

func TestModuleLaunch(t *testing.T) {
	if _, err := Module(nil).Launch(context.Background(), agentmodule.LaunchInput{}); err == nil {
		t.Error("nil runner must refuse")
	}
	r := &fakeRunner{}
	m := Module(r)
	out, err := m.Launch(context.Background(), agentmodule.LaunchInput{
		Agent:  &model.Agent{ID: uuid.New()},
		Task:   &model.Task{ID: uuid.New()},
		Values: map[string]string{"topic": "Rust", "chapters": "4", "locale": "pa"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.RunID == nil || out.ExtraMeta[model.TaskMetaRunID] == nil {
		t.Error("launch must return the run id for the board task")
	}
	if r.got.Brief.Topic != "Rust" || r.got.Brief.ChapterCount != 4 || r.got.Brief.Locale != "pa" {
		t.Errorf("brief not passed through: %+v", r.got.Brief)
	}
	vals := map[string]string{}
	m.AbsorbPrompt("Make a course on Terraform", vals)
	if vals["topic"] == "" {
		t.Error("chat prompt should become the topic")
	}
	if len(m.Schema.Fields) == 0 || m.Schema.Builtin != model.BuiltinCourseGenerator {
		t.Error("schema must be registered for the builtin")
	}
}

func TestRunBudgetScalesWithChapters(t *testing.T) {
	c := Config{PlanBudget: 10, ChapterBudget: 5}
	if c.RunBudget(4) != 30 || c.RunBudget(0) != 15 {
		t.Errorf("budget wrong: %v %v", c.RunBudget(4), c.RunBudget(0))
	}
}
