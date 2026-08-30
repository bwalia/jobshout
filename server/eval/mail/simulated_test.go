//go:build evallive

// Tier 2 without a mailbox: the Mail Agent on a simulated inbox.
//
// The Tier 1 suite scripts the model, so it proves the pipeline is wired but
// says nothing about whether a draft is any good. The other Tier 2 suite needs
// a real Gmail account, which most machines do not have.
//
// This one sits between them. The messages are the same fixtures, the Gmail
// boundary and the Research Agent are still fakes — so the facts a draft may
// use are known exactly — but the classifier and drafter run against a real
// model. That makes the question answerable: given these facts and this email,
// did the agent write a reply a person would send?
//
//	OLLAMA_BASE_URL=http://localhost:11434 EVAL_MAIL_MODEL=qwen3-coder:30b \
//	  go test -tags=evallive ./eval/mail/ -run Simulated -v
package mail_eval

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/eval/harness"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
	"github.com/jobshout/server/internal/service"
)

func TestMailAgentOnSimulatedInbox(t *testing.T) {
	base := envOr("OLLAMA_BASE_URL", "http://localhost:11434")
	if !ollamaReachable(base) {
		t.Skipf("mail: no Ollama at %s; skipping the simulated-inbox suite", base)
	}
	modelName := envOr("EVAL_MAIL_MODEL", "qwen3-coder:30b")

	suite := harness.NewSuite(t, "mail-simulated", ".")
	suite.Tier = "2"

	// A pass here means "no invariant was broken". Whether the writing is any
	// good is a judgement a person still has to make, so the suite writes down
	// what it actually produced rather than only whether it liked it.
	transcript := &strings.Builder{}
	fmt.Fprintf(transcript, "# Simulated inbox — drafts written by %s\n", modelName)

	for _, fx := range loadFixtures(t) {
		fx := fx
		t.Run(fx.Name, func(t *testing.T) {
			r := executeLive(t, fx, base, modelName)
			suite.Case(t, fx.Name, liveChecks(t, r))
			record(transcript, r)
		})
	}

	t.Cleanup(func() {
		path := filepath.Join("out", "mail-simulated-drafts.md")
		if err := os.WriteFile(path, []byte(transcript.String()), 0o644); err != nil {
			t.Logf("could not write the draft transcript: %v", err)
		}
	})
}

// record appends one fixture's outcome in a form a person can read end to end:
// what arrived, what the agent was allowed to know, and what it wrote back.
func record(w *strings.Builder, r *run) {
	fmt.Fprintf(w, "\n## %s\n\n**Subject:** %s  \n**From:** %s\n\n> %s\n\n",
		r.fx.Name, r.fx.Message.Subject, r.fx.Message.FromEmail,
		strings.ReplaceAll(strings.TrimSpace(r.fx.Message.Body), "\n", "\n> "))

	if th := r.threadOrNil(); th != nil {
		fmt.Fprintf(w, "Thread status: `%s`\n\n", th.Status)
	}
	d := r.draftOrNil()
	if d == nil {
		fmt.Fprint(w, "_No draft — the agent judged this needs no reply._\n")
		return
	}
	fmt.Fprintf(w, "**Draft — %s**\n\n```\n%s\n```\n", d.Subject, strings.TrimSpace(d.Body))
}

