// Command apicheck hits a local JobShout API and walks Mail, Task Manager,
// Research, Article, and Chat scenarios. Mail uses MAIL_SIMULATE endpoints
// (fake inbox, no Google).
//
//	MAIL_SIMULATE=1 go run ./cmd/server   # in another terminal
//	go run ./cmd/apicheck -base http://127.0.0.1:8080
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	base := flag.String("base", envOr("APICHECK_BASE", "http://127.0.0.1:8080"), "API origin (no /api/v1 suffix)")
	slow := flag.Bool("slow", true, "also run research / article / chat (needs Ollama)")
	flag.Parse()

	c := &checker{
		base:   strings.TrimRight(*base, "/"),
		client: &http.Client{Timeout: 10 * time.Minute},
	}
	fmt.Printf("apicheck against %s\n", c.base)

	c.step("health", func() error {
		st, body, err := c.do(http.MethodGet, "/health", nil, false)
		if err != nil {
			return err
		}
		if st != 200 {
			return fmt.Errorf("status %d: %s", st, body)
		}
		return nil
	})

	stamp := time.Now().UnixNano()
	email := fmt.Sprintf("apicheck-%d@jobshout.local", stamp)
	c.step("register", func() error {
		st, body, err := c.do(http.MethodPost, "/api/v1/auth/register", map[string]any{
			"email": email, "password": "testpass1234",
			"full_name": "API Check", "org_name": fmt.Sprintf("API Check Org %d", stamp),
		}, false)
		if err != nil {
			return err
		}
		if st != 200 && st != 201 {
			return fmt.Errorf("status %d: %s", st, body)
		}
		c.token = str(asMap(body), "access_token")
		if c.token == "" {
			return fmt.Errorf("no access_token in %s", body)
		}
		return nil
	})
	if c.token == "" {
		fmt.Println("abort: no auth token")
		os.Exit(1)
	}

	c.step("seeded agents", func() error {
		st, body, err := c.do(http.MethodGet, "/api/v1/agents?per_page=100", nil, true)
		if err != nil {
			return err
		}
		if st != 200 {
			return fmt.Errorf("status %d: %s", st, body)
		}
		for _, name := range []string{"Article Writer", "Research Agent", "Mail Agent", "Career Agent"} {
			id := agentIDByName(body, name)
			if id == "" {
				return fmt.Errorf("missing agent %q", name)
			}
			c.agents[name] = id
		}
		return nil
	})

	c.step("create project Inbox", func() error {
		id, err := c.createProject("Inbox")
		if err != nil {
			return err
		}
		c.project = id
		return nil
	})

	c.step("mail launch without mailbox", func() error {
		st, body, err := c.launch(c.agents["Mail Agent"], c.project, map[string]string{})
		if err != nil {
			return err
		}
		if st != 202 {
			return fmt.Errorf("status %d: %s", st, body)
		}
		m := asMap(body)
		if truthy(m["sync_queued"]) {
			return fmt.Errorf("sync should not queue before connect: %s", body)
		}
		task := asMap(anyToJSON(m["task"]))
		if str(task, "id") == "" {
			return fmt.Errorf("no board task: %s", body)
		}
		c.mailTask = str(task, "id")
		return nil
	})

	c.step("simulate connect", func() error {
		st, body, err := c.do(http.MethodPost, "/api/v1/mail/simulate/connect", map[string]any{}, true)
		if err != nil {
			return err
		}
		if st == 404 {
			return fmt.Errorf("MAIL_SIMULATE is off on the server (got 404). Restart with MAIL_SIMULATE=1")
		}
		if st != 200 {
			return fmt.Errorf("status %d: %s", st, body)
		}
		m := asMap(body)
		if !truthy(m["connected"]) {
			return fmt.Errorf("not connected: %s", body)
		}
		return nil
	})

	c.step("GET connection is connected", func() error {
		st, body, err := c.do(http.MethodGet, "/api/v1/mail/connection", nil, true)
		if err != nil {
			return err
		}
		if st != 200 {
			return fmt.Errorf("status %d: %s", st, body)
		}
		m := asMap(body)
		if !truthy(m["connected"]) {
			return fmt.Errorf("GET connection not connected: %s", body)
		}
		return nil
	})

	c.step("mail launch after connect", func() error {
		st, body, err := c.launch(c.agents["Mail Agent"], c.project, map[string]string{
			"research_focus": "prices and availability",
		})
		if err != nil {
			return err
		}
		if st != 202 {
			return fmt.Errorf("status %d: %s", st, body)
		}
		if !truthy(asMap(body)["sync_queued"]) {
			return fmt.Errorf("expected sync_queued: %s", body)
		}
		return nil
	})

	c.step("M1 price + product URL → draft", func() error {
		return c.injectAndSync("th-price-"+fmt.Sprint(stamp), "client@acme.com",
			"What is the price of this machine?",
			"Hi — what is the price of this machine?\n\nhttps://vendor.example/machine-x\n\nThanks.",
			"draft_ready", true)
	})

	c.step("M2 availability + URL → draft", func() error {
		return c.injectAndSync("th-avail-"+fmt.Sprint(stamp), "buyer@shop.com",
			"Do you have this machine available?",
			"Can you confirm availability?\nhttps://vendor.example/lathe-200",
			"draft_ready", true)
	})

	c.step("M3 newsletter ignored", func() error {
		return c.injectAndSync("th-news-"+fmt.Sprint(stamp), "noreply@list.com",
			"This newsletter: August digest",
			"View in browser. Unsubscribe anytime. Weekly roundup.",
			"ignored", false)
	})

	c.step("M4 tracking link skipped, still drafts", func() error {
		return c.injectAndSync("th-track-"+fmt.Sprint(stamp), "ops@partner.com",
			"Can we jump on a call?",
			"Please reply.\nhttps://list-manage.com/unsubscribe/abc\nhttps://vendor.example/ok",
			"draft_ready", true)
	})

	c.step("M5 noreply bounce ignored", func() error {
		return c.injectAndSync("th-bounce-"+fmt.Sprint(stamp), "noreply@vendor.com",
			"Delivery Status Notification",
			"This is an automated message. Do not reply.",
			"ignored", false)
	})

	c.step("M6 playbook URL without body link → draft", func() error {
		st, body, err := c.do(http.MethodPatch, "/api/v1/mail/connection", map[string]any{
			"knowledge_urls": []string{"https://example.com/pricing"},
			"research_focus": "list prices and plan names only",
		}, true)
		if err != nil {
			return err
		}
		if st != 200 {
			return fmt.Errorf("patch connection %d: %s", st, body)
		}
		return c.injectAndSync("th-pinned-"+fmt.Sprint(stamp), "alex@c.com",
			"Team plan price?",
			"How much is the team plan? Thanks.",
			"draft_ready", true)
	})

	c.step("M7 sender watch skips strangers", func() error {
		st, body, err := c.do(http.MethodPatch, "/api/v1/mail/connection", map[string]any{
			"rules": map[string]any{"senders": []string{"vip@acme.com"}},
		}, true)
		if err != nil {
			return err
		}
		if st != 200 {
			return fmt.Errorf("patch senders %d: %s", st, body)
		}
		st, raw, err := c.do(http.MethodPost, "/api/v1/mail/simulate/inbox", map[string]any{
			"messages": []map[string]any{
				{"thread_id": "th-vip-" + fmt.Sprint(stamp), "from": "vip@acme.com", "subject": "Need the spec", "body": "Please send the spec when you can."},
				{"thread_id": "th-stranger-" + fmt.Sprint(stamp), "from": "random@elsewhere.com", "subject": "Buy now", "body": "Great deal https://vendor.example/x"},
			},
		}, true)
		if err != nil {
			return err
		}
		if st != 202 {
			return fmt.Errorf("inject %d: %s", st, raw)
		}
		st, raw, err = c.do(http.MethodPost, "/api/v1/mail/simulate/sync", map[string]any{}, true)
		if err != nil {
			return err
		}
		if st != 200 {
			return fmt.Errorf("sync %d: %s", st, raw)
		}
		vip, err := c.findThread("th-vip-"+fmt.Sprint(stamp), "Need the spec")
		if err != nil {
			return err
		}
		if vip == nil {
			return fmt.Errorf("watched sender thread missing")
		}
		stranger, err := c.findThread("th-stranger-"+fmt.Sprint(stamp), "Buy now")
		if err != nil {
			return err
		}
		if stranger != nil {
			return fmt.Errorf("unwatched sender should not be ingested: %s", anyToJSON(stranger))
		}
		return nil
	})

	c.step("drafts exist and never claim sent", func() error {
		st, body, err := c.do(http.MethodGet, "/api/v1/mail/drafts?per_page=50", nil, true)
		if err != nil {
			return err
		}
		if st != 200 {
			return fmt.Errorf("status %d: %s", st, body)
		}
		drafts := asSlice(asMap(body)["data"])
		if len(drafts) < 2 {
			return fmt.Errorf("want at least 2 drafts, got %d: %s", len(drafts), body)
		}
		for _, d := range drafts {
			m := asMap(anyToJSON(d))
			blob := strings.ToLower(str(m, "body") + " " + str(m, "subject"))
			if strings.Contains(blob, "has been sent") || strings.Contains(blob, "email was sent") {
				return fmt.Errorf("draft claims sent: %s", str(m, "body"))
			}
			if str(m, "status") != "draft" {
				return fmt.Errorf("pending draft status %q", str(m, "status"))
			}
			c.drafts = append(c.drafts, str(m, "id"))
		}
		return nil
	})

	c.step("reject draft does not send", func() error {
		if len(c.drafts) < 2 {
			return fmt.Errorf("need 2 drafts")
		}
		id := c.drafts[0]
		st, body, err := c.do(http.MethodPost, "/api/v1/mail/drafts/"+id+"/reject", map[string]any{}, true)
		if err != nil {
			return err
		}
		if st != 200 {
			return fmt.Errorf("status %d: %s", st, body)
		}
		if str(asMap(body), "status") != "rejected" {
			return fmt.Errorf("status %s", body)
		}
		return nil
	})

	c.step("approve draft simulates send", func() error {
		if len(c.drafts) < 2 {
			return fmt.Errorf("need 2 drafts")
		}
		id := c.drafts[1]
		st, body, err := c.do(http.MethodPost, "/api/v1/mail/drafts/"+id+"/approve", map[string]any{}, true)
		if err != nil {
			return err
		}
		if st != 200 {
			return fmt.Errorf("status %d: %s", st, body)
		}
		if str(asMap(body), "status") != "sent" {
			return fmt.Errorf("expected sent, got %s", body)
		}
		return nil
	})

	c.step("approve again is forbidden", func() error {
		id := c.drafts[1]
		st, body, err := c.do(http.MethodPost, "/api/v1/mail/drafts/"+id+"/approve", map[string]any{}, true)
		if err != nil {
			return err
		}
		if st != 403 {
			return fmt.Errorf("want 403, got %d: %s", st, body)
		}
		return nil
	})

	c.step("task still on board after mail launch", func() error {
		if c.mailTask == "" {
			return fmt.Errorf("no mail task id")
		}
		st, body, err := c.do(http.MethodGet, "/api/v1/tasks/"+c.mailTask, nil, true)
		if err != nil {
			return err
		}
		if st != 200 {
			return fmt.Errorf("status %d: %s", st, body)
		}
		return nil
	})

	if *slow {
		c.step("research launch (slow)", func() error {
			st, body, err := c.launch(c.agents["Research Agent"], c.project, map[string]string{
				"topic":   "Kubernetes cost optimisation",
				"context": "API check; keep it short",
			})
			if err != nil {
				return err
			}
			if st != 202 {
				return fmt.Errorf("status %d: %s", st, body)
			}
			task := asMap(anyToJSON(asMap(body)["task"]))
			if str(task, "id") == "" {
				return fmt.Errorf("no task: %s", body)
			}
			if str(asMap(body), "kind") != "researcher" {
				return fmt.Errorf("kind %s", body)
			}
			return nil
		})

		c.step("article launch (async)", func() error {
			st, body, err := c.launch(c.agents["Article Writer"], c.project, map[string]string{
				"topic":   "edge AI inference",
				"context": "API check; short article is fine",
			})
			if err != nil {
				return err
			}
			if st != 202 {
				return fmt.Errorf("status %d: %s", st, body)
			}
			m := asMap(body)
			task := asMap(anyToJSON(m["task"]))
			if str(task, "id") == "" || str(m, "run_id") == "" {
				return fmt.Errorf("want task + run_id: %s", body)
			}
			c.articleTask = str(task, "id")
			c.articleRun = str(m, "run_id")
			return nil
		})

		c.step("article run finishes", func() error {
			if c.articleRun == "" {
				return fmt.Errorf("no article run")
			}
			deadline := time.Now().Add(8 * time.Minute)
			var last string
			for time.Now().Before(deadline) {
				st, body, err := c.do(http.MethodGet, "/api/v1/blogs/runs/"+c.articleRun, nil, true)
				if err != nil {
					return err
				}
				if st != 200 {
					return fmt.Errorf("status %d: %s", st, body)
				}
				last = body
				status := str(asMap(body), "status")
				if status == "completed" {
					return nil
				}
				if status == "failed" {
					return fmt.Errorf("article run failed: %s", body)
				}
				time.Sleep(5 * time.Second)
			}
			return fmt.Errorf("article run still not done: %s", last)
		})

		c.step("re-run hydrates launch_values", func() error {
			if c.articleTask == "" {
				return fmt.Errorf("no article task")
			}
			st, body, err := c.do(http.MethodGet, "/api/v1/tasks/"+c.articleTask, nil, true)
			if err != nil {
				return err
			}
			if st != 200 {
				return fmt.Errorf("status %d: %s", st, body)
			}
			meta := asMap(anyToJSON(asMap(body)["metadata"]))
			vals := asMap(anyToJSON(meta["launch_values"]))
			if str(vals, "topic") != "edge AI inference" {
				return fmt.Errorf("launch_values.topic=%v meta=%v", vals["topic"], meta)
			}
			return nil
		})

		c.step("chat with one project does not interview project", func() error {
			st, body, err := c.do(http.MethodPost, "/api/v1/chat/route", map[string]any{
				"message": "research kubernetes cost optimisation",
			}, true)
			if err != nil {
				return err
			}
			if st != 200 {
				return fmt.Errorf("status %d: %s", st, body)
			}
			resp := asMap(anyToJSON(asMap(body)["response"]))
			clarify := asMap(anyToJSON(resp["clarify"]))
			if str(clarify, "slot") == "project" {
				return fmt.Errorf("single project must not interview project: %s", body)
			}
			return nil
		})

		c.step("second project then chat asks which", func() error {
			if _, err := c.createProject("Website"); err != nil {
				return err
			}
			st, body, err := c.do(http.MethodPost, "/api/v1/chat/route", map[string]any{
				"message": "research kubernetes cost optimisation",
			}, true)
			if err != nil {
				return err
			}
			if st != 200 {
				return fmt.Errorf("status %d: %s", st, body)
			}
			m := asMap(body)
			resp := asMap(anyToJSON(m["response"]))
			clarify := asMap(anyToJSON(resp["clarify"]))
			slot := str(clarify, "slot")
			msg := strings.ToLower(str(m, "message") + " " + str(resp, "message"))
			if slot != "project" && !strings.Contains(msg, "project") {
				return fmt.Errorf("expected project interview, got %s", body)
			}
			return nil
		})

		c.step("chat thin research prompt asks topic", func() error {
			st, body, err := c.do(http.MethodPost, "/api/v1/chat/route", map[string]any{
				"message": "run the research agent",
			}, true)
			if err != nil {
				return err
			}
			if st != 200 {
				return fmt.Errorf("status %d: %s", st, body)
			}
			m := asMap(body)
			resp := asMap(anyToJSON(m["response"]))
			clarify := asMap(anyToJSON(resp["clarify"]))
			slot := str(clarify, "slot")
			msg := strings.ToLower(str(m, "message") + " " + str(resp, "message") + " " + str(clarify, "question"))
			if slot != "topic" && !strings.Contains(msg, "research") {
				return fmt.Errorf("expected topic interview, got %s", body)
			}
			if strings.Contains(msg, "has been sent") {
				return fmt.Errorf("chat claimed mail sent: %s", body)
			}
			return nil
		})

		c.step("chat sync gmail never claims sent", func() error {
			st, body, err := c.do(http.MethodPost, "/api/v1/chat/route", map[string]any{
				"message": "sync gmail and draft replies",
			}, true)
			if err != nil {
				return err
			}
			if st != 200 {
				return fmt.Errorf("status %d: %s", st, body)
			}
			blob := strings.ToLower(body)
			if strings.Contains(blob, "has been sent") || strings.Contains(blob, "i sent the") {
				return fmt.Errorf("chat claimed sent: %s", body)
			}
			return nil
		})
	}

	fmt.Printf("\n%d passed, %d failed\n", c.pass, c.fail)
	if c.fail > 0 {
		os.Exit(1)
	}
}

