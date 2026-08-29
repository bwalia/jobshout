// Package research_eval evaluates the Research Agent together with the run
// persistence added around it.
//
// The agent's own logic already has close unit coverage in internal/research;
// what had none was the seam this suite exercises — that a run leaves a row, in
// the right states, with the brief attached, whichever way it ends. Until those
// rows existed the agent could not appear on the board at all.
package research_eval

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/eval/harness"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
	"github.com/jobshout/server/internal/service"
)

// collapse reduces any run of whitespace to a single space, so a quote can be
// compared with a source document that wraps in a different place.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// docText is the page the fake backend serves. Quotes asserted below are taken
// from it, so a finding that does not appear here is an invented one.
const docText = `Kubernetes 1.31 promoted the Gateway API to general availability after an
extended beta period. The API separates operator and developer concerns into distinct
resources, which Ingress never did. Teams migrating large Ingress estates can use the
ingress2gateway conversion tool to generate equivalent Gateway resources automatically.`

const sourceURL = "https://kubernetes.io/blog/gateway-ga"

func goodBackend() *fixedBackend {
	return &fixedBackend{
		sources: []research.Source{{URL: sourceURL, Title: "Gateway API is GA"}},
		docs: map[string]*research.Document{
			sourceURL: {
				Source: research.Source{URL: sourceURL, Title: "Gateway API is GA", Site: "kubernetes.io"},
				Text:   docText,
			},
		},
	}
}

// scriptedModel answers each of the agent's four prompts. The triggers are the
// agent's own phrasing, so a prompt rewrite that changes them fails loudly here
// rather than silently returning an unmatched-call error at runtime.
func scriptedModel() *harness.FakeLLM {
	return harness.NewFakeLLM(
		harness.Script{Match: "planning research", Reply: `{"queries": ["kubernetes gateway api ga"]}`},
		harness.Script{Match: "extracting citable facts", Reply: `{"findings": [
			{"claim": "Gateway API reached GA in Kubernetes 1.31.",
			 "quote": "Kubernetes 1.31 promoted the Gateway API to general availability after an extended beta period."}
		]}`},
		harness.Script{Match: "fact-checking citations", Reply: `{"verdicts": [{"index": 0, "supported": true}]}`},
		harness.Script{Match: "Summarise the current state", Reply: "Gateway API is now GA."},
	)
}

type rig struct {
	svc    service.ResearchService
	runs   *memRunRepo
	agents *memAgentRepo
	back   *fixedBackend
	orgID  uuid.UUID
}

func newRig(t *testing.T, back *fixedBackend, llm *harness.FakeLLM) *rig {
	t.Helper()
	client := research.NewWith(back, []research.Searcher{back}, nil, zap.NewNop())
	cfg := research.DefaultAgentConfig()
	cfg.MinFindings = 1
	agent := research.NewAgent(client, llm, cfg, zap.NewNop())

	runs := newMemRunRepo()
	agents := newMemAgentRepo()
	return &rig{
		svc:    service.NewResearchService(agent, client, agents, runs, zap.NewNop()),
		runs:   runs,
		agents: agents,
		back:   back,
		orgID:  uuid.New(),
	}
}

