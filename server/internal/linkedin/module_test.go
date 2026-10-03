package linkedin

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/model"
)

type fakeRunner struct{ got LaunchRequest }

func (f *fakeRunner) Launch(_ context.Context, req LaunchRequest) (*LaunchResult, error) {
	f.got = req
	return &LaunchResult{ArticleID: uuid.New(), ArticleTitle: "T", Variants: req.Variants}, nil
}

func TestVariantsFromValue(t *testing.T) {
	cases := map[string][]string{
		"":          model.LinkedInVariants,
		"both":      model.LinkedInVariants,
		"technical": {model.LinkedInVariantTechnical},
		"Business":  {model.LinkedInVariantBusiness},
	}
	for in, want := range cases {
		if got := VariantsFromValue(in); !reflect.DeepEqual(got, want) {
			t.Errorf("VariantsFromValue(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLaunchMapsValues(t *testing.T) {
	r := &fakeRunner{}
	m := Module(r)
	if m.Builtin != model.BuiltinLinkedIn || m.Schema.Builtin != model.BuiltinLinkedIn || m.TabSlug == "" {
		t.Fatalf("module = %+v", m)
	}
	agent := Seed(uuid.New())
	task := &model.Task{ID: uuid.New()}
	out, err := m.Launch(context.Background(), agentmodule.LaunchInput{
		OrgID: agent.OrgID, UserID: uuid.New(), Agent: agent, Task: task,
		Values: map[string]string{"article": " flaky deploys ", "variants": "technical", "notes": "on-call"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.got.Article != "flaky deploys" || r.got.Notes != "on-call" || r.got.AgentID != agent.ID ||
		r.got.TaskID == nil || *r.got.TaskID != task.ID || !reflect.DeepEqual(r.got.Variants, []string{"technical"}) {
		t.Errorf("request = %+v", r.got)
	}
	if out.Message == "" || out.Status != "" {
		t.Errorf("output = %+v", out)
	}
	if _, err := Module(nil).Launch(context.Background(), agentmodule.LaunchInput{Agent: agent}); err == nil {
		t.Error("nil runner launched")
	}
}

func TestAbsorbPromptTakesOnlyAnArticleID(t *testing.T) {
	m := Module(nil)
	id := uuid.New().String()
	vals := map[string]string{}
	m.AbsorbPrompt("post article "+id+" to linkedin", vals)
	if vals["article"] != id {
		t.Errorf("article = %q", vals["article"])
	}
	vals = map[string]string{}
	m.AbsorbPrompt("post my latest article to linkedin", vals)
	if vals["article"] != "" {
		t.Errorf("free text absorbed as an article: %q", vals["article"])
	}
}