// executeLive mirrors execute(), swapping the scripted model for a real one.
// Everything else — the fake Gmail boundary, the fixture research brief, the
// in-memory repository — is held identical so a difference in the result is a
// difference in the model's judgement and nothing else.
func executeLive(t *testing.T, fx fixture, base, modelName string) *run {
	t.Helper()
	orgID := uuid.New()
	repo := newMemMailRepo()
	agents := newMemAgentRepo()

	gm := &fakeGmail{messages: []mail.InboxMessage{fx.Message.inbox()}}

	rs := &fakeResearch{available: fx.Research.Available, brief: fx.Research.Brief}
	if fx.Research.Fail {
		rs.err = errors.New("research backend unavailable")
	}
	if rs.brief == nil && !fx.Research.Fail {
		rs.brief = &research.Brief{}
	}

	client := llm.NewOllamaClient(base, modelName)
	logger := zap.NewNop()
	cfg := mail.Config{
		ClientID:     "cid",
		ClientSecret: "csecret",
		TokenKey:     testKey,
		RedirectURL:  "https://jobshout.test/api/v1/mail/connection/oauth/callback",
	}
	svc := service.NewMailService(
		repo, agents, gm,
		mail.NewClassifier(client, logger),
		mail.NewDrafter(client, logger),
		rs, cfg, logger,
	)

	key, err := mail.KeyFromSecret(testKey)
	if err != nil {
		t.Fatalf("test key: %v", err)
	}
	enc, err := mail.Encrypt(key, []byte("refresh-token"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := repo.UpsertConnection(context.Background(), &model.MailConnection{
		OrgID:              orgID,
		GoogleEmail:        "ops@ourco.example",
		RefreshTokenEnc:    enc,
		WatchKnowledgeURLs: fx.Connection.KnowledgeURLs,
		ResearchFocus:      fx.Connection.ResearchFocus,
		ReplyInstructions:  fx.Connection.ReplyInstructions,
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	// A real model on a laptop is slow; the fixtures are short but there are
	// two calls per message.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	r := &run{fx: fx, gmail: gm, research: rs, repo: repo, orgID: orgID}
	r.err = svc.SyncNow(ctx, orgID)
	return r
}

// liveChecks judges the outcome rather than the wiring. The invariants that
// must hold whatever the model says are fatal; the quality judgements are not,
// because a non-fatal failure is a report on the model, not a broken build.
func liveChecks(t *testing.T, r *run) []harness.Check {
	t.Helper()
	th := r.threadOrNil()
	draft := r.draftOrNil()

	checks := []harness.Check{
		{Name: "sync_completed", Fatal: true, Fn: func() error { return r.err }},
		{Name: "never_sends_without_approval", Fatal: true, Fn: func() error {
			return harness.RequireZero("messages sent", len(r.gmail.sends))
		}},
		{Name: "thread_was_recorded", Fatal: true, Fn: func() error {
			if th == nil {
				return errors.New("no thread was written for the message")
			}
			return nil
		}},
	}

	if r.fx.Expect.DraftExpected {
		checks = append(checks,
			harness.Check{Name: "a_draft_was_written", Fatal: true, Fn: func() error {
				if draft == nil {
					return fmt.Errorf("the model declined to draft; thread status %q", statusOf(th))
				}
				return nil
			}},
			harness.Check{Name: "draft_is_not_empty", Fatal: true, Fn: func() error {
				if draft == nil {
					return nil // already reported
				}
				if strings.TrimSpace(draft.Body) == "" {
					return errors.New("the draft body is empty")
				}
				return nil
			}},
			harness.Check{Name: "draft_addresses_the_sender", Fn: func() error {
				if draft == nil {
					return nil
				}
				return harness.RequireEqual("recipient", draft.ToEmail, r.fx.Message.FromEmail)
			}},
			harness.Check{Name: "draft_cites_only_researched_sources", Fatal: true, Fn: func() error {
				if draft == nil {
					return nil
				}
				return harness.RequireSubset("draft URLs",
					harness.URLsIn(draft.Body), allowedURLs(r))
			}},
			harness.Check{Name: "draft_reads_like_an_email_not_a_form", Fn: func() error {
				if draft == nil {
					return nil
				}
				return notAMachineDump(draft.Body)
			}},
			harness.Check{Name: "draft_has_no_unfilled_placeholder", Fatal: true, Fn: func() error {
				if draft == nil {
					return nil
				}
				return noPlaceholders(draft.Body)
			}},
			harness.Check{Name: "draft_is_a_sendable_length", Fn: func() error {
				if draft == nil {
					return nil
				}
				n := len(strings.Fields(draft.Body))
				// The floor only applies when nobody asked for brevity. An
				// operator whose instruction reads "Terse. No greeting, no
				// sign-off." has already decided how long a reply should be,
				// and an eval that overrules them is testing the wrong thing —
				// this check failed a correct 10-word answer before it said so.
				if n < 12 && strings.TrimSpace(r.fx.Connection.ReplyInstructions) == "" {
					return fmt.Errorf("the draft is %d words — too short to answer anything", n)
				}
				if n > 400 {
					return fmt.Errorf("the draft is %d words — nobody sends that as a reply", n)
				}
				return nil
			}},
		)
		// The brief's headline case: the facts researched from the sender's own
		// link have to actually reach the reply, or the research was for show.
		for _, want := range r.fx.Expect.DraftMentions {
			want := want
			checks = append(checks, harness.Check{
				Name: "draft_uses_the_researched_fact_" + slug(want),
				Fn: func() error {
					if draft == nil {
						return nil
					}
					return harness.RequireContains("draft body", draft.Body, want)
				},
			})
		}
	} else {
		checks = append(checks, harness.Check{
			Name: "no_draft_for_mail_that_needs_none", Fatal: true, Fn: func() error {
				if draft != nil {
					return fmt.Errorf("drafted a reply to %q, which needs none", r.fx.Message.Subject)
				}
				return nil
			},
		})
	}

	return checks
}

// allowedURLs is every URL the agent could honestly cite: the ones it was given
// and the ones research returned. Anything else in a draft is invented.
func allowedURLs(r *run) []string {
	out := harness.URLsIn(r.fx.Message.Body)
	out = append(out, r.fx.Connection.KnowledgeURLs...)
	if r.fx.Research.Brief != nil {
		for _, s := range r.fx.Research.Brief.Sources {
			out = append(out, s.URL)
		}
		for _, f := range r.fx.Research.Brief.Findings {
			out = append(out, f.SourceURL)
		}
	}
	return out
}

// noPlaceholders catches the defect that makes a draft unsendable however well
// it reads: a template token the model left for someone to fill in. The Mail
// Agent's whole contract is that a human approves and sends what it wrote, so
// "[Your Name]" is not a small blemish — it is the draft failing to be a draft.
var placeholderPattern = regexp.MustCompile(`\[(?i:your |insert |name|company|position|title|signature|sender|contact)[^\]]*\]|\{\{[^}]+\}\}|<[A-Z_ ]{3,}>`)

func noPlaceholders(body string) error {
	if m := placeholderPattern.FindString(body); m != "" {
		return fmt.Errorf("the draft still contains the placeholder %q — a human would have to edit it before sending", m)
	}
	return nil
}

// notAMachineDump catches the failure mode a small model falls into most often:
// answering with its scaffolding — JSON, a bullet list of fields, or the prompt
// echoed back — instead of prose a person would send.
func notAMachineDump(body string) error {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return errors.New("the draft is raw JSON, not an email")
	}
	for _, leak := range []string{"```", "\"body\":", "\"subject\":", "As an AI", "<think>"} {
		if strings.Contains(body, leak) {
			return fmt.Errorf("the draft leaks scaffolding: %q", leak)
		}
	}
	return nil
}

func (r *run) threadOrNil() *model.MailThread {
	ths := r.repo.threadsOf(r.orgID)
	if len(ths) == 0 {
		return nil
	}
	return &ths[0]
}

func (r *run) draftOrNil() *model.MailDraft {
	ds := r.repo.allDrafts()
	if len(ds) == 0 {
		return nil
	}
	return &ds[0]
}

func statusOf(th *model.MailThread) string {
	if th == nil {
		return "<no thread>"
	}
	return th.Status
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func ollamaReachable(base string) bool {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(strings.TrimRight(base, "/") + "/api/tags")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