func TestResearchEval(t *testing.T) {
	suite := harness.NewSuite(t, "research", ".")

	t.Run("happy_path_records_a_completed_run", func(t *testing.T) {
		r := newRig(t, goodBackend(), scriptedModel())
		out, err := r.svc.Run(context.Background(), r.orgID,
			research.Request{Topic: "Kubernetes Gateway API"}, nil,
			service.ResearchRunOptions{Source: model.ResearchSourceTaskManager})
		if err != nil {
			t.Fatalf("research failed: %v", err)
		}

		run := r.runs.only()
		suite.Case(t, "happy_path", []harness.Check{
			{Name: "brief_is_usable", Fatal: true, Fn: func() error {
				if !out.Brief.IsUsable() {
					return errors.New("brief reports itself unusable")
				}
				return nil
			}},
			{Name: "every_finding_is_sourced", Fatal: true, Fn: func() error {
				for _, f := range out.Brief.Findings {
					if f.SourceURL == "" || f.Quote == "" {
						return errors.New("a finding arrived without its citation")
					}
				}
				return nil
			}},
			{Name: "findings_quote_the_real_document", Fatal: true, Fn: func() error {
				// Compared with whitespace collapsed, because the document
				// wraps mid-sentence and a quote is judged by its words, not
				// by where the source happened to break the line.
				doc := collapse(docText)
				for _, f := range out.Brief.Findings {
					if err := harness.RequireContains("source document", doc, collapse(f.Quote)); err != nil {
						return err
					}
				}
				return nil
			}},
			{Name: "a_run_row_was_written", Fatal: true, Fn: func() error {
				return harness.RequireEqual("run rows", r.runs.count(), 1)
			}},
			{Name: "run_finished_completed", Fatal: true, Fn: func() error {
				return harness.RequireEqual("run status", run.Status, model.ResearchRunCompleted)
			}},
			{Name: "run_is_marked_usable", Fatal: true, Fn: func() error {
				return harness.RequireEqual("run usable", run.Usable, true)
			}},
			{Name: "run_carries_the_brief", Fatal: true, Fn: func() error {
				var stored research.Brief
				if err := json.Unmarshal(run.Brief, &stored); err != nil {
					return err
				}
				return harness.RequireEqual("stored findings",
					len(stored.Findings), len(out.Brief.Findings))
			}},
			{Name: "run_is_attributed_to_the_agent", Fatal: true, Fn: func() error {
				if run.AgentID == nil {
					return errors.New("run has no agent, so it cannot appear on the board")
				}
				return nil
			}},
			{Name: "run_records_its_source", Fatal: true, Fn: func() error {
				return harness.RequireEqual("run source", run.Source, model.ResearchSourceTaskManager)
			}},
			{Name: "run_completed_at_is_set", Fatal: true, Fn: func() error {
				if run.CompletedAt == nil {
					return errors.New("completed run has no completion time")
				}
				return nil
			}},
			{Name: "phases_were_recorded_for_the_board", Fatal: true, Fn: func() error {
				return harness.RequireAtLeast("phase updates", len(r.runs.phaseTrail()), 1)
			}},
		})
	})

	t.Run("unreadable_sources_fail_the_run_honestly", func(t *testing.T) {
		back := goodBackend()
		back.fetchErrs = map[string]error{sourceURL: errors.New("404 not found")}
		r := newRig(t, back, scriptedModel())

		_, err := r.svc.Run(context.Background(), r.orgID,
			research.Request{Topic: "Kubernetes Gateway API"}, nil,
			service.ResearchRunOptions{Source: model.ResearchSourceChat})
		run := r.runs.only()

		suite.Case(t, "unreadable_sources", []harness.Check{
			// The agent must refuse rather than return a confident empty brief.
			// A research agent that invents when it read nothing is worse than
			// one that errors.
			{Name: "research_reports_failure", Fatal: true, Fn: func() error {
				if err == nil {
					return errors.New("research succeeded despite reading nothing")
				}
				return nil
			}},
			{Name: "run_row_records_the_failure", Fatal: true, Fn: func() error {
				if run == nil {
					return errors.New("no run row was written for the failed run")
				}
				return harness.RequireEqual("run status", run.Status, model.ResearchRunFailed)
			}},
			{Name: "failure_reason_is_kept", Fatal: true, Fn: func() error {
				if run.ErrorMessage == nil || *run.ErrorMessage == "" {
					return errors.New("failed run has no error message")
				}
				return nil
			}},
			{Name: "failed_run_is_not_marked_usable", Fatal: true, Fn: func() error {
				return harness.RequireEqual("run usable", run.Usable, false)
			}},
		})
	})

	t.Run("pinned_urls_are_read_not_searched", func(t *testing.T) {
		r := newRig(t, goodBackend(), scriptedModel())
		_, err := r.svc.Run(context.Background(), r.orgID,
			research.Request{Topic: "price", URLs: []string{sourceURL}}, nil,
			service.ResearchRunOptions{Source: model.ResearchSourceMail})
		if err != nil {
			t.Fatalf("pinned research failed: %v", err)
		}
		run := r.runs.only()

		suite.Case(t, "pinned_urls", []harness.Check{
			// This is the path the Mail Agent depends on: a link a sender
			// pasted is fetched directly rather than searched around.
			{Name: "the_pinned_page_was_fetched", Fatal: true, Fn: func() error {
				return harness.RequireSubset("fetched URLs",
					[]string{sourceURL}, r.back.fetchedURLs())
			}},
			{Name: "no_web_search_was_performed", Fatal: true, Fn: func() error {
				return harness.RequireZero("search calls", r.back.searchCalls())
			}},
			{Name: "run_records_the_pinned_urls", Fatal: true, Fn: func() error {
				return harness.RequireSubset("run URLs", []string{sourceURL}, run.URLs)
			}},
		})
	})

	t.Run("research_survives_an_unwritable_run_row", func(t *testing.T) {
		r := newRig(t, goodBackend(), scriptedModel())
		r.runs.failNext = errors.New("database is down")

		out, err := r.svc.Run(context.Background(), r.orgID,
			research.Request{Topic: "Kubernetes Gateway API"}, nil,
			service.ResearchRunOptions{Source: model.ResearchSourceAPI})

		suite.Case(t, "bookkeeping_never_fails_the_work", []harness.Check{
			// Trading a completed brief for an unwritable audit record would be
			// a bad exchange, so the run row is best-effort by design.
			{Name: "research_still_returns_a_brief", Fatal: true, Fn: func() error {
				if err != nil {
					return err
				}
				if out == nil || out.Brief == nil {
					return errors.New("no brief was returned")
				}
				return nil
			}},
			{Name: "no_run_row_was_invented", Fatal: true, Fn: func() error {
				return harness.RequireEqual("run rows", r.runs.count(), 0)
			}},
		})
	})
}
