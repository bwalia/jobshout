package metalead

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
)

func TestVerifyWebhookChallenge(t *testing.T) {
	s := &Service{cfg: Config{WebhookVerify: "secret"}}
	got, ok := s.VerifyWebhookChallenge("subscribe", "secret", "challenge-1")
	if !ok || got != "challenge-1" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	if _, ok := s.VerifyWebhookChallenge("subscribe", "wrong", "x"); ok {
		t.Fatal("expected reject")
	}
}

func TestValidMetaSignature(t *testing.T) {
	body := []byte(`{"object":"page"}`)
	mac := hmac.New(sha256.New, []byte("app-secret"))
	_, _ = mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !validMetaSignature("app-secret", body, sig) {
		t.Fatal("expected valid")
	}
	if validMetaSignature("app-secret", body, "sha256=deadbeef") {
		t.Fatal("expected invalid")
	}
}

type memStore struct {
	mu    sync.Mutex
	conns map[string]*connRow // by name
	byPage map[string]*connRow
	runs  map[uuid.UUID]*model.MetaLeadRun
	leads map[string]*model.MetaLead
}

func newMemStore() *memStore {
	return &memStore{
		conns:  map[string]*connRow{},
		byPage: map[string]*connRow{},
		runs:   map[uuid.UUID]*model.MetaLeadRun{},
		leads:  map[string]*model.MetaLead{},
	}
}

