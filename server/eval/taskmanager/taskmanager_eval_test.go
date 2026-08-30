// Package taskmanager_eval evaluates the Agent Run Contract — the single front
// door every surface uses to start an agent.
//
// The defect it guards against is the one that motivated the contract: "run
// agent X with inputs Y" was implemented three separate times, so the same
// agent could behave differently depending on whether it was launched from the
// Task Manager, from chat, or through the generic task-run path.
package taskmanager_eval

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/eval/harness"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/platformtools"
	"github.com/jobshout/server/internal/service"
)

func builtinAgent(orgID uuid.UUID, name, builtin string) model.Agent {
	return model.Agent{
		ID: uuid.New(), OrgID: orgID, Name: name, Role: "specialist",
		Metadata: map[string]any{model.MetadataKeyBuiltin: builtin},
	}
}

type rig struct {
	svc     service.AgentRunService
	runs    *memAgentRunRepo
	agents  *memAgentRepo
	runners map[string]*recordingRunner
	orgID   uuid.UUID
	byName  map[string]model.Agent
}

// newRig wires the real AgentRunService over recording runners, so the checks
// exercise the service's own validation and dispatch rather than a fake of it.
func newRig(t *testing.T) *rig {
	t.Helper()
	orgID := uuid.New()

	specs := []struct{ name, builtin, kind string }{
		{"Article Writer", model.BuiltinArticleWriter, "blog_run"},
		{"Research Agent", model.BuiltinResearcher, "research_run"},
		{"Pentester", model.BuiltinPentester, "pentest_run"},
		{"PR Reviewer", model.BuiltinPRReviewer, "review_run"},
		{"Mail Agent", model.BuiltinMail, "mail_sync"},
	}

	byName := map[string]model.Agent{}
	agents := []model.Agent{}
	runners := map[string]*recordingRunner{}
	var asRunners []service.AgentRunner

	for _, s := range specs {
		a := builtinAgent(orgID, s.name, s.builtin)
		agents = append(agents, a)
		byName[s.name] = a
		r := &recordingRunner{builtin: s.builtin, kind: s.kind, externalID: uuid.NewString()}
		runners[s.builtin] = r
		asRunners = append(asRunners, r)
	}
	// A user-created agent with no builtin marker, taking the generic path.
	custom := model.Agent{ID: uuid.New(), OrgID: orgID, Name: "Custom", Role: "helper"}
	agents = append(agents, custom)
	byName["Custom"] = custom
	generic := &recordingRunner{builtin: "", kind: "task_run", externalID: uuid.NewString()}
	runners[""] = generic
	asRunners = append(asRunners, generic)

	repo := newMemAgentRepo(agents...)
	runs := newMemAgentRunRepo()
	return &rig{
		svc:     service.NewAgentRunService(runs, repo, zap.NewNop(), asRunners...),
		runs:    runs,
		agents:  repo,
		runners: runners,
		orgID:   orgID,
		byName:  byName,
	}
}

func (r *rig) start(t *testing.T, agentName string, inputs map[string]string, source string) (*model.AgentRun, error) {
	t.Helper()
	run, _, err := r.svc.Start(context.Background(), r.orgID, model.CreateAgentRunRequest{
		AgentID: r.byName[agentName].ID,
		Inputs:  inputs,
	}, nil, source)
	return run, err
}

