package chattools_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentmodules"
	"github.com/jobshout/server/internal/marketingmanager"
	"github.com/jobshout/server/internal/marketingmanager/chattools"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/platformtools"
)

// Wired exactly as main does it, then driven through the real chat registry
// and guard: the agent is registered, its tools reach chat, and a send is held
// for confirmation before OpsAPI is called at all.
func TestWiredLikeMainThroughRegistryAndGuard(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		data := any(map[string]any{"uuid": "c1", "name": "Launch", "status": "validated"})
		if strings.HasSuffix(r.URL.Path, "/sending") {
			data = map[string]any{"dry_run": true}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
	}))
	t.Cleanup(srv.Close)

	svc := &marketingmanager.Service{Ring: marketingmanager.RingConfig{BaseURL: srv.URL, Key: "k", Namespace: "workstation"}}
	agentmodule.SetToolInstaller(model.BuiltinMarketingManager, chattools.Installer(svc))
	agentmodules.Register(agentmodules.Deps{Marketing: svc})

	m, ok := agentmodule.Lookup(model.BuiltinMarketingManager)
	if !ok || m.Seed == nil || m.Launch == nil || m.InstallTools == nil || len(m.Schema.Fields) == 0 || m.ChatHint == "" {
		t.Fatalf("module not fully registered: ok=%v seed=%v launch=%v tools=%v", ok, m.Seed != nil, m.Launch != nil, m.InstallTools != nil)
	}
	if b := m.Seed(uuid.New()).Metadata[model.MetadataKeyBuiltin]; b != model.BuiltinMarketingManager {
		t.Fatalf("seed builtin %v", b)
	}

	reg := platformtools.NewRegistryWithTools(platformtools.Deps{})
	send, ok := reg.Get("marketing_send")
	if !ok {
		t.Fatal("marketing_send did not reach the chat registry")
	}
	write, ok := reg.Get("marketing_write")
	if !ok {
		t.Fatal("marketing_write did not reach the chat registry")
	}

	guard := platformtools.NewGuard(nil, nil)
	ctx := platformtools.WithIdentity(context.Background(), platformtools.Identity{OrgID: uuid.New(), UserID: uuid.New()})
	args := map[string]any{"action": "schedule_campaign", "id": "c1"}
	if err := guard.Check(ctx, send, args, false); !errors.Is(err, platformtools.ErrNeedsConfirm) {
		t.Fatalf("unconfirmed send: want ErrNeedsConfirm, got %v", err)
	}
	if err := guard.Check(ctx, write, map[string]any{"action": "create_template"}, false); err != nil {
		t.Fatalf("drafts should not need confirmation: %v", err)
	}
	mu.Lock()
	n := len(calls)
	mu.Unlock()
	if n != 0 {
		t.Fatalf("OpsAPI called before confirmation: %v", calls)
	}
	// Without a signed-in org there is nothing to act for.
	if err := guard.Check(context.Background(), send, args, true); err == nil {
		t.Fatal("call without identity passed the guard")
	}
}
