package chattools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/marketingmanager"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/platformtools"
)

type auditRec struct{ logs []*model.AuditLog }

func (a *auditRec) RecordAction(_ context.Context, l *model.AuditLog) error {
	a.logs = append(a.logs, l)
	return nil
}

func fakeServer(t *testing.T, dryRun bool) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := any(map[string]any{"uuid": "c1", "name": "Shop launch", "status": "validated"})
		switch {
		case strings.HasSuffix(r.URL.Path, "/sending"):
			data = map[string]any{"dry_run": dryRun}
		case strings.HasSuffix(r.URL.Path, "/audience-preview"):
			data = map[string]any{"audience_total": 120, "eligible": 100}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func registry(svc *Service) *platformtools.Registry {
	reg := platformtools.NewRegistry()
	Installer(svc)(reg, platformtools.Deps{})
	return reg
}

func TestToolsByKind(t *testing.T) {
	reg := registry(&Service{})
	for name, destructive := range map[string]bool{"marketing_read": false, "marketing_write": false, "marketing_send": true} {
		tl, ok := reg.Get(name)
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if tl.Destructive() != destructive {
			t.Errorf("%s destructive=%v", name, tl.Destructive())
		}
	}
	// Reading needs agents.read; anything that changes or sends needs agents.execute.
	for name, perm := range map[string]string{"marketing_read": model.PermAgentsRead, "marketing_write": model.PermAgentsExecute, "marketing_send": model.PermAgentsExecute} {
		tl, _ := reg.Get(name)
		if p, ok := tl.(interface{ Permission() string }); !ok || p.Permission() != perm {
			t.Errorf("%s permission wrong", name)
		}
	}
	send, _ := reg.Get("marketing_send")
	if _, ok := send.(platformtools.EffectDescriber); !ok {
		t.Fatal("marketing_send has no effect description for the approval")
	}
}

func TestSendDescribedAndAudited(t *testing.T) {
	srv := fakeServer(t, true)
	svc := &Service{Ring: marketingmanager.RingConfig{BaseURL: srv.URL, Key: "k", Namespace: "workstation"}}
	audit := &auditRec{}
	tl := &tool{svc: svc, kind: Send, audit: audit}
	ctx := platformtools.WithIdentity(context.Background(), platformtools.Identity{OrgID: uuid.New(), UserID: uuid.New()})
	args := map[string]any{"action": "schedule_campaign", "id": "c1"}

	eff := tl.DescribeEffect(ctx, args)
	for _, want := range []string{"Shop launch", "100 eligible", "TEST MODE", "workstation"} {
		if !strings.Contains(eff, want) {
			t.Errorf("effect %q lacks %q", eff, want)
		}
	}
	if _, err := tl.Run(ctx, args); err != nil {
		t.Fatal(err)
	}
	if len(audit.logs) != 1 || audit.logs[0].Action != "marketing.schedule_campaign" || audit.logs[0].Metadata["send_mode"] != "dry_run" {
		t.Fatalf("audit %+v", audit.logs)
	}
}

func TestRefusedSendIsAudited(t *testing.T) {
	srv := fakeServer(t, false) // live workspace, live send not allowed
	svc := &Service{Ring: marketingmanager.RingConfig{BaseURL: srv.URL, Key: "k", Namespace: "workstation"}}
	audit := &auditRec{}
	tl := &tool{svc: svc, kind: Send, audit: audit}
	ctx := platformtools.WithIdentity(context.Background(), platformtools.Identity{OrgID: uuid.New()})
	if _, err := tl.Run(ctx, map[string]any{"action": "schedule_campaign", "id": "c1", "live_send_approved": true}); err == nil {
		t.Fatal("live send went ahead on a server that blocks it")
	}
	if len(audit.logs) != 1 || audit.logs[0].Metadata["outcome"] != "error" {
		t.Fatalf("audit %+v", audit.logs)
	}
}
