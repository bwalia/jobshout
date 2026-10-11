package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestAPIKeyRepoPG_LifecycleAndIsolation(t *testing.T) {
	pool := platformPG(t)
	ctx := context.Background()
	repo := NewAPIKeyRepository(pool)
	orgA, _ := seedOrgAgent(t, pool)
	orgB, _ := seedOrgAgent(t, pool)
	admin := seedUser(t, pool, orgA)

	mk := func(raw string, exp *time.Time) *model.APIKey {
		k := &model.APIKey{OrgID: orgA, Name: "opsapi", Prefix: raw[:12], Scopes: []string{model.ScopeRuns}, CreatedBy: &admin, ExpiresAt: exp}
		if err := repo.Create(ctx, k, hashOf(raw)); err != nil {
			t.Fatal(err)
		}
		return k
	}
	live := mk("jsk_aaaaaaaa_live", nil)
	past := time.Now().Add(-time.Hour)
	mk("jsk_bbbbbbbb_expired", &past)

	p, err := repo.Authenticate(ctx, hashOf("jsk_aaaaaaaa_live"))
	if err != nil || p.OrgID != orgA || p.UserID != admin || !p.HasScope(model.ScopeRuns) {
		t.Fatalf("authenticate live = %+v, %v", p, err)
	}
	if _, err := repo.Authenticate(ctx, hashOf("jsk_bbbbbbbb_expired")); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("expired key err = %v", err)
	}
	if _, err := repo.Authenticate(ctx, hashOf("jsk_unknown")); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("unknown key err = %v", err)
	}

	// The hash, not the key, is what is stored.
	var stored string
	_ = pool.QueryRow(ctx, `SELECT key_hash FROM org_api_keys WHERE id = $1`, live.ID).Scan(&stored)
	if stored != hashOf("jsk_aaaaaaaa_live") {
		t.Fatal("stored value is not the key hash")
	}

	// Org B can neither list nor revoke org A's key.
	if keys, _ := repo.ListByOrg(ctx, orgB); len(keys) != 0 {
		t.Fatalf("org B lists %d of A's keys", len(keys))
	}
	if err := repo.Revoke(ctx, orgB, live.ID); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("cross-org revoke err = %v", err)
	}
	if _, err := repo.Authenticate(ctx, hashOf("jsk_aaaaaaaa_live")); err != nil {
		t.Fatal("cross-org revoke took effect")
	}

	// The key stops working when its creator leaves the org.
	_, _ = pool.Exec(ctx, `UPDATE users SET org_id = $2 WHERE id = $1`, admin, orgB)
	if _, err := repo.Authenticate(ctx, hashOf("jsk_aaaaaaaa_live")); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("key of a departed creator err = %v", err)
	}
	_, _ = pool.Exec(ctx, `UPDATE users SET org_id = $2 WHERE id = $1`, admin, orgA)

	if err := repo.Revoke(ctx, orgA, live.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Authenticate(ctx, hashOf("jsk_aaaaaaaa_live")); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("revoked key err = %v", err)
	}
	keys, _ := repo.ListByOrg(ctx, orgA)
	if len(keys) != 2 || uuid.Nil == keys[0].ID {
		t.Fatalf("org A keys = %+v", keys)
	}
}