type checker struct {
	base        string
	client      *http.Client
	token       string
	agents      map[string]string
	project     string
	mailTask    string
	articleTask string
	articleRun  string
	drafts      []string
	pass, fail  int
}

func (c *checker) step(name string, fn func() error) {
	if c.agents == nil {
		c.agents = map[string]string{}
	}
	start := time.Now()
	err := fn()
	d := time.Since(start).Round(time.Millisecond)
	if err != nil {
		c.fail++
		fmt.Printf("FAIL  %s (%s): %v\n", name, d, err)
		return
	}
	c.pass++
	fmt.Printf("PASS  %s (%s)\n", name, d)
}

func (c *checker) createProject(name string) (string, error) {
	st, body, err := c.do(http.MethodPost, "/api/v1/projects/", map[string]any{
		"name": name, "description": "apicheck",
	}, true)
	if err != nil {
		return "", err
	}
	if st != 200 && st != 201 {
		return "", fmt.Errorf("status %d: %s", st, body)
	}
	id := str(asMap(body), "id")
	if id == "" {
		return "", fmt.Errorf("no project id: %s", body)
	}
	return id, nil
}

func (c *checker) launch(agentID, projectID string, values map[string]string) (int, string, error) {
	return c.do(http.MethodPost, "/api/v1/tasks/launch", map[string]any{
		"agent_id": agentID, "project_id": projectID, "values": values,
	}, true)
}

