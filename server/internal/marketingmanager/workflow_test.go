package marketingmanager

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// The natural-language flow end to end, in test mode: find a template, make a
// draft campaign from it, validate, test-send to a confirmed tester, then send.
func TestCampaignWorkflowInTestMode(t *testing.T) {
	f := newFake(t)
	s := f.service("qa@example.com")
	ctx := context.Background()

	steps := []struct {
		kind Kind
		call Call
	}{
		{Read, Call{Action: "list_templates", Query: map[string]any{"search": "launch"}}},
		{Write, Call{Action: "create_campaign", Body: map[string]any{"name": "Workstation Shop launch", "channel": "email", "subject": "The shop is open"}}},
		{Write, Call{Action: "validate_campaign", ID: "c1", Body: map[string]any{"list_uuid": "l1"}}},
		{Send, Call{Action: "test_send", ID: "c1", Body: map[string]any{"email": "QA@example.com"}}},
		{Send, Call{Action: "schedule_campaign", ID: "c1", Body: map[string]any{"send_now": true}}},
	}
	for _, st := range steps {
		res, err := s.Run(ctx, org, st.kind, st.call)
		if err != nil {
			t.Fatalf("%s: %v", st.call.Action, err)
		}
		if st.kind == Send && res.SendMode != ModeDryRun {
			t.Fatalf("%s: send mode %q, want dry_run", st.call.Action, res.SendMode)
		}
		if st.kind != Send && res.SendMode != "" {
			t.Fatalf("%s reported a send mode", st.call.Action)
		}
	}
	for _, want := range []string{
		"GET " + marketingPrefix + "/templates",
		"POST " + marketingPrefix + "/campaigns",
		"POST " + marketingPrefix + "/campaigns/c1/validate",
		"POST " + marketingPrefix + "/campaigns/c1/test-send",
		"POST " + marketingPrefix + "/campaigns/c1/schedule",
	} {
		if f.called(want) != 1 {
			t.Errorf("%s called %d times", want, f.called(want))
		}
	}
	// Every call names the workspace and carries the key; the create is idempotent.
	for i, h := range f.headers {
		if h.Get("X-Namespace-Slug") != "workstation" || h.Get("Authorization") != "Bearer "+testKey {
			t.Fatalf("call %d (%s) headers %v", i, f.calls[i], h)
		}
		if f.calls[i] == "POST "+marketingPrefix+"/campaigns" && h.Get("Idempotency-Key") == "" {
			t.Fatal("create_campaign sent no Idempotency-Key")
		}
	}
}

func TestDescribeScheduleShowsEverythingBeforeASend(t *testing.T) {
	f := newFake(t)
	f.handlers["GET "+marketingPrefix+"/campaigns/c1"] = func(w http.ResponseWriter, _ *http.Request) {
		ok(w, map[string]any{"uuid": "c1", "name": "Shop launch", "status": "validated", "subject": "The shop is open",
			"from_email": "shop@workstation.example", "from_name": "Workstation Shop", "list_id": "7", "template_id": 3})
	}
	f.handlers["GET "+marketingPrefix+"/lists"] = func(w http.ResponseWriter, _ *http.Request) {
		ok(w, []map[string]any{{"id": 6, "uuid": "l6", "name": "Staff"}, {"id": 7, "uuid": "l7", "name": "Opted-in customers"}})
	}
	f.handlers["GET "+marketingPrefix+"/templates"] = func(w http.ResponseWriter, _ *http.Request) {
		ok(w, []map[string]any{{"id": 3, "uuid": "t3", "name": "Launch v2 (approved)"}})
	}
	f.handlers["POST "+marketingPrefix+"/campaigns/c1/audience-preview"] = func(w http.ResponseWriter, _ *http.Request) {
		ok(w, map[string]any{"audience_total": 540, "eligible": 500, "suppressed": 25, "no_consent": 15, "exact": true})
	}
	got := f.service().Describe(context.Background(), org, Call{Action: "schedule_campaign", ID: "c1",
		Body: map[string]any{"scheduled_at": "2026-11-02T09:00:00Z"}})
	for _, want := range []string{
		`"workstation"`, "Workstation Shop <shop@workstation.example>", `"The shop is open"`,
		`template "Launch v2 (approved)"`, `list "Opted-in customers"`, "500 eligible recipients of 540",
		"25 suppressed", "15 without consent", "at 2026-11-02T09:00:00Z", "sends automatically", "TEST MODE",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("confirmation lacks %q:\n%s", want, got)
		}
	}
	for _, c := range f.calls {
		if strings.HasSuffix(c, "/schedule") {
			t.Fatal("describing a send performed it")
		}
	}
}

