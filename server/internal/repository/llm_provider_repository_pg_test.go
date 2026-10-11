package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/secretbox"
)

func TestLLMProviderRepoPG_SealsAPIKey(t *testing.T) {
	pool := platformPG(t)
	ctx := context.Background()
	orgID, _ := seedOrgAgent(t, pool)
	box, _ := secretbox.New("test-key")
	repo := NewLLMProviderRepositoryWithBox(pool, box)

	key := strings.Join([]string{"sk", "live", "abcdef123456"}, "-")
	p := &model.LLMProviderConfig{ID: uuid.New(), OrgID: orgID, Name: "byo", ProviderType: "openai",
		BaseURL: "https://example.invalid", APIKey: key, DefaultModel: "m", IsActive: true}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := pool.QueryRow(ctx, `SELECT api_key FROM llm_provider_configs WHERE id = $1`, p.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !secretbox.IsSealed(stored) || strings.Contains(stored, key) {
		t.Fatalf("api_key stored unsealed: %q", stored)
	}
	got, err := repo.GetByID(ctx, p.ID)
	if err != nil || got.APIKey != key {
		t.Fatalf("GetByID key = %q, %v", got.APIKey, err)
	}
	active, err := repo.ListActiveWithSecrets(ctx, orgID)
	if err != nil || len(active) != 1 || active[0].APIKey != key {
		t.Fatalf("ListActiveWithSecrets = %+v, %v", active, err)
	}
	// A legacy plaintext row stays readable.
	if _, err := pool.Exec(ctx, `UPDATE llm_provider_configs SET api_key = 'legacy' WHERE id = $1`, p.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetByID(ctx, p.ID); got.APIKey != "legacy" {
		t.Fatalf("legacy key = %q", got.APIKey)
	}
	// Update re-seals.
	nk := "rotated-key"
	if _, err := repo.Update(ctx, p.ID, model.UpdateLLMProviderRequest{APIKey: &nk}); err != nil {
		t.Fatal(err)
	}
	_ = pool.QueryRow(ctx, `SELECT api_key FROM llm_provider_configs WHERE id = $1`, p.ID).Scan(&stored)
	if !secretbox.IsSealed(stored) {
		t.Fatal("updated key stored unsealed")
	}
}
