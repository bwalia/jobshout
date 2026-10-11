package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type memRepo struct {
	byHash map[string]model.APIKey
}

func (m *memRepo) Create(_ context.Context, k *model.APIKey, hash string) error {
	m.byHash[hash] = *k
	return nil
}
func (m *memRepo) ListByOrg(context.Context, uuid.UUID) ([]model.APIKey, error) { return nil, nil }
func (m *memRepo) Revoke(context.Context, uuid.UUID, uuid.UUID) error           { return nil }
func (m *memRepo) Authenticate(_ context.Context, hash string) (*model.APIKeyPrincipal, error) {
	k, ok := m.byHash[hash]
	if !ok {
		return nil, repository.ErrAPIKeyNotFound
	}
	return &model.APIKeyPrincipal{KeyID: k.ID, OrgID: k.OrgID, UserID: *k.CreatedBy, Scopes: k.Scopes}, nil
}

func TestCreateAndAuthenticate(t *testing.T) {
	repo := &memRepo{byHash: map[string]model.APIKey{}}
	svc := NewService(repo)
	org, user := uuid.New(), uuid.New()
	created, err := svc.Create(context.Background(), org, user, model.CreateAPIKeyRequest{
		Name: "OpsAPI", Scopes: []string{model.ScopeRuns, model.ScopeRuns, model.ScopeApprovals},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Key, created.Prefix+"_") || !LooksLikeKey(created.Key) || len(created.Key) < 50 {
		t.Fatalf("key %q / prefix %q", created.Key, created.Prefix)
	}
	if len(created.Scopes) != 2 {
		t.Fatalf("scopes not de-duplicated: %v", created.Scopes)
	}
	for h := range repo.byHash {
		if strings.Contains(h, created.Key) || h != Hash(created.Key) {
			t.Fatal("repository received something other than the hash")
		}
	}
	p, err := svc.Authenticate(context.Background(), created.Key)
	if err != nil || p.OrgID != org || p.UserID != user {
		t.Fatalf("authenticate = %+v, %v", p, err)
	}
	if _, err := svc.Authenticate(context.Background(), created.Key+"x"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("tampered key err = %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), "eyJhbGciOi.jwt"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("jwt-shaped token err = %v", err)
	}

	past := time.Now().Add(-time.Minute)
	if _, err := svc.Create(context.Background(), org, user, model.CreateAPIKeyRequest{Name: "x", Scopes: []string{model.ScopeRuns}, ExpiresAt: &past}); err == nil {
		t.Fatal("past expiry accepted")
	}
	if _, err := svc.Create(context.Background(), org, user, model.CreateAPIKeyRequest{Name: "x", Scopes: []string{"admin"}}); err == nil {
		t.Fatal("unknown scope accepted")
	}
}
