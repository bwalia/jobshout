package platformtools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/tools"
)

type fakeRBAC struct {
	perms map[string]bool
}

func (f fakeRBAC) UserHasPermission(_ context.Context, _, _ uuid.UUID, p string) (bool, error) {
	return f.perms[p], nil
}
func (f fakeRBAC) UserPermissions(_ context.Context, _, _ uuid.UUID) ([]string, error) {
	var out []string
	for p, ok := range f.perms {
		if ok {
			out = append(out, p)
		}
	}
	return out, nil
}

type fakePolicy struct {
	err error
}

func (f fakePolicy) ListPolicies(context.Context, uuid.UUID) ([]model.AgentPolicy, error) {
	return nil, nil
}
func (f fakePolicy) EnforcePolicy(context.Context, uuid.UUID, uuid.UUID, string, string) error {
	return f.err
}

func identCtx() context.Context {
	return WithIdentity(context.Background(), Identity{OrgID: uuid.New(), UserID: uuid.New()})
}

func TestGuard_RBACDenied(t *testing.T) {
	g := NewGuard(fakeRBAC{perms: map[string]bool{}}, nil)
	tool := newTool("task_create", "c", "work", model.PermTasksCreate, false, false, tools.ObjectSchema(nil), func(context.Context, map[string]any) (*Result, error) {
		t.Fatal("must not run")
		return nil, nil
	})
	err := g.Check(identCtx(), tool, nil, false)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("err = %v; want denied", err)
	}
	if !strings.Contains(err.Error(), model.PermTasksCreate) {
		t.Fatalf("should name the permission: %v", err)
	}
}

func TestGuard_DestructiveNeedsConfirm(t *testing.T) {
	g := NewGuard(nil, nil)
	tool := newTool("agent_delete", "d", "agents", model.PermAgentsDelete, true, false, tools.ObjectSchema(nil), func(context.Context, map[string]any) (*Result, error) {
		t.Fatal("must not run")
		return nil, nil
	})
	err := g.Check(identCtx(), tool, nil, false)
	if !errors.Is(err, ErrNeedsConfirm) {
		t.Fatalf("err = %v; want confirm", err)
	}
	if err := g.Check(identCtx(), tool, nil, true); err != nil {
		t.Fatalf("confirmed should pass: %v", err)
	}
}

func TestGuard_PolicyBlocksExecute(t *testing.T) {
	g := NewGuard(nil, fakePolicy{err: errors.New("budget limit exceeded: daily cap")})
	tool := newTool("agent_execute", "e", "agents", "", false, false, tools.ObjectSchema(nil), func(context.Context, map[string]any) (*Result, error) {
		t.Fatal("must not run")
		return nil, nil
	})
	err := g.Check(identCtx(), tool, nil, false)
	if !errors.Is(err, ErrPolicy) {
		t.Fatalf("err = %v; want policy", err)
	}
}

func TestGuard_EmptyPermissionAllowed(t *testing.T) {
	g := NewGuard(fakeRBAC{perms: map[string]bool{}}, nil)
	tool := newTool("help", "h", "config", "", false, true, tools.ObjectSchema(nil), func(context.Context, map[string]any) (*Result, error) {
		return &Result{}, nil
	})
	if err := g.Check(identCtx(), tool, nil, false); err != nil {
		t.Fatalf("help should be allowed: %v", err)
	}
}

func TestGuard_NoIdentity(t *testing.T) {
	g := NewGuard(nil, nil)
	tool := newTool("help", "h", "config", "", false, true, tools.ObjectSchema(nil), func(context.Context, map[string]any) (*Result, error) {
		return &Result{}, nil
	})
	if err := g.Check(context.Background(), tool, nil, false); err == nil {
		t.Fatal("expected no-identity error")
	}
}

func TestFilterByPermissions(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newTool("help", "h", "config", "", false, true, map[string]any{"type": "object"}, nilRun))
	reg.Register(newTool("agent_delete", "d", "agents", model.PermAgentsDelete, true, false, map[string]any{"type": "object"}, nilRun))
	allowed := reg.FilterByPermissions(map[string]bool{model.PermAgentsDelete: false})
	for _, t0 := range allowed {
		if t0.Name() == "agent_delete" {
			t.Fatal("delete must not be offered without permission")
		}
	}
	if len(reg.FilterByPermissions(nil)) != 2 {
		t.Fatal("nil map means all tools")
	}
}

func nilRun(context.Context, map[string]any) (*Result, error) { return &Result{}, nil }
