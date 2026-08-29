package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
)

func dummyPriceEmail() mail.InboxMessage {
	return mail.InboxMessage{
		GmailThreadID: "th-price",
		FromEmail:     "client@acme.com",
		FromName:      "Pat Client",
		ToEmail:       "org@example.com",
		Subject:       "What is the price of this machine?",
		Body:          "Hi — what is the price of this machine?\n\nhttps://vendor.example/machine-x\n\nThanks.",
	}
}

func dummyAvailabilityEmail() mail.InboxMessage {
	return mail.InboxMessage{
		GmailThreadID: "th-avail",
		FromEmail:     "buyer@shop.com",
		Subject:       "Do you have this machine available?",
		Body:          "Can you confirm availability?\nhttps://vendor.example/lathe-200",
	}
}

func TestEval_PriceEmailResearchesBodyURL(t *testing.T) {
	rs := &fakeResearch{}
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "question", NeedsResearch: false, SuggestedAction: "reply",
		Reason: "price question", TriageLabel: "sales", Urgency: "normal",
	}}
	gmail := &fakeGmail{
		email:    "org@example.com",
		tokens:   mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{dummyPriceEmail()},
	}
	svc, repo, orgID := setupMail(t, gmail, class, rs)
	connectOrg(t, svc, repo, orgID)
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if rs.calls != 1 {
		t.Fatalf("research calls %d, want 1", rs.calls)
	}
	found := false
	for _, u := range rs.lastReq.URLs {
		if u == "https://vendor.example/machine-x" {
			found = true
		}
	}
	if !found {
		t.Fatalf("body URL missing from research request: %+v", rs.lastReq.URLs)
	}
	listed, _ := repo.ListThreads(context.Background(), orgID, model.PaginationParams{})
	if len(listed.Data) != 1 || listed.Data[0].Status != model.MailThreadDraftReady {
		t.Fatalf("thread %+v", listed.Data)
	}
	drafts, _ := repo.ListDraftsByStatus(context.Background(), orgID, model.MailDraftDraft, model.PaginationParams{})
	if len(drafts.Data) != 1 {
		t.Fatalf("drafts %d", len(drafts.Data))
	}
	if strings.Contains(strings.ToLower(drafts.Data[0].Body), "has been sent") {
		t.Error("draft must not claim the email was sent")
	}
	if drafts.Data[0].ResearchBriefID == nil {
		t.Error("draft should carry the research brief")
	}
}

func TestEval_AvailabilityEmailResearchesBodyURL(t *testing.T) {
	rs := &fakeResearch{}
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "question", NeedsResearch: false, SuggestedAction: "reply",
		Reason: "stock", TriageLabel: "sales", Urgency: "normal",
	}}
	gmail := &fakeGmail{
		email:    "org@example.com",
		tokens:   mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{dummyAvailabilityEmail()},
	}
	svc, repo, orgID := setupMail(t, gmail, class, rs)
	connectOrg(t, svc, repo, orgID)
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if rs.calls != 1 {
		t.Fatalf("research calls %d", rs.calls)
	}
	if len(rs.lastReq.URLs) == 0 || rs.lastReq.URLs[0] != "https://vendor.example/lathe-200" {
		t.Fatalf("URLs %+v", rs.lastReq.URLs)
	}
}

func TestEval_PinnedPlaybookWithoutBodyURL(t *testing.T) {
	rs := &fakeResearch{}
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "request", NeedsResearch: false, SuggestedAction: "reply",
		Reason: "thanks", TriageLabel: "general", Urgency: "low",
	}}
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{{
			GmailThreadID: "th-pinned-eval", FromEmail: "alex@c.com",
			Subject: "Team plan price?", Body: "How much is the team plan?",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, class, rs)
	connectOrg(t, svc, repo, orgID)
	c, _ := repo.GetConnectionByOrg(context.Background(), orgID)
	c.WatchKnowledgeURLs = []string{"https://example.com/pricing"}
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if rs.calls != 1 {
		t.Fatalf("research calls %d", rs.calls)
	}
	if len(rs.lastReq.URLs) != 1 || rs.lastReq.URLs[0] != "https://example.com/pricing" {
		t.Fatalf("URLs %+v", rs.lastReq.URLs)
	}
}

func TestEval_NewsletterIgnoredNoResearchNoDraft(t *testing.T) {
	rs := &fakeResearch{}
	gmail := &fakeGmail{
		email:    "org@example.com",
		tokens:   mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{mail.InboxMessage{
			GmailThreadID: "th-news-eval",
			FromEmail:     "news@vendor.example",
			Subject:       "This week's digest",
			Body:          "View in browser. Click unsubscribe if you no longer want this newsletter.",
		}},
	}
	// Heuristic classifier — no scripted override.
	svc, repo, orgID := setupMail(t, gmail, nil, rs)
	connectOrg(t, svc, repo, orgID)
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if rs.calls != 0 {
		t.Fatalf("newsletter must not research, calls=%d", rs.calls)
	}
	listed, _ := repo.ListThreads(context.Background(), orgID, model.PaginationParams{})
	if len(listed.Data) != 1 || listed.Data[0].Status != model.MailThreadIgnored {
		t.Fatalf("thread %+v", listed.Data)
	}
	drafts, _ := repo.ListDraftsByStatus(context.Background(), orgID, model.MailDraftDraft, model.PaginationParams{})
	if len(drafts.Data) != 0 {
		t.Fatalf("ignore must not draft, got %d", len(drafts.Data))
	}
}

func TestEval_HeuristicPriceLinkNeedsResearch(t *testing.T) {
	got := mail.HeuristicClassify(dummyPriceEmail())
	if got.SuggestedAction == "ignore" {
		t.Fatal("price email must not be ignored")
	}
	if !got.NeedsResearch {
		t.Fatal("price + link should need research")
	}
}
