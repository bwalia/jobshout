package creditcontroller

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jobshout/server/internal/agentmodule"
)

func TestLoadConfigHasNoLocalhostDefault(t *testing.T) {
	t.Setenv("AIVC_BASE_URL", "")
	if got := LoadConfig().BaseURL; got != "" {
		t.Fatalf("BaseURL = %q, want empty so deployed envs run in demo mode", got)
	}
}

func TestDemoModeServesFixturesTheTabParses(t *testing.T) {
	c := NewClient(Config{Timeout: time.Second}, nil)
	ctx := context.Background()

	if c.LiveConfigured() || c.Mode() != "demo" {
		t.Fatalf("empty base URL must be demo mode, got %s", c.Mode())
	}
	st := c.Status(ctx)
	if st["ok"] != true || st["mode"] != "demo" {
		t.Fatalf("status = %v, want ok demo", st)
	}

	list, err := c.ListInvoices(ctx)
	if err != nil {
		t.Fatalf("ListInvoices: %v", err)
	}
	// Round-trip through JSON: that is what the web client receives.
	var wire struct {
		Count    int     `json:"count"`
		TotalGBP float64 `json:"total_gbp"`
		Invoices []struct {
			InvoiceID string   `json:"invoice_id"`
			Supplier  string   `json:"supplier_name"`
			Amount    *float64 `json:"amount_gbp"`
			Scenario  string   `json:"scenario_hint"`
			Workflow  *struct {
				RunID  string `json:"run_id"`
				Status string `json:"status"`
			} `json:"workflow"`
		} `json:"invoices"`
	}
	roundTrip(t, list, &wire)
	if wire.Count == 0 || wire.Count != len(wire.Invoices) || wire.TotalGBP <= 0 {
		t.Fatalf("invoice list = %+v", wire)
	}
	untriaged := 0
	for _, inv := range wire.Invoices {
		if inv.InvoiceID == "" || inv.Supplier == "" || inv.Amount == nil || inv.Scenario == "" {
			t.Fatalf("incomplete invoice row %+v", inv)
		}
		if inv.Workflow == nil {
			untriaged++
		}
	}
	if got := untriagedIDs(list); len(got) != untriaged || untriaged == 0 {
		t.Fatalf("untriagedIDs = %v, want %d ids", got, untriaged)
	}

	var sum struct {
		Period  string `json:"period"`
		Mailbox struct {
			Total     int     `json:"total_invoices"`
			Untriaged int     `json:"untriaged"`
			TotalGBP  float64 `json:"total_gbp"`
		} `json:"mailbox"`
		Queue struct {
			Awaiting int `json:"awaiting_approval"`
			Items    []struct {
				RunID string `json:"run_id"`
			} `json:"items"`
		} `json:"queue"`
		Playbook []struct {
			Step int    `json:"step"`
			Task string `json:"task"`
		} `json:"playbook"`
	}
	raw, err := c.CreditControllerSummary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	roundTrip(t, raw, &sum)
	if sum.Mailbox.Total != wire.Count || sum.Mailbox.Untriaged != untriaged || sum.Mailbox.TotalGBP != wire.TotalGBP {
		t.Fatalf("summary mailbox %+v disagrees with list (count %d, untriaged %d, total %v)", sum.Mailbox, wire.Count, untriaged, wire.TotalGBP)
	}
	if sum.Queue.Awaiting == 0 || sum.Queue.Awaiting != len(sum.Queue.Items) || len(sum.Playbook) == 0 || !strings.HasSuffix(sum.Period, "(demo)") {
		t.Fatalf("summary = %+v", sum)
	}

	q, err := c.Queue(ctx)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if items, _ := q["awaiting_approval"].([]map[string]any); len(items) != sum.Queue.Awaiting {
		t.Fatalf("queue has %d items, summary says %d", len(items), sum.Queue.Awaiting)
	}

	first := wire.Invoices[0].InvoiceID
	inv, err := c.GetInvoice(ctx, first)
	if err != nil {
		t.Fatalf("GetInvoice(%s): %v", first, err)
	}
	if doc, _ := inv["document_text"].(string); !strings.Contains(doc, first) {
		t.Fatalf("document_text does not mention %s: %q", first, doc)
	}
	_, err = c.GetInvoice(ctx, "INV-9999")
	assertStatus(t, err, http.StatusNotFound)
}

