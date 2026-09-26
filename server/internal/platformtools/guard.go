package platformtools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// Guard errors. The chat loop maps these to ActionRecord statuses.
var (
	ErrDenied       = errors.New("permission denied")
	ErrNeedsConfirm = errors.New("confirmation required")
	ErrPolicy       = errors.New("blocked by policy")
	ErrUnknownTool  = errors.New("unknown tool")
)

// PermissionChecker is the RBAC surface the guard needs.
type PermissionChecker interface {
	UserHasPermission(ctx context.Context, userID, orgID uuid.UUID, permission string) (bool, error)
	UserPermissions(ctx context.Context, userID, orgID uuid.UUID) ([]string, error)
}

// PolicyEnforcer is the governance surface the guard needs.
type PolicyEnforcer interface {
	ListPolicies(ctx context.Context, orgID uuid.UUID) ([]model.AgentPolicy, error)
	EnforcePolicy(ctx context.Context, orgID, agentID uuid.UUID, provider, modelName string) error
}

// Guard is policy → RBAC → org scope → confirm, applied to every tool call.
type Guard struct {
	rbac     PermissionChecker
	policies PolicyEnforcer
	cache    sync.Map // orgID string → policyCache
}

type policyCache struct {
	at       time.Time
	policies []model.AgentPolicy
}

const policyTTL = 15 * time.Second

func NewGuard(rbac PermissionChecker, policies PolicyEnforcer) *Guard {
	return &Guard{rbac: rbac, policies: policies}
}

// PermissionsFor returns the caller's permission set, or nil if RBAC is unset
// (tests / bootstrap). A nil map means "allow every registered tool".
func (g *Guard) PermissionsFor(ctx context.Context, ident Identity) (map[string]bool, error) {
	if g == nil || g.rbac == nil {
		return nil, nil
	}
	perms, err := g.rbac.UserPermissions(ctx, ident.UserID, ident.OrgID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(perms))
	for _, p := range perms {
		out[p] = true
	}
	return out, nil
}

// Check runs the guard chain. confirmed is true when the caller presented a
// valid confirmation token for this exact tool+args.
func (g *Guard) Check(ctx context.Context, t PlatformTool, args map[string]any, confirmed bool) error {
	if t == nil {
		return ErrUnknownTool
	}
	ident, ok := IdentityFrom(ctx)
	if !ok || ident.OrgID == uuid.Nil {
		return errNoIdentity
	}

	if err := g.checkPolicy(ctx, ident, t); err != nil {
		return err
	}
	if err := g.checkRBAC(ctx, ident, t); err != nil {
		return err
	}
	if t.Destructive() && !confirmed {
		return fmt.Errorf("%w: %s", ErrNeedsConfirm, t.Name())
	}
	return nil
}

func (g *Guard) checkRBAC(ctx context.Context, ident Identity, t PlatformTool) error {
	if t.Permission() == "" || g == nil || g.rbac == nil {
		return nil
	}
	ok, err := g.rbac.UserHasPermission(ctx, ident.UserID, ident.OrgID, t.Permission())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: you need the %s permission. Ask an organisation admin to grant it", ErrDenied, t.Permission())
	}
	return nil
}

func (g *Guard) checkPolicy(ctx context.Context, ident Identity, t PlatformTool) error {
	if g == nil || g.policies == nil {
		return nil
	}
	// Execute-class tools go through the structured governance enforcer.
	switch t.Name() {
	case "agent_execute", "workflow_run", "multi_agent_run", "goal_create":
		if err := g.policies.EnforcePolicy(ctx, ident.OrgID, uuid.Nil, "", ""); err != nil {
			if errors.Is(err, service.ErrPolicyBlocked) || errors.Is(err, service.ErrBudgetExceeded) {
				return fmt.Errorf("%w: %s", ErrPolicy, humaniseError(err))
			}
			return fmt.Errorf("%w: %s", ErrPolicy, humaniseError(err))
		}
	}
	return nil
}

func (g *Guard) Policies(ctx context.Context, orgID uuid.UUID) []model.AgentPolicy {
	if g == nil || g.policies == nil {
		return nil
	}
	key := orgID.String()
	if v, ok := g.cache.Load(key); ok {
		c := v.(policyCache)
		if time.Since(c.at) < policyTTL {
			return c.policies
		}
	}
	list, err := g.policies.ListPolicies(ctx, orgID)
	if err != nil {
		return nil
	}
	g.cache.Store(key, policyCache{at: time.Now(), policies: list})
	return list
}

func humaniseError(err error) string {
	if err == nil {
		return "something went wrong"
	}
	s := err.Error()
	s = strings.TrimPrefix(s, "execution blocked by policy: ")
	s = strings.TrimPrefix(s, "budget limit exceeded: ")
	return s
}

var _ PermissionChecker = (service.RBACService)(nil)
var _ PolicyEnforcer = (service.GovernanceService)(nil)