func TestDescribeScheduleFallsBackAndFlagsGaps(t *testing.T) {
	f := newFake(t)
	f.dryRun = false
	f.handlers["GET "+marketingPrefix+"/campaigns/c1"] = func(w http.ResponseWriter, _ *http.Request) {
		ok(w, map[string]any{"uuid": "c1", "name": "No list yet", "status": "draft", "list_id": nil})
	}
	got := f.service().Describe(context.Background(), org, Call{Action: "schedule_campaign", ID: "c1"})
	for _, want := range []string{"news@example.com> (workspace default)", "Subject: not set", "no audience attached", "Schedule: now", "LIVE workspace", "refused"} {
		if !strings.Contains(got, want) {
			t.Errorf("confirmation lacks %q:\n%s", want, got)
		}
	}
}

func TestTemplateOperations(t *testing.T) {
	f := newFake(t)
	s := f.service()
	ctx := context.Background()
	if _, err := s.Run(ctx, org, Write, Call{Action: "update_template", ID: "t1", Body: map[string]any{"subject": "New"}}); err != nil {
		t.Fatal(err)
	}
	if f.called("PUT "+marketingPrefix+"/templates/t1/studio") != 1 {
		t.Fatalf("edit did not go through the studio (sanitise + analyse): %v", f.calls)
	}
	for _, a := range []string{"preview_template", "analyse_template"} {
		if _, err := s.Run(ctx, org, Read, Call{Action: a, Body: map[string]any{"body_html": "<p>Hi {{first_name}}</p>"}}); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
	if _, err := s.Run(ctx, org, Write, Call{Action: "clone_template", ID: "t1", Body: map[string]any{"name": "Copy"}}); err != nil {
		t.Fatal(err)
	}
	// Missing or hostile ids never reach OpsAPI.
	before := len(f.calls)
	for _, id := range []string{"", "../../crm/contacts", "t1/../x", "a b"} {
		if _, err := s.Run(ctx, org, Read, Call{Action: "get_template", ID: id}); err == nil {
			t.Errorf("id %q accepted", id)
		}
	}
	if len(f.calls) != before {
		t.Fatalf("bad ids reached OpsAPI: %v", f.calls[before:])
	}
}

func TestOpsAPIErrorsAreExplained(t *testing.T) {
	f := newFake(t)
	f.handlers["GET "+marketingPrefix+"/templates"] = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"error":"Marketing Cloud is turned off for this workspace","details":{"code":"PLUGIN_DISABLED"}}`))
	}
	f.handlers["POST "+marketingPrefix+"/campaigns"] = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"success":false,"error":"Validation failed","details":{"from_email":"must be an email"}}`))
	}
	s := f.service()
	_, err := s.Run(context.Background(), org, Read, Call{Action: "list_templates"})
	if err == nil || !strings.Contains(err.Error(), "plugin is turned off") || !IsCode(err, codePluginDisabled) {
		t.Fatalf("plugin-disabled error: %v", err)
	}
	_, err = s.Run(context.Background(), org, Write, Call{Action: "create_campaign", Body: map[string]any{"name": "x", "from_email": "nope"}})
	if err == nil || !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "from_email") {
		t.Fatalf("validation error should name the field: %v", err)
	}
}

// Existing orgs get the agent from migration 072; it must say what Seed says.
func TestSeedMigrationMatchesSeed(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/000072_marketing_manager_agent.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	a := Seed(uuid.New())
	for _, want := range []string{a.Name, *a.Description, strings.ReplaceAll(*a.SystemPrompt, "'", "''"), `"builtin":"` + model.BuiltinMarketingManager + `"`, "WHERE NOT EXISTS"} {
		if !strings.Contains(sql, want) {
			t.Errorf("migration lacks %q", want)
		}
	}
}
