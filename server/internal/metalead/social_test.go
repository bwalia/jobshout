package metalead

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
)

func entries(t *testing.T, body string) webhookPayload {
	t.Helper()
	var p webhookPayload
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func keys(its []interaction) string {
	var out []string
	for _, it := range its {
		out = append(out, it.Source+"="+it.Key)
	}
	return strings.Join(out, " ")
}

// Real payload shapes from Meta's webhook docs; the business's own sends and
// anything but new comments are left out.
func TestInteractionsFrom(t *testing.T) {
	ig := entries(t, `{"object":"instagram","entry":[{"id":"ig1","messaging":[
		{"sender":{"id":"u1"},"recipient":{"id":"ig1"},"message":{"mid":"m1","text":"price?"}},
		{"sender":{"id":"ig1"},"recipient":{"id":"u1"},"message":{"mid":"m2","text":"£40","is_echo":true}},
		{"sender":{"id":"u2"},"recipient":{"id":"ig1"},"read":{"mid":"m1"}},
		{"sender":{"id":"u3"},"recipient":{"id":"ig1"},"message":{"mid":"m3","attachments":[{"type":"image"}]}}
	],"changes":[
		{"field":"comments","value":{"id":"c1","from":{"id":"u4","username":"ada"},"text":"do you ship to UK?","media":{"id":"md1"}}},
		{"field":"comments","value":{"comment_id":"c2","from":{"id":"u5","username":"bob"},"text":"nice","media":{"id":"md1"}}},
		{"field":"comments","value":{"id":"c3","from":{"id":"ig1","username":"us"},"text":"thanks!"}},
		{"field":"mentions","value":{"comment_id":"c4","media_id":"md9"}},
		{"field":"story_insights","value":{"media_id":"s1","reach":4}}
	]}]}`)
	got := keys(interactionsFrom(ig.Object, ig.Entry[0]))
	want := "instagram_dm=dm:ig1:u1 instagram_comment=c1 instagram_comment=c2 instagram_mention=c4"
	if got != want {
		t.Fatalf("instagram:\n got %s\nwant %s", got, want)
	}

	fb := entries(t, `{"object":"page","entry":[{"id":"p1","messaging":[
		{"sender":{"id":"psid1"},"recipient":{"id":"p1"},"message":{"mid":"m1","text":"are you open sunday"}}
	],"changes":[
		{"field":"feed","value":{"item":"comment","verb":"add","comment_id":"p1_9_1","post_id":"p1_9","from":{"id":"x","name":"Cy"},"message":"how much?","created_time":1520544816}},
		{"field":"feed","value":{"item":"comment","verb":"edited","comment_id":"p1_9_2","post_id":"p1_9","from":{"id":"x"},"message":"edit"}},
		{"field":"feed","value":{"item":"reaction","verb":"add","post_id":"p1_9","reaction_type":"like"}},
		{"field":"feed","value":{"item":"comment","verb":"add","comment_id":"p1_9_3","from":{"id":"p1","name":"Us"},"message":"DM us"}},
		{"field":"mention","value":{"item":"post","verb":"add","post_id":"z_7"}},
		{"field":"leadgen","value":{"leadgen_id":"lg1","page_id":"p1"}}
	]}]}`)
	its := interactionsFrom(fb.Object, fb.Entry[0])
	got = keys(its)
	want = "facebook_dm=dm:p1:psid1 facebook_comment=p1_9_1 facebook_mention=z_7"
	if got != want {
		t.Fatalf("facebook:\n got %s\nwant %s", got, want)
	}
	if its[1].AuthorName != "Cy" || its[1].Permalink != "https://www.facebook.com/p1_9" {
		t.Fatalf("comment = %+v", its[1])
	}
}

func TestContactFields(t *testing.T) {
	got := contactFields("hi, £1500 budget, order #4471 — reach me on 07700 900123 or Ada.L@example.co.uk.", "ada_l")
	want := []FieldData{
		{Name: "email", Values: []string{"Ada.L@example.co.uk"}},
		{Name: "phone", Values: []string{"07700 900123"}},
		{Name: "full_name", Values: []string{"ada_l"}},
	}
	if g, w := FieldDataJSON(got), FieldDataJSON(want); string(g) != string(w) {
		t.Fatalf("got %s\nwant %s", g, w)
	}
	if len(contactFields("is 1500 ok for 3 days?", "")) != 0 {
		t.Fatal("a price is not a phone")
	}
}

func TestGroundEvidence(t *testing.T) {
	b := &model.MetaLeadBrief{Evidence: `"How much  for a WEBSITE"`, Flags: []string{}}
	groundEvidence(b, "hey\nhow much for a website? thanks")
	if b.Evidence != "How much  for a WEBSITE" || len(b.Flags) != 0 {
		t.Fatalf("a real quote must stay: %+v", b)
	}
	b = &model.MetaLeadBrief{Evidence: "I want to buy 10 units", Flags: []string{}}
	groundEvidence(b, "hey, nice post")
	if b.Evidence != "" || len(b.Flags) != 1 {
		t.Fatalf("a made-up quote must go: %+v", b)
	}
}

func TestDecodeBriefLooseShapes(t *testing.T) {
	b, err := decodeBrief(`{"is_lead":"yes","category":"Pricing","fit_score":"72","flags":"needs postcode","spam_risk":"LOW"}`)
	if err != nil {
		t.Fatal(err)
	}
	if b.IsLead == nil || !*b.IsLead || b.Category != "pricing" || b.FitScore != 72 || len(b.Flags) != 1 || b.SpamRisk != "low" {
		t.Fatalf("brief = %+v", b)
	}
	if b, _ := decodeBrief(`{"intent":"x"}`); b.IsLead != nil {
		t.Fatal("a form brief has no is_lead")
	}
}

// One Instagram DM thread: a greeting (not a lead), Meta retrying it, then a
// price question with an email — appended, re-qualified and pushed to CRM once.
func TestWebhookDMThread(t *testing.T) {
	tokenKey := "test-meta-token-key-32bytes!!!!"
	key, _ := mail.KeyFromSecret(tokenKey)
	enc, _ := mail.Encrypt(key, []byte("page-token"))
	store := newMemStore()
	org := uuid.New()
	conn := &connRow{
		MetaLeadConnection: model.MetaLeadConnection{ID: uuid.New(), OrgID: org, Name: "ig", PageID: "page1", IGAccountID: "ig1"},
		AccessTokenEnc:     enc,
	}
	store.byPage["ig1"] = conn

	llm := &countingLLM{responses: []string{
		`{"is_lead":false,"category":"other","fit_score":5,"evidence":"","summary":"Greeting"}`,
		`{"is_lead":"true","category":"pricing","fit_score":"70","evidence":"\"How much for a website?\"","summary":"Wants a website price"}`,
	}}
	crm := &stubCRM{}
	svc := NewService(store, fakeGraph{objects: map[string]string{"u1": `{"name":"Ada L","username":"ada_l"}`}},
		&Analyser{LLM: llm}, Config{TokenKey: tokenKey}, nil)
	svc.BindCRM(crm)

	dm := func(mid, text string) webhookPayload {
		return entries(t, `{"object":"instagram","entry":[{"id":"ig1","messaging":[{"sender":{"id":"u1"},"recipient":{"id":"ig1"},"message":{"mid":"`+mid+`","text":"`+text+`"}}]}]}`)
	}
	svc.processWebhook(dm("m1", "hi there"))
	svc.processWebhook(dm("m1", "hi there")) // Meta's retry
	lead := store.leads["dm:ig1:u1"]
	if llm.i != 1 || lead.AuthorName != "ada_l" || lead.CRMError != "skipped: not a lead" {
		t.Fatalf("after greeting: llm calls=%d lead=%+v", llm.i, lead)
	}

	svc.processWebhook(dm("m2", "how much for a website? ada@example.com"))
	lead = store.leads["dm:ig1:u1"]
	var brief model.MetaLeadBrief
	_ = json.Unmarshal(lead.Analysis, &brief)
	if lead.Body != "hi there\nhow much for a website? ada@example.com" || brief.Evidence != "How much for a website?" {
		t.Fatalf("thread = %q brief = %+v", lead.Body, brief)
	}
	if len(crm.calls) != 1 || crm.calls[0].Email != "ada@example.com" || crm.calls[0].FirstName != "ada_l" || lead.CRMContactID != "ct-1" {
		t.Fatalf("crm calls = %+v lead = %+v", crm.calls, lead)
	}
}