func (c *checker) findThread(thread, subject string) (map[string]any, error) {
	st, raw, err := c.do(http.MethodGet, "/api/v1/mail/threads?per_page=50", nil, true)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, fmt.Errorf("threads %d: %s", st, raw)
	}
	for _, row := range asSlice(asMap(raw)["data"]) {
		m := asMap(anyToJSON(row))
		if str(m, "gmail_thread_id") == thread || str(m, "subject") == subject {
			return m, nil
		}
	}
	return nil, nil
}

func (c *checker) injectAndSync(thread, from, subject, body, wantStatus string, wantDraft bool) error {
	st, raw, err := c.do(http.MethodPost, "/api/v1/mail/simulate/inbox", map[string]any{
		"messages": []map[string]any{{
			"thread_id": thread, "from": from, "subject": subject, "body": body,
		}},
	}, true)
	if err != nil {
		return err
	}
	if st != 202 {
		return fmt.Errorf("inject %d: %s", st, raw)
	}
	st, raw, err = c.do(http.MethodPost, "/api/v1/mail/simulate/sync", map[string]any{}, true)
	if err != nil {
		return err
	}
	if st != 200 {
		return fmt.Errorf("sync %d: %s", st, raw)
	}
	found, err := c.findThread(thread, subject)
	if err != nil {
		return err
	}
	if found == nil {
		return fmt.Errorf("thread %s not found", thread)
	}
	if str(found, "status") != wantStatus {
		return fmt.Errorf("thread status %q want %q (%s)", str(found, "status"), wantStatus, anyToJSON(found))
	}
	if wantDraft && str(found, "status") == "draft_ready" {
		// ok
	}
	return nil
}

func (c *checker) do(method, path string, payload any, auth bool) (int, string, error) {
	var rdr io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, "", err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b), nil
}

func agentIDByName(listJSON, name string) string {
	m := asMap(listJSON)
	for _, row := range asSlice(m["data"]) {
		item := asMap(anyToJSON(row))
		if str(item, "name") == name {
			return str(item, "id")
		}
	}
	return ""
}

func asMap(s string) map[string]any {
	var m map[string]any
	_ = json.Unmarshal([]byte(s), &m)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func str(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[k].(string); ok {
		return s
	}
	return ""
}

func truthy(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

func anyToJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func envOr(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}
