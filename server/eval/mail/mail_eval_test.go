// Package mail_eval evaluates the Mail Agent end to end: a fixture message goes
// in at the Gmail boundary and the suite asserts on what came out — the thread
// status, whether a draft was written, what the Research Agent was asked, and
// above all that nothing was sent.
//
// The classifier and drafter under test are the real ones (mail.NewClassifier /
// mail.NewDrafter) driven by a scripted model, so the prompts and the JSON
// parsing are exercised rather than stubbed past.
package mail_eval

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/eval/harness"
	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
	"github.com/jobshout/server/internal/service"
)

// --- fixture shape -------------------------------------------------------

type fixture struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Message     message `json:"message"`

	// ClassifyReply and DraftReply are the scripted model replies. They are raw
	// JSON strings rather than structs so a fixture can also express a
	// malformed reply when we want to test the parser's fallback.
	ClassifyReply string `json:"classify_reply"`
	DraftReply    string `json:"draft_reply"`

	Connection connectionFixture `json:"connection"`
	Research   researchFixture   `json:"research"`
	Expect     expectations      `json:"expect"`
}

type message struct {
	GmailThreadID  string `json:"gmail_thread_id"`
	GmailMessageID string `json:"gmail_message_id"`
	FromEmail      string `json:"from_email"`
	FromName       string `json:"from_name"`
	ToEmail        string `json:"to_email"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
	Snippet        string `json:"snippet"`
}

type connectionFixture struct {
	KnowledgeURLs     []string `json:"knowledge_urls"`
	ResearchFocus     string   `json:"research_focus"`
	ReplyInstructions string   `json:"reply_instructions"`
}

type researchFixture struct {
	Available bool            `json:"available"`
	Fail      bool            `json:"fail"`
	Brief     *research.Brief `json:"brief"`
}

type expectations struct {
	ThreadStatus     string `json:"thread_status"`
	DraftExpected    bool   `json:"draft_expected"`
	ResearchExpected bool   `json:"research_expected"`

	SenderURLsReachResearch  bool `json:"sender_urls_reach_research"`
	ResearchURLsArePinned    bool `json:"research_urls_are_pinned"`
	ReplyInstructionsInPromt bool `json:"reply_instructions_in_prompt"`
	SubjectNoDoublePrefix    bool `json:"subject_no_double_prefix"`
}

func (m message) inbox() mail.InboxMessage {
	return mail.InboxMessage{
		GmailThreadID:  m.GmailThreadID,
		GmailMessageID: m.GmailMessageID,
		FromEmail:      m.FromEmail,
		FromName:       m.FromName,
		ToEmail:        m.ToEmail,
		Subject:        m.Subject,
		Snippet:        m.Snippet,
		Body:           m.Body,
	}
}

// --- the run -------------------------------------------------------------

// run is one fixture driven through the pipeline, with everything the checks
// need to make an assertion.
type run struct {
	fx       fixture
	gmail    *fakeGmail
	research *fakeResearch
	llm      *harness.FakeLLM
	repo     *memMailRepo
	orgID    uuid.UUID
	err      error
}

// testKey is a 32-byte hex secret for token encryption at rest. It is a fixed
// literal because the eval must be deterministic, and it never leaves this file.
const testKey = "0000000000000000000000000000000000000000000000000000000000000001"

func execute(t *testing.T, fx fixture) *run {
	t.Helper()
	orgID := uuid.New()
	repo := newMemMailRepo()
	agents := newMemAgentRepo()

	msg := fx.Message.inbox()
	gm := &fakeGmail{messages: []mail.InboxMessage{msg}}

	rs := &fakeResearch{available: fx.Research.Available, brief: fx.Research.Brief}
	if fx.Research.Fail {
		rs.err = errors.New("research backend unavailable")
	}
	if rs.brief == nil && !fx.Research.Fail {
		rs.brief = &research.Brief{}
	}

	// Script the model: the classify prompt and the draft prompt are the two
	// calls the pipeline makes, and each carries a distinctive anchor string.
	fake := harness.NewFakeLLM(
		harness.Script{Match: "Classify this inbound email", Reply: fx.ClassifyReply},
		harness.Script{Match: "Draft a reply to this email", Reply: fx.DraftReply},
	)

	logger := zap.NewNop()
	cfg := mail.Config{
		ClientID:     "cid",
		ClientSecret: "csecret",
		TokenKey:     testKey,
		RedirectURL:  "https://jobshout.test/api/v1/mail/connection/oauth/callback",
	}
	svc := service.NewMailService(
		repo, agents, gm,
		mail.NewClassifier(fake, logger),
		mail.NewDrafter(fake, logger),
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
	conn := &model.MailConnection{
		OrgID:              orgID,
		GoogleEmail:        "ops@ourco.example",
		RefreshTokenEnc:    enc,
		WatchKnowledgeURLs: fx.Connection.KnowledgeURLs,
		ResearchFocus:      fx.Connection.ResearchFocus,
		ReplyInstructions:  fx.Connection.ReplyInstructions,
	}
	if err := repo.UpsertConnection(context.Background(), conn); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	r := &run{fx: fx, gmail: gm, research: rs, llm: fake, repo: repo, orgID: orgID}
	r.err = svc.SyncNow(context.Background(), orgID)
	return r
}

// thread returns the single processed thread.
func (r *run) thread(t *testing.T) model.MailThread {
	t.Helper()
	ths := r.repo.threadsOf(r.orgID)
	if len(ths) != 1 {
		t.Fatalf("expected exactly one thread, got %d", len(ths))
	}
	return ths[0]
}

// senderURLs are the links the fixture's sender actually pasted.
func (r *run) senderURLs() []string {
	return harness.URLsIn(r.fx.Message.Body)
}

// briefURLs are every URL the research brief vouched for.
func (r *run) briefURLs() []string {
	var out []string
	if r.fx.Research.Brief == nil {
		return nil
	}
	for _, f := range r.fx.Research.Brief.Findings {
		out = append(out, f.SourceURL)
	}
	for _, s := range r.fx.Research.Brief.Sources {
		out = append(out, s.URL)
	}
	return out
}

// --- the suite -----------------------------------------------------------

func TestMailAgentEval(t *testing.T) {
	suite := harness.NewSuite(t, "mail", ".")

	for _, fx := range loadFixtures(t) {
		fx := fx
		t.Run(fx.Name, func(t *testing.T) {
			r := execute(t, fx)
			if r.err != nil {
				t.Fatalf("sync failed: %v", r.err)
			}
			th := r.thread(t)
			drafts := r.repo.allDrafts()

			checks := []harness.Check{
				// The single most important property of this agent.
				{Name: "never_sends_without_approval", Fatal: true, Fn: func() error {
					return harness.RequireZero("gmail send", r.gmail.SendCount())
				}},
				{Name: "thread_reaches_expected_status", Fatal: true, Fn: func() error {
					return harness.RequireEqual("thread status", th.Status, fx.Expect.ThreadStatus)
				}},
				{Name: "draft_written_only_when_expected", Fatal: true, Fn: func() error {
					if fx.Expect.DraftExpected {
						return harness.RequireEqual("draft count", len(drafts), 1)
					}
					return harness.RequireZero("draft written", len(drafts))
				}},
				{Name: "research_called_only_when_expected", Fatal: true, Fn: func() error {
					if fx.Expect.ResearchExpected {
						return harness.RequireAtLeast("research calls", r.research.Calls(), 1)
					}
					return harness.RequireZero("research call", r.research.Calls())
				}},
				// The fabrication guard: a draft may only cite sources the
				// brief actually returned.
				{Name: "draft_cites_only_researched_sources", Fatal: true, Fn: func() error {
					if len(drafts) == 0 {
						return nil
					}
					return harness.RequireSubset("draft body",
						harness.URLsIn(drafts[0].Body), r.briefURLs())
				}},
				{Name: "draft_never_claims_sent", Fatal: true, Fn: func() error {
					if len(drafts) == 0 {
						return nil
					}
					return harness.RequireAbsent("draft body", drafts[0].Body,
						"i have sent", "message was sent", "已发送")
				}},
				{Name: "draft_prompt_forbids_claiming_sent", Fatal: true, Fn: func() error {
					if !fx.Expect.DraftExpected {
						return nil
					}
					return harness.RequireContains("draft prompt", r.llm.AllPrompts(),
						"Do not claim the reply has been sent")
				}},
				// The reference stored on the thread must be the id research
				// actually handed back. It used to be a fresh uuid.New(),
				// naming a row that did not exist anywhere.
				{Name: "research_reference_names_the_real_run", Fatal: true, Fn: func() error {
					if !fx.Expect.ResearchExpected || fx.Research.Fail {
						return nil
					}
					if th.ResearchBriefID == nil {
						return errors.New("thread kept no reference to the research run")
					}
					return harness.RequireEqual("stored research run id",
						*th.ResearchBriefID, r.research.RunID())
				}},
				{Name: "no_secrets_in_thread_error", Fatal: true, Fn: func() error {
					if th.ErrorMessage == nil {
						return nil
					}
					return harness.RequireAbsent("thread error", *th.ErrorMessage,
						"refresh-token", "csecret", testKey)
				}},
			}

			// The brief's headline promise: a link the sender pasted is the
			// thing the Research Agent is pointed at. research.Request.URLs is
			// what selects the direct-fetch path, so it is the field that
			// decides whether the link is read or merely searched around.
			if fx.Expect.SenderURLsReachResearch {
				checks = append(checks, harness.Check{
					Name: "sender_links_are_fetched_directly", Fatal: true,
					Fn: func() error {
						req, ok := r.research.Last()
						if !ok {
							return errors.New("research was never called")
						}
						return harness.RequireSubset(
							"sender links missing from research URLs",
							r.senderURLs(), req.URLs)
					},
				})
			}

			if fx.Expect.ResearchURLsArePinned {
				checks = append(checks, harness.Check{
					Name: "pinned_knowledge_urls_are_used", Fatal: true,
					Fn: func() error {
						req, ok := r.research.Last()
						if !ok {
							return errors.New("research was never called")
						}
						return harness.RequireSubset("research URLs",
							fx.Connection.KnowledgeURLs, req.URLs)
					},
				})
			}

			if fx.Expect.ReplyInstructionsInPromt {
				checks = append(checks, harness.Check{
					Name: "reply_style_reaches_the_draft_prompt", Fatal: true,
					Fn: func() error {
						return harness.RequireContains("draft prompt",
							r.llm.AllPrompts(), fx.Connection.ReplyInstructions)
					},
				})
			}

			if fx.Expect.SubjectNoDoublePrefix {
				checks = append(checks, harness.Check{
					Name: "subject_has_one_re_prefix", Fatal: true,
					Fn: func() error {
						if len(drafts) == 0 {
							return errors.New("no draft to inspect")
						}
						n := strings.Count(strings.ToLower(drafts[0].Subject), "re:")
						return harness.RequireEqual("Re: prefixes", n, 1)
					},
				})
			}

			rep := suite.Case(t, fx.Name, checks)
			rep.Note("%s", fx.Description)
		})
	}
}

func loadFixtures(t *testing.T) []fixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("fixtures", "*.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no fixtures found")
	}
	out := make([]fixture, 0, len(paths))
	for _, p := range paths {
		blob, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var fx fixture
		if err := json.Unmarshal(blob, &fx); err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		out = append(out, fx)
	}
	return out
}
