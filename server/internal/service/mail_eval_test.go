package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
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

func TestEval_DraftUsesResearchBriefAndNeverClaimsSent(t *testing.T) {
	rs := &fakeResearch{brief: &research.Brief{
		Topic:   "price",
		Summary: "The lathe-200 list price is 18400 GBP.",
		Findings: []research.Finding{{
			Claim: "list price is 18400 GBP", SourceURL: "https://vendor.example/lathe-200",
		}},
		Sources: []research.Source{{URL: "https://vendor.example/lathe-200", Title: "lathe"}},
	}}
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "question", NeedsResearch: true, SuggestedAction: "reply",
		Reason: "price", TriageLabel: "sales", Urgency: "normal",
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
	drafts, _ := repo.ListDraftsByStatus(context.Background(), orgID, model.MailDraftDraft, model.PaginationParams{})
	if len(drafts.Data) != 1 {
		t.Fatalf("drafts %d", len(drafts.Data))
	}
	body := strings.ToLower(drafts.Data[0].Body)
	if strings.Contains(body, "has been sent") || strings.Contains(body, "email was sent") {
		t.Error("draft must not claim the email was sent")
	}
	if !strings.Contains(drafts.Data[0].Body, "18400") {
		t.Fatalf("draft should use the research brief, got %q", drafts.Data[0].Body)
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

func TestEval_BoardUpdateTouchesOnlyBoundLaunchTask(t *testing.T) {
	rs := &fakeResearch{}
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "question", NeedsResearch: false, SuggestedAction: "reply",
		Reason: "price", TriageLabel: "sales", Urgency: "normal",
	}}
	gmail := &fakeGmail{
		email:    "org@example.com",
		tokens:   mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{dummyPriceEmail()},
	}
	svc, repo, orgID := setupMail(t, gmail, class, rs)
	connectOrg(t, svc, repo, orgID)

	bound := uuid.New()
	other := uuid.New()
	board := &stubMailTasks{tasks: map[uuid.UUID]*model.Task{
		bound: {ID: bound, Status: "in_progress", Metadata: map[string]any{model.TaskMetaLaunchKind: model.BuiltinMail}},
		other: {ID: other, Status: "in_progress", Metadata: map[string]any{model.TaskMetaLaunchKind: model.BuiltinMail}},
	}}
	svc.BindTasks(board)
	svc.BindLaunchTask(orgID, bound)

	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if board.tasks[bound].Status != "review" {
		t.Fatalf("bound task status %q, want review", board.tasks[bound].Status)
	}
	if board.tasks[other].Status != "in_progress" {
		t.Fatalf("unrelated mail task was mutated: %q", board.tasks[other].Status)
	}
	if board.tasks[bound].Description == nil || !strings.Contains(*board.tasks[bound].Description, "Draft ready") {
		t.Fatalf("bound task description = %v", board.tasks[bound].Description)
	}
}

func TestEval_BoundEmptySyncMarksOnlyThatTaskDone(t *testing.T) {
	gmail := &fakeGmail{
		email:    "org@example.com",
		tokens:   mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{mail.InboxMessage{
			GmailThreadID: "th-news-eval-bound",
			FromEmail:     "news@vendor.example",
			Subject:       "This week's digest",
			Body:          "View in browser. Click unsubscribe if you no longer want this newsletter.",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, nil, &fakeResearch{})
	connectOrg(t, svc, repo, orgID)
	bound := uuid.New()
	other := uuid.New()
	board := &stubMailTasks{tasks: map[uuid.UUID]*model.Task{
		bound: {ID: bound, Status: "in_progress", Metadata: map[string]any{model.TaskMetaLaunchKind: model.BuiltinMail}},
		other: {ID: other, Status: "in_progress", Metadata: map[string]any{model.TaskMetaLaunchKind: model.BuiltinMail}},
	}}
	svc.BindTasks(board)
	svc.BindLaunchTask(orgID, bound)
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if board.tasks[bound].Status != "done" {
		t.Fatalf("bound empty sync should finish that card, got %q", board.tasks[bound].Status)
	}
	if board.tasks[other].Status != "in_progress" {
		t.Fatalf("sibling mail card was closed: %q", board.tasks[other].Status)
	}
}

func TestEval_UnboundPeriodicSyncDoesNotCloseMailTasks(t *testing.T) {
	gmail := &fakeGmail{
		email:    "org@example.com",
		tokens:   mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{mail.InboxMessage{
			GmailThreadID: "th-news-eval-2",
			FromEmail:     "news@vendor.example",
			Subject:       "This week's digest",
			Body:          "View in browser. Click unsubscribe if you no longer want this newsletter.",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, nil, &fakeResearch{})
	connectOrg(t, svc, repo, orgID)
	orphan := uuid.New()
	board := &stubMailTasks{tasks: map[uuid.UUID]*model.Task{
		orphan: {ID: orphan, Status: "in_progress", Metadata: map[string]any{model.TaskMetaLaunchKind: model.BuiltinMail}},
	}}
	svc.BindTasks(board)
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if board.tasks[orphan].Status != "in_progress" {
		t.Fatalf("periodic/unbound sync closed a mail task: %q", board.tasks[orphan].Status)
	}
}

type stubMailTasks struct {
	tasks map[uuid.UUID]*model.Task
}

func (s *stubMailTasks) Create(context.Context, uuid.UUID, model.CreateTaskRequest) (*model.Task, error) {
	return nil, nil
}
func (s *stubMailTasks) GetByID(_ context.Context, id uuid.UUID) (*model.Task, error) {
	return s.tasks[id], nil
}
func (s *stubMailTasks) List(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	return nil, nil
}
func (s *stubMailTasks) ListByOrg(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	return nil, nil
}
func (s *stubMailTasks) ListComments(context.Context, uuid.UUID) ([]model.TaskComment, error) {
	return nil, nil
}
func (s *stubMailTasks) AddComment(context.Context, uuid.UUID, uuid.UUID, string) (*model.TaskComment, error) {
	return nil, nil
}
func (s *stubMailTasks) Update(_ context.Context, id uuid.UUID, req model.UpdateTaskRequest) (*model.Task, error) {
	t := s.tasks[id]
	if t == nil {
		return nil, nil
	}
	if req.Description != nil {
		t.Description = req.Description
	}
	return t, nil
}
func (s *stubMailTasks) Delete(context.Context, uuid.UUID) error { return nil }
func (s *stubMailTasks) Transition(_ context.Context, id uuid.UUID, status string, _ *uuid.UUID) error {
	if t := s.tasks[id]; t != nil {
		t.Status = status
	}
	return nil
}
func (s *stubMailTasks) Reorder(context.Context, uuid.UUID, string, int, *uuid.UUID) error { return nil }
func (s *stubMailTasks) History(context.Context, uuid.UUID) (*model.TaskHistory, error) {
	return nil, nil
}
func (s *stubMailTasks) FindByLaunchRunID(context.Context, uuid.UUID) (*model.Task, error) {
	return nil, nil
}
