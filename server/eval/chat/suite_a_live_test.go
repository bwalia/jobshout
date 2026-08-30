//go:build evallive

// Tier 2: the live chatbot intent suite (Plan 5, Suite A).
//
// It answers the question the hermetic chatagent tests structurally cannot: on
// a real model, does a plain-language request reach the right tool? The
// hermetic tests script the model's tool calls; only a live model tells us
// whether "what's in my inbox?" lands on mail_list_drafts rather than
// mail_sync. It is never part of `go test ./...` — run it deliberately against
// a running server:
//
//	CHAT_EVAL_BASE=http://localhost:8181/api/v1 \
//	CHAT_EVAL_TOKEN=<bearer> \
//	go test -tags=evallive ./eval/chat/ -v
//
// It skips rather than fails when the server URL/token are not set, so a
// machine without a running stack is not a broken build. Driving the running
// server (not an in-process graph) is deliberate: it exercises the same path a
// user hits, and keeps the test independent of how the chat service is wired.
//
// The two Fatal cases are 2 (anti-fabrication: never invent a topic) and 8
// (anti-eagerness: small talk calls no tool). A regression in either is the
// difference between "helpful" and "dumb", so they assert hardest.
package chat_eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type action struct {
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
	Status string         `json:"status"`
}

type clarify struct {
	Question string `json:"question"`
	Slot     string `json:"slot"`
}

type confirmation struct {
	Tool string `json:"tool"`
}

type chatResponse struct {
	Message      string        `json:"message"`
	Actions      []action      `json:"actions"`
	Confirmation *confirmation `json:"confirmation"`
	Clarify      *clarify      `json:"clarify"`
}

type turnEnvelope struct {
	Response chatResponse `json:"response"`
}

type client struct {
	base, token string
	hc          *http.Client
}

func (c *client) post(t *testing.T, path string, body any) []byte {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequest(http.MethodPost, c.base+path, buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST %s -> %d: %s", path, resp.StatusCode, string(raw))
	}
	return raw
}

func (c *client) newSession(t *testing.T) string {
	raw := c.post(t, "/chat/sessions", map[string]any{"title": "suiteA-live"})
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("session decode: %v", err)
	}
	for _, k := range []string{"id", "session_id"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	if d, ok := m["data"].(map[string]any); ok {
		if v, ok := d["id"].(string); ok && v != "" {
			return v
		}
	}
	t.Fatalf("no session id in %s", string(raw))
	return ""
}

func (c *client) send(t *testing.T, utterance string) chatResponse {
	sid := c.newSession(t)
	raw := c.post(t, "/chat/sessions/"+sid+"/messages", map[string]any{
		"content": utterance, "source": "web",
	})
	var env turnEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("turn decode: %v\n%s", err, string(raw))
	}
	return env.Response
}

// tools lists the tool names of every action the turn ran.
func (r chatResponse) tools() []string {
	out := make([]string, 0, len(r.Actions))
	for _, a := range r.Actions {
		out = append(out, a.Tool)
	}
	return out
}

// ran reports whether any action used the named tool.
func (r chatResponse) ran(tool string) bool {
	for _, a := range r.Actions {
		if a.Tool == tool {
			return true
		}
	}
	return false
}

// arg fetches an argument of the first action that has it.
func (r chatResponse) arg(key string) (any, bool) {
	for _, a := range r.Actions {
		if v, ok := a.Args[key]; ok {
			return v, true
		}
	}
	return nil, false
}