func TestDemoModeRefusesWritesWithoutAServerError(t *testing.T) {
	c := NewClient(Config{}, nil)
	ctx := context.Background()
	_, err := c.GenerateInvoices(ctx, "weekly", 10)
	assertStatus(t, err, http.StatusConflict)
	_, err = c.Triage(ctx, "INV-1005")
	assertStatus(t, err, http.StatusConflict)
	_, err = c.TriageBatch(ctx, []string{"INV-1005"})
	assertStatus(t, err, http.StatusConflict)
	_, err = c.Approve(ctx, "run-7a1c11", true, "", "")
	assertStatus(t, err, http.StatusConflict)
	if !strings.Contains(err.Error(), "AIVC_BASE_URL") {
		t.Fatalf("refusal should say how to enable it: %q", err)
	}
}

func TestUnreachableRuntimeIsReadable(t *testing.T) {
	// Grab a free port, then close it so the dial is refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	ln.Close()

	c := NewClient(Config{BaseURL: base, Timeout: 2 * time.Second}, nil)
	_, err = c.ListInvoices(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if strings.Contains(msg, "dial tcp") || !strings.Contains(msg, "unreachable at "+base) || !strings.Contains(msg, "connection refused") {
		t.Fatalf("unreadable error: %q", msg)
	}
	assertStatus(t, err, 0)

	st := c.Status(context.Background())
	if st["ok"] != false || st["mode"] != "live" || st["message"] != msg {
		t.Fatalf("status = %v", st)
	}
	issues := Module(c).Ready(context.Background(), [16]byte{})
	if len(issues) != 1 || issues[0].Code != "aivc_unreachable" || issues[0].Message != msg {
		t.Fatalf("Ready = %+v", issues)
	}
}

func TestLiveUpstreamErrorsAreSummarised(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/ap/invoices/INV-404":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"invoice INV-404 not found"}`))
		case "/v1/ap/approve":
			if r.Header.Get("X-User") != "s.oyelaran" {
				t.Errorf("approve X-User = %q, want controller identity", r.Header.Get("X-User"))
			}
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"detail":"segregation of duties"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, User: "ap.clerk", Roles: "finance", Timeout: 2 * time.Second}, nil)
	_, err := c.GetInvoice(context.Background(), "INV-404")
	assertStatus(t, err, http.StatusNotFound)
	if !strings.Contains(err.Error(), "invoice INV-404 not found") {
		t.Fatalf("detail not surfaced: %q", err)
	}
	// 403 must not pass through: the web client would treat it as a dead session.
	_, err = c.Approve(context.Background(), "run-1", true, "", "")
	assertStatus(t, err, 0)
}

func TestDemoLaunchFinishesWithExplanation(t *testing.T) {
	launch := Module(NewClient(Config{}, nil)).Launch
	ctx := context.Background()

	out, err := launch(ctx, agentmodule.LaunchInput{Values: map[string]string{"action": "summary"}})
	if err != nil || out.Status != "done" || out.ExtraMeta["summary"] == nil {
		t.Fatalf("summary launch = %+v, %v", out, err)
	}
	for _, action := range []string{"generate_weekly", "triage_untriaged", "month_end"} {
		out, err := launch(ctx, agentmodule.LaunchInput{Values: map[string]string{"action": action}})
		if err != nil {
			t.Fatalf("%s: demo launch must not error (task would stick in progress): %v", action, err)
		}
		if out.Status != "done" || !strings.Contains(out.Message, "aivc-agents") || out.ExtraMeta["write_live"] != false {
			t.Fatalf("%s launch = %+v", action, out)
		}
	}
	if issues := Module(NewClient(Config{}, nil)).Ready(ctx, [16]byte{}); len(issues) != 1 || issues[0].Severity != "info" {
		t.Fatalf("demo Ready = %+v", issues)
	}
}

func roundTrip(t *testing.T, in any, out any) {
	t.Helper()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
}

func assertStatus(t *testing.T, err error, want int) {
	t.Helper()
	var ccErr *Error
	if !errors.As(err, &ccErr) {
		t.Fatalf("error %v is not *creditcontroller.Error", err)
	}
	if ccErr.Status != want {
		t.Fatalf("status = %d, want %d (%s)", ccErr.Status, want, ccErr.Msg)
	}
}
