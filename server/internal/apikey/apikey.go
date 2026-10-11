// Package apikey issues and verifies org-scoped API keys.
//
// Key format: "jsk_" + 8 hex (the displayable prefix) + "_" + 43 chars of
// base64url randomness (256 bits). Only SHA-256(key) is stored; the key is
// high-entropy, so a fast hash is the right tool (no password stretching).
package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ErrInvalidKey is returned for a malformed, unknown, revoked or expired key.
var ErrInvalidKey = errors.New("invalid api key")

// Generate returns a new raw key, its display prefix, and its storage hash.
func Generate() (raw, prefix, hash string, err error) {
	id := make([]byte, 4)
	secret := make([]byte, 32)
	if _, err = rand.Read(id); err != nil {
		return "", "", "", err
	}
	if _, err = rand.Read(secret); err != nil {
		return "", "", "", err
	}
	prefix = model.APIKeyPrefix + hex.EncodeToString(id)
	raw = prefix + "_" + base64.RawURLEncoding.EncodeToString(secret)
	return raw, prefix, Hash(raw), nil
}

// Hash is the stored form of a key.
func Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// LooksLikeKey reports whether a bearer token is an API key (vs a JWT).
func LooksLikeKey(token string) bool { return strings.HasPrefix(token, model.APIKeyPrefix) }

// Service manages an org's keys and authenticates them.
type Service struct {
	repo repository.APIKeyRepository
	now  func() time.Time
}

// NewService builds a Service.
func NewService(repo repository.APIKeyRepository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Create issues a key for orgID acting as createdBy. The returned value is the
// only time the full key is available.
func (s *Service) Create(ctx context.Context, orgID, createdBy uuid.UUID, req model.CreateAPIKeyRequest) (*model.CreatedAPIKey, error) {
	if req.ExpiresAt != nil && !req.ExpiresAt.After(s.now()) {
		return nil, fmt.Errorf("expires_at must be in the future")
	}
	scopes := dedupe(req.Scopes)
	for _, sc := range scopes {
		if !validScope(sc) {
			return nil, fmt.Errorf("unknown scope %q", sc)
		}
	}
	raw, prefix, hash, err := Generate()
	if err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}
	k := model.APIKey{
		ID: uuid.New(), OrgID: orgID, Name: strings.TrimSpace(req.Name), Prefix: prefix,
		Scopes: scopes, CreatedBy: &createdBy, ExpiresAt: req.ExpiresAt,
	}
	if err := s.repo.Create(ctx, &k, hash); err != nil {
		return nil, fmt.Errorf("store api key: %w", err)
	}
	return &model.CreatedAPIKey{APIKey: k, Key: raw}, nil
}

// List returns the org's keys (no secrets).
func (s *Service) List(ctx context.Context, orgID uuid.UUID) ([]model.APIKey, error) {
	return s.repo.ListByOrg(ctx, orgID)
}

// Revoke revokes one of the org's keys.
func (s *Service) Revoke(ctx context.Context, orgID, keyID uuid.UUID) error {
	return s.repo.Revoke(ctx, orgID, keyID)
}

// Authenticate resolves a raw bearer key to its principal.
func (s *Service) Authenticate(ctx context.Context, raw string) (*model.APIKeyPrincipal, error) {
	if !LooksLikeKey(raw) || len(raw) < 20 {
		return nil, ErrInvalidKey
	}
	p, err := s.repo.Authenticate(ctx, Hash(raw))
	if err != nil {
		if errors.Is(err, repository.ErrAPIKeyNotFound) {
			return nil, ErrInvalidKey
		}
		return nil, err
	}
	return p, nil
}

func validScope(s string) bool {
	for _, v := range model.AllAPIKeyScopes {
		if v == s {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
