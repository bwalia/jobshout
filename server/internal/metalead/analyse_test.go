package metalead

import (
	"context"
	"testing"

	"github.com/jobshout/server/internal/model"
)

func TestDecodeBriefTolerant(t *testing.T) {
	raw := "Here you go:\n```json\n{\"intent\":\"demo\",\"fit_score\":120,\"next_action\":\"Call\",\"spam_risk\":\"\",\"flags\":null,\"summary\":\"Good\"}\n```"
	b, err := decodeBrief(raw)
	if err != nil {
		t.Fatal(err)
	}
	if b.FitScore != 100 || b.SpamRisk != "low" || b.Intent != "demo" || b.Flags == nil {
		t.Fatalf("brief = %+v", b)
	}
}

func TestDecodeRollupDefaultsCount(t *testing.T) {
	r, err := decodeRollup(`{"themes":["saas"],"quality_mix":"mixed","recommended_followups":["email"]}`, 3)
	if err != nil {
		t.Fatal(err)
	}
	if r.LeadCount != 3 || len(r.Themes) != 1 {
		t.Fatalf("rollup = %+v", r)
	}
}

func TestAnalyserLeadAndRollup(t *testing.T) {
	a := &Analyser{LLM: &countingLLM{
		responses: []string{
			`{"intent":"buy","fit_score":80,"next_action":"Book","spam_risk":"low","flags":[],"summary":"OK"}`,
			`{"themes":["buy"],"quality_mix":"high","recommended_followups":["Book demo"],"lead_count":1}`,
		},
	}, Model: "test"}
	brief, err := a.AnalyseLead(context.Background(), []FieldData{{Name: "email", Values: []string{"x@y.com"}}}, "UK SaaS")
	if err != nil {
		t.Fatal(err)
	}
	if brief.FitScore != 80 {
		t.Fatalf("brief = %+v", brief)
	}
	rollup, err := a.AnalyseRollup(context.Background(), []model.MetaLeadBrief{*brief}, "UK SaaS")
	if err != nil {
		t.Fatal(err)
	}
	if rollup.QualityMix != "high" {
		t.Fatalf("rollup = %+v", rollup)
	}
}

type countingLLM struct {
	responses []string
	i         int
}

func (c *countingLLM) Complete(_ context.Context, _, _ string, _ int, _ bool) (string, error) {
	if c.i >= len(c.responses) {
		return "{}", nil
	}
	out := c.responses[c.i]
	c.i++
	return out, nil
}
