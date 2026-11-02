package metalead

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
)

func TestContactFromFields(t *testing.T) {
	c := ContactFromFields([]FieldData{
		{Name: "email", Values: []string{"Ada@Example.com"}},
		{Name: "full_name", Values: []string{"Ada Lovelace"}},
		{Name: "phone_number", Values: []string{"+441234"}},
		{Name: "company_name", Values: []string{"Analytical Engines"}},
	}, &model.MetaLeadBrief{FitScore: 88}, "lg1")
	if c.Email != "Ada@Example.com" || c.FirstName != "Ada" || c.LastName != "Lovelace" || c.Phone != "+441234" {
		t.Fatalf("contact = %+v", c)
	}
	if c.JobTitle == "" || c.LeadgenID != "lg1" {
		t.Fatalf("job/leadgen = %+v", c)
	}
}

func TestContactFromFieldsEmailOnly(t *testing.T) {
	c := ContactFromFields([]FieldData{
		{Name: "email", Values: []string{"solo@example.com"}},
	}, nil, "")
	if c.FirstName != "solo" || c.Email != "solo@example.com" {
		t.Fatalf("contact = %+v", c)
	}
}

func TestShouldPushCRM(t *testing.T) {
	if !ShouldPushCRM(nil, false) {
		t.Fatal("nil brief should push")
	}
	if ShouldPushCRM(&model.MetaLeadBrief{SpamRisk: "high"}, false) {
		t.Fatal("high spam should skip")
	}
	if !ShouldPushCRM(&model.MetaLeadBrief{SpamRisk: "high"}, true) {
		t.Fatal("force should push")
	}
}

type stubCRM struct {
	calls []CRMContact
	fail  error
}

func (s *stubCRM) UpsertContact(_ context.Context, _ uuid.UUID, _ string, c CRMContact) (CRMResult, error) {
	s.calls = append(s.calls, c)
	if s.fail != nil {
		return CRMResult{}, s.fail
	}
	return CRMResult{ContactID: "ct-1", Website: "workstation", Existed: false}, nil
}

func (s *stubCRM) ConfiguredFor(context.Context, uuid.UUID, string) bool { return true }

func TestSyncLeadCRMHappyPath(t *testing.T) {
	store := newMemStore()
	org := uuid.New()
	lead := &model.MetaLead{
		ID: uuid.New(), OrgID: org, LeadgenID: "lg9",
		FieldData: FieldDataJSON([]FieldData{{Name: "email", Values: []string{"a@b.com"}}, {Name: "first_name", Values: []string{"Ann"}}}),
		Status:    model.MetaLeadStatusAnalysed,
	}
	_ = store.UpsertLead(context.Background(), lead)

	crm := &stubCRM{}
	svc := NewService(store, fakeGraph{}, nil, Config{}, nil)
	svc.BindCRM(crm)

	brief := &model.MetaLeadBrief{FitScore: 70, SpamRisk: "low"}
	var fields []FieldData
	_ = json.Unmarshal(lead.FieldData, &fields)
	if err := svc.syncLeadCRM(context.Background(), org, lead.ID, fields, brief, "workstation", false); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetLead(context.Background(), org, lead.ID)
	if err != nil || got.CRMContactID != "ct-1" || got.CRMWebsite != "workstation" || got.CRMSyncedAt == nil {
		t.Fatalf("lead = %+v err=%v", got, err)
	}
	if len(crm.calls) != 1 || crm.calls[0].Email != "a@b.com" {
		t.Fatalf("calls = %+v", crm.calls)
	}
}

func TestSyncLeadCRMSkipsHighSpam(t *testing.T) {
	store := newMemStore()
	org := uuid.New()
	lead := &model.MetaLead{ID: uuid.New(), OrgID: org, LeadgenID: "lg", FieldData: FieldDataJSON([]FieldData{{Name: "email", Values: []string{"a@b.com"}}})}
	_ = store.UpsertLead(context.Background(), lead)
	crm := &stubCRM{}
	svc := NewService(store, fakeGraph{}, nil, Config{}, nil)
	svc.BindCRM(crm)
	_ = svc.syncLeadCRM(context.Background(), org, lead.ID, []FieldData{{Name: "email", Values: []string{"a@b.com"}}},
		&model.MetaLeadBrief{SpamRisk: "high"}, "", false)
	got, _ := store.GetLead(context.Background(), org, lead.ID)
	if got.CRMContactID != "" || got.CRMError == "" || len(crm.calls) != 0 {
		t.Fatalf("expected skip: %+v calls=%d", got, len(crm.calls))
	}
}

func TestSyncLeadCRMRecordsError(t *testing.T) {
	store := newMemStore()
	org := uuid.New()
	lead := &model.MetaLead{ID: uuid.New(), OrgID: org, LeadgenID: "lg", FieldData: FieldDataJSON([]FieldData{{Name: "email", Values: []string{"a@b.com"}}})}
	_ = store.UpsertLead(context.Background(), lead)
	crm := &stubCRM{fail: errors.New("boom")}
	svc := NewService(store, fakeGraph{}, nil, Config{}, nil)
	svc.BindCRM(crm)
	err := svc.syncLeadCRM(context.Background(), org, lead.ID, []FieldData{{Name: "email", Values: []string{"a@b.com"}}},
		&model.MetaLeadBrief{SpamRisk: "low"}, "", false)
	if err == nil {
		t.Fatal("want error")
	}
	got, _ := store.GetLead(context.Background(), org, lead.ID)
	if got.CRMError == "" {
		t.Fatal("expected crm_error")
	}
}

func TestLaunchPushesCRM(t *testing.T) {
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
		MetaLeadConnection: model.MetaLeadConnection{ID: uuid.New(), OrgID: org, Name: "ig", PageID: "p1"},
		AccessTokenEnc:     enc,
	}
	store.conns["ig"] = conn
	store.byPage["p1"] = conn

	crm := &stubCRM{}
	svc := NewService(store, fakeGraph{}, nil, Config{TokenKey: tokenKey, MaxLeadsPerLaunch: 50}, nil)
	svc.BindCRM(crm)

	res, err := svc.Launch(context.Background(), LaunchRequest{
		OrgID: org, UserID: uuid.New(), Connection: "ig", Limit: 5, PushCRM: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(crm.calls) >= 1 {
			return
		}
		run, _ := store.GetRun(context.Background(), org, res.RunID)
		if run != nil && run.Status == model.MetaLeadRunStatusDone && len(crm.calls) >= 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("CRM not called; calls=%d", len(crm.calls))
}
