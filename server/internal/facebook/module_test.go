package facebook

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
)

type fakeRunner struct {
	got LaunchRequest
	st  *Status
}

func (f *fakeRunner) Launch(_ context.Context, req LaunchRequest) (*LaunchResult, error) {
	f.got = req
	return &LaunchResult{PageName: "Acme", Posts: 3, Days: 7}, nil
}
func (f *fakeRunner) Status(context.Context, uuid.UUID) (*Status, error) { return f.st, nil }

func TestLaunchMapsValues(t *testing.T) {
	r := &fakeRunner{}
	m := Module(r)
	if m.Builtin != Builtin || m.Schema.Builtin != Builtin || m.TabSlug == "" {
		t.Fatalf("module = %+v", m)
	}
	agent := Seed(uuid.New())
	_, err := m.Launch(context.Background(), agentmodule.LaunchInput{
		OrgID: agent.OrgID, UserID: uuid.New(), Agent: agent,
		Values: map[string]string{"page": " Acme ", "objective": "promotion", "days": "14", "per_week": "5", "images": "no", "notes": "open day 14 Feb"},
	})
	if err != nil {
		t.Fatal(err)
	}
	g := r.got
	if g.Page != "Acme" || g.Days != 14 || g.PerWeek != 5 || g.Images || g.Notes != "open day 14 Feb" || g.AgentID != agent.ID {
		t.Errorf("request = %+v", g)
	}
	if g.Objective == "" {
		t.Error("objective not mapped")
	}
	if _, err := Module(nil).Launch(context.Background(), agentmodule.LaunchInput{Agent: agent}); err == nil {
		t.Error("nil runner launched")
	}
}

func TestDefaultsImagesOn(t *testing.T) {
	r := &fakeRunner{}
	agent := Seed(uuid.New())
	if _, err := Module(r).Launch(context.Background(), agentmodule.LaunchInput{Agent: agent, Values: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if !r.got.Images {
		t.Error("images should default on")
	}
}

func TestAbsorbPromptOnlyTakesSubstance(t *testing.T) {
	m := Module(nil)
	vals := map[string]string{}
	m.AbsorbPrompt("run it", vals)
	if vals["notes"] != "" {
		t.Errorf("short prompt absorbed: %q", vals["notes"])
	}
	m.AbsorbPrompt("plan posts about our spring open day on 14 March", vals)
	if vals["notes"] == "" {
		t.Error("substantive prompt not absorbed")
	}
}

func TestReadyReportsMissingSetup(t *testing.T) {
	cases := []struct {
		st   *Status
		code string
	}{
		{&Status{}, "facebook_not_configured"},
		{&Status{Configured: true}, "facebook_not_connected"},
		{&Status{Configured: true, Connected: true}, ""},
	}
	for _, c := range cases {
		issues := Module(&fakeRunner{st: c.st}).Ready(context.Background(), uuid.New())
		switch {
		case c.code == "" && len(issues) != 0, c.code != "" && (len(issues) != 1 || issues[0].Code != c.code):
			t.Errorf("status %+v: issues = %+v", c.st, issues)
		}
	}
}

func TestPageCreationIsNeverClaimed(t *testing.T) {
	if CreationGuide.Supported || len(CreationGuide.Steps) == 0 {
		t.Errorf("guide = %+v", CreationGuide)
	}
}
