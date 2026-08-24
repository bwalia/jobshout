package platformtools

import (
	"context"

	"github.com/google/uuid"
)

type identityKey struct{}
type disclosedKey struct{}

// Identity is the authenticated caller. Tools read org and user from here
// and must never accept an org_id argument from the model.
type Identity struct {
	OrgID     uuid.UUID
	UserID    uuid.UUID
	SessionID uuid.UUID
}

func WithIdentity(ctx context.Context, ident Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, ident)
}

func IdentityFrom(ctx context.Context) (Identity, bool) {
	v, ok := ctx.Value(identityKey{}).(Identity)
	return v, ok
}

func MustIdentity(ctx context.Context) Identity {
	ident, ok := IdentityFrom(ctx)
	if !ok {
		return Identity{}
	}
	return ident
}

// AddDisclosedTools records tool names catalog_search revealed this turn so
// the loop can inject their schemas on the next iteration.
func AddDisclosedTools(ctx context.Context, names []string) context.Context {
	existing, _ := ctx.Value(disclosedKey{}).([]string)
	return context.WithValue(ctx, disclosedKey{}, append(append([]string{}, existing...), names...))
}

func DisclosedTools(ctx context.Context) []string {
	names, _ := ctx.Value(disclosedKey{}).([]string)
	return names
}

// PermissionsFrom returns the RBAC set attached by WithPermissions.
func PermissionsFrom(ctx context.Context) map[string]bool {
	m, _ := ctxPerms(ctx)
	return m
}