func TestAgentRunContractEval(t *testing.T) {
	suite := harness.NewSuite(t, "task-manager", ".")

	t.Run("every_builtin_dispatches_to_its_own_runner", func(t *testing.T) {
		r := newRig(t)
		cases := []struct {
			agent, builtin string
			inputs         map[string]string
		}{
			{"Article Writer", model.BuiltinArticleWriter, map[string]string{"topic": "Kubernetes cost control"}},
			{"Research Agent", model.BuiltinResearcher, map[string]string{"topic": "Gateway API"}},
			{"Pentester", model.BuiltinPentester, map[string]string{"target": "https://int.example.com"}},
			{"PR Reviewer", model.BuiltinPRReviewer, map[string]string{"repo": "acme/api", "pr_number": "42"}},
			{"Mail Agent", model.BuiltinMail, map[string]string{}},
			{"Custom", "", map[string]string{"prompt": "summarise last week's incidents"}},
		}

		var checks []harness.Check
		for _, c := range cases {
			c := c
			run, err := r.start(t, c.agent, c.inputs, model.AgentRunSourceTaskManager)
			checks = append(checks,
				harness.Check{Name: c.agent + "_started", Fatal: true, Fn: func() error {
					if err != nil {
						return err
					}
					if run == nil {
						return errors.New("no run was returned")
					}
					return nil
				}},
				harness.Check{Name: c.agent + "_reached_its_runner", Fatal: true, Fn: func() error {
					calls, _ := r.runners[c.builtin].snapshot()
					return harness.RequireEqual("runner calls", calls, 1)
				}},
				harness.Check{Name: c.agent + "_records_the_external_row", Fatal: true, Fn: func() error {
					if run == nil || run.ExternalRunID == nil {
						return errors.New("run does not name the specialist row it created")
					}
					return harness.RequireEqual("external kind",
						run.ExternalKind, r.runners[c.builtin].kind)
				}},
			)
		}
		checks = append(checks, harness.Check{
			Name: "one_run_row_per_launch", Fatal: true, Fn: func() error {
				return harness.RequireEqual("run rows", r.runs.count(), len(cases))
			}},
		)
		suite.Case(t, "dispatch", checks)
	})

	t.Run("missing_required_input_is_a_question_not_a_failure", func(t *testing.T) {
		r := newRig(t)
		_, err := r.start(t, "Article Writer", map[string]string{}, model.AgentRunSourceTaskManager)
		miss, isMissing := service.AsMissingInput(err)

		suite.Case(t, "missing_input", []harness.Check{
			{Name: "reports_the_missing_slot", Fatal: true, Fn: func() error {
				if !isMissing {
					return errors.New("expected a missing-input error, got: " + errText(err))
				}
				return harness.RequireEqual("slot", miss.Missing[0], "topic")
			}},
			{Name: "asks_a_question", Fatal: true, Fn: func() error {
				if miss == nil || miss.Question == "" {
					return errors.New("no question to put to the user")
				}
				return nil
			}},
			// Nothing may be written before the inputs are known: a half-filled
			// form must not leave a run on the board.
			{Name: "writes_nothing_yet", Fatal: true, Fn: func() error {
				return harness.RequireZero("run rows", r.runs.count())
			}},
			{Name: "does_not_reach_the_runner", Fatal: true, Fn: func() error {
				calls, _ := r.runners[model.BuiltinArticleWriter].snapshot()
				return harness.RequireZero("runner calls", calls)
			}},
		})
	})

	t.Run("defaults_are_applied_before_dispatch", func(t *testing.T) {
		r := newRig(t)
		_, err := r.start(t, "Pentester",
			map[string]string{"target": "https://int.example.com"}, model.AgentRunSourceTaskManager)
		_, inputs := r.runners[model.BuiltinPentester].snapshot()

		suite.Case(t, "defaults", []harness.Check{
			{Name: "start_succeeds_without_the_defaulted_field", Fatal: true, Fn: func() error {
				return err
			}},
			{Name: "scan_mode_defaults_to_quick", Fatal: true, Fn: func() error {
				return harness.RequireEqual("scan_mode", inputs["scan_mode"], "quick")
			}},
		})
	})

	t.Run("a_failing_runner_is_recorded_not_swallowed", func(t *testing.T) {
		r := newRig(t)
		r.runners[model.BuiltinResearcher].err = errors.New("research is not configured")
		run, err := r.start(t, "Research Agent",
			map[string]string{"topic": "anything"}, model.AgentRunSourceTaskManager)

		var stored *model.AgentRun
		if run != nil {
			stored, _ = r.runs.GetByID(context.Background(), run.ID)
		}

		suite.Case(t, "runner_failure", []harness.Check{
			{Name: "the_run_row_exists", Fatal: true, Fn: func() error {
				if stored == nil {
					return errors.New("a failed dispatch left no record")
				}
				return nil
			}},
			{Name: "status_is_failed", Fatal: true, Fn: func() error {
				return harness.RequireEqual("status", stored.Status, model.AgentRunFailed)
			}},
			{Name: "the_reason_is_kept", Fatal: true, Fn: func() error {
				if stored.ErrorMessage == nil || *stored.ErrorMessage == "" {
					return errors.New("failed run has no error message")
				}
				return harness.RequireContains("error", *stored.ErrorMessage, "not configured")
			}},
			// Start still returns the run rather than only an error, so the
			// caller has something to show and link to.
			{Name: "caller_still_gets_a_run", Fn: func() error {
				if err != nil && run == nil {
					return errors.New("dispatch failure gave the caller nothing to reference")
				}
				return nil
			}},
		})
	})

	t.Run("an_agent_from_another_org_is_not_runnable", func(t *testing.T) {
		r := newRig(t)
		_, _, err := r.svc.Start(context.Background(), uuid.New(), model.CreateAgentRunRequest{
			AgentID: r.byName["Article Writer"].ID,
			Inputs:  map[string]string{"topic": "x"},
		}, nil, model.AgentRunSourceAPI)

		suite.Case(t, "org_isolation", []harness.Check{
			{Name: "refused", Fatal: true, Fn: func() error {
				if err == nil {
					return errors.New("an agent in another organisation was runnable")
				}
				return nil
			}},
			{Name: "nothing_written", Fatal: true, Fn: func() error {
				return harness.RequireZero("run rows", r.runs.count())
			}},
		})
	})

	// The executable form of "there is one way to run an agent". If these two
	// ever diverge, the contract has been bypassed again.
	t.Run("chat_and_task_manager_produce_the_same_run", func(t *testing.T) {
		r := newRig(t)
		inputs := map[string]string{"topic": "Kubernetes cost control"}

		fromTaskManager, err := r.start(t, "Article Writer", inputs, model.AgentRunSourceTaskManager)
		if err != nil {
			t.Fatalf("task manager launch failed: %v", err)
		}
		_, tmInputs := r.runners[model.BuiltinArticleWriter].snapshot()

		// Now the same request through the chat tool.
		reg := platformtools.NewRegistryWithTools(platformtools.Deps{
			Agents:    &stubAgentService{repo: r.agents},
			AgentRuns: r.svc,
		})
		tool, ok := reg.Get("agent_execute")
		if !ok {
			t.Fatal("agent_execute is not registered")
		}
		ctx := platformtools.WithIdentity(context.Background(),
			platformtools.Identity{OrgID: r.orgID, UserID: uuid.New()})
		res, err := tool.Run(ctx, map[string]any{
			"name": "Article Writer", "topic": "Kubernetes cost control",
		})
		if err != nil {
			t.Fatalf("chat launch failed: %v", err)
		}
		_, chatInputs := r.runners[model.BuiltinArticleWriter].snapshot()

		all := r.runs.all()

		suite.Case(t, "one_front_door", []harness.Check{
			{Name: "chat_did_not_ask_a_question", Fatal: true, Fn: func() error {
				if res != nil && len(res.Missing) > 0 {
					return errors.New("chat asked for input it already had: " + res.Question)
				}
				return nil
			}},
			{Name: "both_surfaces_recorded_a_run", Fatal: true, Fn: func() error {
				return harness.RequireEqual("run rows", len(all), 2)
			}},
			{Name: "both_reached_the_same_runner_with_the_same_inputs", Fatal: true, Fn: func() error {
				return harness.RequireEqual("topic",
					chatInputs["topic"], tmInputs["topic"])
			}},
			{Name: "runs_differ_only_by_source", Fatal: true, Fn: func() error {
				a, b := all[0], all[1]
				if err := harness.RequireEqual("agent", a.AgentID, b.AgentID); err != nil {
					return err
				}
				if err := harness.RequireEqual("builtin", a.Builtin, b.Builtin); err != nil {
					return err
				}
				if err := harness.RequireEqual("external kind", a.ExternalKind, b.ExternalKind); err != nil {
					return err
				}
				if string(a.Inputs) != string(b.Inputs) {
					return errors.New("the two surfaces recorded different inputs: " +
						string(a.Inputs) + " vs " + string(b.Inputs))
				}
				if a.Source == b.Source {
					return errors.New("both runs claim the same source; one should be chat")
				}
				return nil
			}},
			{Name: "chat_run_is_attributed_to_chat", Fatal: true, Fn: func() error {
				for _, run := range all {
					if run.Source == model.AgentRunSourceChat {
						return nil
					}
				}
				return errors.New("no run was recorded as coming from chat")
			}},
			{Name: "task_manager_run_is_attributed_correctly", Fatal: true, Fn: func() error {
				return harness.RequireEqual("source",
					fromTaskManager.Source, model.AgentRunSourceTaskManager)
			}},
			{Name: "inputs_are_replayable", Fatal: true, Fn: func() error {
				var decoded map[string]string
				if err := json.Unmarshal(all[0].Inputs, &decoded); err != nil {
					return err
				}
				return harness.RequireEqual("stored topic",
					decoded["topic"], "Kubernetes cost control")
			}},
		})
	})

	// The contract is only "one front door" if the specialist tools use it too.
	// They did not: research_run, article_generate, pentest_start and mail_sync
	// each called their own service, so an agent started by name from chat was
	// recorded and an agent started by capability was not. A live chat turn
	// found this after the contract had supposedly landed.
	t.Run("specialist_tools_use_the_front_door", func(t *testing.T) {
		r := newRig(t)
		reg := platformtools.NewRegistryWithTools(platformtools.Deps{
			Agents:    &stubAgentService{repo: r.agents},
			AgentRuns: r.svc,
			Research:  stubResearchSvc{},
			Blog:      stubBlogSvc{},
			Pentest:   stubPentestSvc{},
			Mail:      stubMailSvc{},
		})
		ctx := platformtools.WithIdentity(context.Background(),
			platformtools.Identity{OrgID: r.orgID, UserID: uuid.New()})

		calls := []struct {
			tool    string
			args    map[string]any
			builtin string
		}{
			{"research_run", map[string]any{"topic": "grid storage"}, model.BuiltinResearcher},
			{"article_generate", map[string]any{"topic": "grid storage"}, model.BuiltinArticleWriter},
			{"pentest_start", map[string]any{"target": "https://example.com", "scan_mode": "quick"}, model.BuiltinPentester},
			{"mail_sync", map[string]any{"senders": "sales@example.com"}, model.BuiltinMail},
		}

		checks := []harness.Check{}
		for _, c := range calls {
			c := c
			tool, ok := reg.Get(c.tool)
			if !ok {
				t.Fatalf("%s is not registered", c.tool)
			}
			// A panic here means the tool reached past the front door into its
			// specialist service, whose methods these stubs do not implement.
			res, err := tool.Run(ctx, c.args)
			checks = append(checks,
				harness.Check{Name: c.tool + "_did_not_call_its_service_directly", Fatal: true, Fn: func() error {
					return err
				}},
				harness.Check{Name: c.tool + "_did_not_ask_for_input_it_had", Fatal: true, Fn: func() error {
					if res != nil && len(res.Missing) > 0 {
						return errors.New(c.tool + " asked: " + res.Question)
					}
					return nil
				}},
				harness.Check{Name: c.tool + "_reached_its_runner", Fatal: true, Fn: func() error {
					n, _ := r.runners[c.builtin].snapshot()
					return harness.RequireAtLeast(c.tool+" runner calls", n, 1)
				}},
			)
		}

		all := r.runs.all()
		checks = append(checks,
			harness.Check{Name: "every_call_left_a_run_row", Fatal: true, Fn: func() error {
				return harness.RequireEqual("run rows", len(all), len(calls))
			}},
			harness.Check{Name: "every_run_is_attributed_to_chat", Fatal: true, Fn: func() error {
				for _, run := range all {
					if run.Source != model.AgentRunSourceChat {
						return errors.New("a specialist tool recorded source " + run.Source +
							"; chat-started work must say so")
					}
				}
				return nil
			}},
		)
		suite.Case(t, "specialist_front_door", checks)
	})
}

func errText(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