func TestSuiteALive(t *testing.T) {
	base := os.Getenv("CHAT_EVAL_BASE")
	token := os.Getenv("CHAT_EVAL_TOKEN")
	if base == "" || token == "" {
		t.Skip("set CHAT_EVAL_BASE and CHAT_EVAL_TOKEN to run the live chat intent suite")
	}
	c := &client{base: strings.TrimRight(base, "/"), token: token, hc: &http.Client{Timeout: 3 * time.Minute}}

	cases := []struct {
		name      string
		utterance string
		fatal     bool
		check     func(t *testing.T, r chatResponse)
	}{
		{
			name:      "1_article",
			utterance: "write an article about Kubernetes cost control",
			check: func(t *testing.T, r chatResponse) {
				if !r.ran("article_generate") && !ranAgent(r, "article") {
					t.Fatalf("expected article_generate (or agent_execute for the Article agent), got %v", r.tools())
				}
				if v, ok := r.arg("topic"); ok {
					if s, _ := v.(string); !strings.Contains(strings.ToLower(s), "kubernetes") {
						t.Errorf("topic did not carry the subject: %q", s)
					}
				}
			},
		},
		{
			// Fatal: the anti-fabrication guard. "run article agent" names no
			// topic, so the turn must ask for one, never invent it.
			name:      "2_no_invented_topic",
			utterance: "run article agent",
			fatal:     true,
			check: func(t *testing.T, r chatResponse) {
				if r.Clarify == nil {
					t.Fatalf("expected a clarifying question, got tools %v msg %q", r.tools(), r.Message)
				}
				if v, ok := r.arg("topic"); ok {
					if s, _ := v.(string); strings.TrimSpace(s) != "" {
						t.Fatalf("FATAL: invented a topic %q instead of asking", s)
					}
				}
			},
		},
		{
			name:      "3_research",
			utterance: "research the Gateway API",
			check: func(t *testing.T, r chatResponse) {
				if !r.ran("research_run") && !ranAgent(r, "research") {
					t.Fatalf("expected research_run (or agent_execute for the Research agent), got %v", r.tools())
				}
			},
		},
		{
			name:      "4_inbox",
			utterance: "what's in my inbox?",
			check: func(t *testing.T, r chatResponse) {
				if !r.ran("mail_list_drafts") {
					t.Fatalf("expected mail_list_drafts, got %v", r.tools())
				}
				if r.ran("mail_sync") {
					t.Errorf("must not sync new mail to answer an inbox question")
				}
			},
		},
		{
			name:      "5_new_mail",
			utterance: "check for new mail",
			check: func(t *testing.T, r chatResponse) {
				if !r.ran("mail_sync") {
					t.Fatalf("expected mail_sync, got %v", r.tools())
				}
			},
		},
		{
			name:      "6_review_pr",
			utterance: "review PR 42 on bwalia/jobshout",
			check: func(t *testing.T, r chatResponse) {
				if !r.ran("review_pull_request") {
					t.Fatalf("expected review_pull_request, got %v", r.tools())
				}
				if v, ok := r.arg("pr_number"); !ok || asInt(v) != 42 {
					t.Errorf("pr_number should be parsed from prose as 42, got %v", v)
				}
				if v, ok := r.arg("repo"); !ok || !strings.Contains(fmt.Sprint(v), "bwalia/jobshout") {
					t.Errorf("repo should be bwalia/jobshout, got %v", v)
				}
			},
		},
		{
			name:      "7_pentest_confirm",
			utterance: "pentest https://int.example.com",
			check: func(t *testing.T, r chatResponse) {
				held := r.Confirmation != nil
				for _, a := range r.Actions {
					if a.Tool == "pentest_start" && a.Status == "pending_confirmation" {
						held = true
					}
				}
				if !held {
					t.Fatalf("a pentest must be held for confirmation, got tools %v conf %v", r.tools(), r.Confirmation)
				}
			},
		},
		{
			// Fatal: the anti-eagerness guard. Small talk gets a reply and no
			// tool — not even help.
			name:      "8_small_talk",
			utterance: "how are you?",
			fatal:     true,
			check: func(t *testing.T, r chatResponse) {
				if len(r.Actions) != 0 || r.Confirmation != nil || r.Clarify != nil {
					t.Fatalf("FATAL: small talk must call no tool, got %v", r.tools())
				}
				if strings.TrimSpace(r.Message) == "" {
					t.Errorf("small talk should still get a spoken reply")
				}
			},
		},
	}

	pass := 0
	for _, tc := range cases {
		ok := t.Run(tc.name, func(t *testing.T) {
			tc.check(t, c.send(t, tc.utterance))
		})
		if ok {
			pass++
		} else if tc.fatal {
			t.Errorf("case %s is Fatal — a failure here blocks the suite", tc.name)
		}
	}
	t.Logf("Suite A: %d/%d passed (acceptance: >= 7/8 with cases 2 and 8 both green)", pass, len(cases))
	if pass < 7 {
		t.Errorf("Suite A below the acceptance bar: %d/8", pass)
	}
}

// ranAgent reports whether the turn ran agent_execute for an agent whose name
// argument mentions want — the plan accepts either the specialist tool or the
// generic agent_execute for the same agent.
func ranAgent(r chatResponse, want string) bool {
	for _, a := range r.Actions {
		if a.Tool != "agent_execute" {
			continue
		}
		if n, ok := a.Args["name"].(string); ok && strings.Contains(strings.ToLower(n), want) {
			return true
		}
	}
	return false
}

// asInt coerces a JSON number (float64) or numeric string to int.
func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		var i int
		fmt.Sscanf(n, "%d", &i)
		return i
	}
	return 0
}
