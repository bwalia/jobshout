package platformtools

import (
	"context"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

type identityKey struct{}
type disclosedKey struct{}
type sessionEntitiesKey struct{}

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

// WithSessionEntities attaches the mutable session entity map so tools can
// read last_{kind} (for example last_execution) without an LLM-supplied id.
func WithSessionEntities(ctx context.Context, ents map[string]model.SessionEntity) context.Context {
	return context.WithValue(ctx, sessionEntitiesKey{}, ents)
}

// SessionEntitiesFrom returns the session entity map, or nil.
func SessionEntitiesFrom(ctx context.Context) map[string]model.SessionEntity {
	ents, _ := ctx.Value(sessionEntitiesKey{}).(map[string]model.SessionEntity)
	return ents
}

// PermissionsFrom returns the RBAC set attached by WithPermissions.
func PermissionsFrom(ctx context.Context) map[string]bool {
	m, _ := ctxPerms(ctx)
	return m
}
