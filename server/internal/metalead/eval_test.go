package metalead

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func TestScoreEval(t *testing.T) {
	yes, no := true, false
	lead := func(label bool) model.MetaLead {
		return model.MetaLead{ID: uuid.New(), Body: "msg", LabelIsLead: &label}
	}
	cases := []model.MetaLead{lead(true), lead(true), lead(false), lead(false)}
	votes := [][]evalVote{
		{{IsLead: &yes, Grounded: true, MS: 100}, {IsLead: &yes, Grounded: true, MS: 100}, {IsLead: &yes, MS: 100}}, // right, consistent
		{{IsLead: &no}, {IsLead: &no}, {IsLead: &yes}},                                                              // missed lead
		{{IsLead: &no}, {IsLead: &no}, {IsLead: &no}},                                                               // right, consistent
		{{IsLead: &yes}, {}, {IsLead: &no}},                                                                         // tie after a failure
	}
	r := scoreEval("claude:x", cases, votes)
	if r.Accuracy != 0.5 || r.Consistency != 0.5 || r.Precision != 1 || r.Recall != 0.5 ||
		r.Grounded != 0.4 || r.Failures != 1 || r.AvgLatencyMS != 25 {
		t.Fatalf("row = %+v", r)
	}
	if len(r.Misses) != 2 || r.Misses[0].Votes != "lead 1/3" || !r.Misses[0].Label || r.Misses[1].Label {
		t.Fatalf("misses = %+v", r.Misses)
	}
}

func TestStartEval(t *testing.T) {
	store := newMemStore()
	org := uuid.New()
	svc := NewService(store, fakeGraph{}, nil, Config{}, nil)
	svc.BindModels(func(provider, name string) (Completer, error) {
		if provider != "good" {
			return nil, errors.New("llm: unknown provider " + provider)
		}
		return funcLLM(func(prompt string) (string, error) {
			if strings.Contains(prompt, "<<<\nhow much") {
				return `{"is_lead":true,"evidence":"how much"}`, nil
			}
			return `{"is_lead":false}`, nil
		}), nil
	})
	if _, err := svc.StartEval(context.Background(), org, uuid.New(), []string{"good:m"}, 1); err == nil {
		t.Fatal("nothing labelled yet")
	}
	yes, no := true, false
	store.leads["a"] = &model.MetaLead{ID: uuid.New(), Body: "how much for 2?", LabelIsLead: &yes}
	store.leads["b"] = &model.MetaLead{ID: uuid.New(), Body: "lovely 😍", LabelIsLead: &no}
	store.leads["c"] = &model.MetaLead{ID: uuid.New(), Body: "not labelled"}
	if _, err := svc.StartEval(context.Background(), org, uuid.New(), []string{"no-provider"}, 1); err == nil {
		t.Fatal("a model must be provider:model")
	}

	e, err := svc.StartEval(context.Background(), org, uuid.New(), []string{"good:m", "bad:x", "good:m"}, 9)
	if err != nil {
		t.Fatal(err)
	}
	if e.CaseCount != 2 || e.Runs != maxEvalRuns || len(e.Models) != 2 {
		t.Fatalf("eval = %+v", e)
	}
	var got []model.MetaLeadEval
	waitFor(t, func() bool {
		got, _, _ = svc.ListEvals(context.Background(), org)
		return got[0].Status == model.MetaLeadRunStatusDone
	})
	good, bad := got[0].Report[0], got[0].Report[1]
	if good.Accuracy != 1 || good.Consistency != 1 || good.Grounded != 1 || bad.Error == "" {
		t.Fatalf("report = %+v", got[0].Report)
	}
}