func (m *memStore) UpsertConnection(_ context.Context, orgID uuid.UUID, name, pageID, pageName string, tokenEnc []byte, connectedBy *uuid.UUID) (*model.MetaLeadConnection, error) {
	c := &connRow{
		MetaLeadConnection: model.MetaLeadConnection{
			ID: uuid.New(), OrgID: orgID, Name: name, PageID: pageID, PageName: pageName,
			ConnectedBy: connectedBy, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
		AccessTokenEnc: tokenEnc,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conns[name] = c
	m.byPage[pageID] = c
	out := c.MetaLeadConnection
	return &out, nil
}

func (m *memStore) GetConnectionByName(_ context.Context, _ uuid.UUID, name string) (*connRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conns[name]
	if !ok {
		return nil, errNotFound(name)
	}
	cp := *c
	return &cp, nil
}

func (m *memStore) GetConnectionByPageID(_ context.Context, pageID string) (*connRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.byPage[pageID]
	if !ok {
		return nil, errNotFound(pageID)
	}
	cp := *c
	return &cp, nil
}

func errNotFound(s string) error { return &notFound{s} }

type notFound struct{ s string }

func (e *notFound) Error() string { return "not found: " + e.s }

func (m *memStore) ListConnections(context.Context, uuid.UUID) ([]model.MetaLeadConnection, error) {
	return nil, nil
}
func (m *memStore) CreateRun(_ context.Context, run *model.MetaLeadRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	cp := *run
	m.runs[run.ID] = &cp
	return nil
}
func (m *memStore) UpdateRun(_ context.Context, run *model.MetaLeadRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *run
	m.runs[run.ID] = &cp
	return nil
}
func (m *memStore) GetRun(_ context.Context, _, runID uuid.UUID) (*model.MetaLeadRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[runID]
	if !ok {
		return nil, errNotFound(runID.String())
	}
	cp := *r
	return &cp, nil
}
func (m *memStore) ListRuns(context.Context, uuid.UUID, int) ([]model.MetaLeadRun, error) {
	return nil, nil
}
func (m *memStore) UpsertLead(_ context.Context, lead *model.MetaLead) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if lead.ID == uuid.Nil {
		lead.ID = uuid.New()
	}
	cp := *lead
	m.leads[lead.LeadgenID] = &cp
	return nil
}
func (m *memStore) UpdateLeadAnalysis(_ context.Context, _, leadID uuid.UUID, analysis json.RawMessage, status, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, L := range m.leads {
		if L.ID == leadID {
			L.Analysis = analysis
			L.Status = status
			L.ErrorMessage = errMsg
		}
	}
	return nil
}
func (m *memStore) ListLeadsByRun(context.Context, uuid.UUID, uuid.UUID) ([]model.MetaLead, error) {
	return nil, nil
}
func (m *memStore) ListRecentLeads(context.Context, uuid.UUID, int) ([]model.MetaLead, error) {
	return nil, nil
}
func (m *memStore) GetLeadByLeadgenID(_ context.Context, _ uuid.UUID, id string) (*model.MetaLead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	L, ok := m.leads[id]
	if !ok {
		return nil, errNotFound(id)
	}
	cp := *L
	return &cp, nil
}

type fakeGraph struct{}

func (fakeGraph) ListForms(context.Context, string, string) ([]FormSummary, error) {
	return []FormSummary{{ID: "f1", Name: "Demo"}}, nil
}
func (fakeGraph) ListLeads(context.Context, string, string, *time.Time, int) ([]LeadSummary, error) {
	return []LeadSummary{{
		ID: "lg1", CreatedTime: "2026-10-01T12:00:00Z",
		FieldData: []FieldData{{Name: "email", Values: []string{"a@b.com"}}},
	}}, nil
}
func (fakeGraph) GetLead(context.Context, string, string) (*LeadDetail, error) {
	return &LeadDetail{
		ID: "lg1", FormID: "f1", CreatedTime: "2026-10-01T12:00:00Z",
		FieldData: []FieldData{{Name: "email", Values: []string{"a@b.com"}}},
	}, nil
}

func TestLaunchHappyPath(t *testing.T) {
	tokenKey := "test-meta-token-key-32bytes!!!!"
	key, err := mail.KeyFromSecret(tokenKey)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := mail.Encrypt(key, []byte("page-token"))
	if err != nil {
		t.Fatal(err)
	}
	store := newMemStore()
	org := uuid.New()
	conn := &connRow{
		MetaLeadConnection: model.MetaLeadConnection{
			ID: uuid.New(), OrgID: org, Name: "workstation-ig", PageID: "page1",
		},
		AccessTokenEnc: enc,
	}
	store.conns["workstation-ig"] = conn
	store.byPage["page1"] = conn

	svc := NewService(store, fakeGraph{}, nil, Config{TokenKey: tokenKey, MaxLeadsPerLaunch: 50}, nil)
	res, err := svc.Launch(context.Background(), LaunchRequest{
		OrgID: org, UserID: uuid.New(), Connection: "workstation-ig", Limit: 5, Focus: "UK SaaS",
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := store.GetRun(context.Background(), org, res.RunID)
		if err == nil && run.Status == model.MetaLeadRunStatusDone && run.LeadCount == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	run, _ := store.GetRun(context.Background(), org, res.RunID)
	t.Fatalf("run did not complete: %+v leads=%d", run, len(store.leads))
}

func TestHandleWebhookLeadgen(t *testing.T) {
	tokenKey := "test-meta-token-key-32bytes!!!!"
	key, err := mail.KeyFromSecret(tokenKey)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := mail.Encrypt(key, []byte("page-token"))
	if err != nil {
		t.Fatal(err)
	}
	store := newMemStore()
	org := uuid.New()
	conn := &connRow{
		MetaLeadConnection: model.MetaLeadConnection{
			ID: uuid.New(), OrgID: org, Name: "ig", PageID: "page99",
		},
		AccessTokenEnc: enc,
	}
	store.byPage["page99"] = conn

	svc := NewService(store, fakeGraph{}, nil, Config{
		AppSecret: "app-secret", TokenKey: tokenKey,
	}, nil)

	body := []byte(`{"entry":[{"id":"page99","changes":[{"field":"leadgen","value":{"leadgen_id":"lg1","page_id":"page99","form_id":"f1"}}]}]}`)
	mac := hmac.New(sha256.New, []byte("app-secret"))
	_, _ = mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if err := svc.HandleWebhook(context.Background(), body, sig); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.leads["lg1"]; !ok {
		t.Fatal("expected lead stored")
	}
}
