package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func TestOrgLLMRoutingPG_UpsertGet(t *testing.T) {
	pool := platformPG(t)
	ctx := context.Background()
	orgID, _ := seedOrgAgent(t, pool)
	repo := NewOrgLLMRoutingRepository(pool)

	if got, err := repo.Get(ctx, orgID); err != nil || got != nil {
		t.Fatalf("absent row = %+v, %v", got, err)
	}
	step := model.LLMRouteStep{ProviderConfigID: uuid.New(), Model: "m1"}
	if err := repo.Upsert(ctx, &model.OrgLLMRouting{OrgID: orgID, Enabled: true, Chain: []model.LLMRouteStep{step}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(ctx, &model.OrgLLMRouting{OrgID: orgID, Enabled: false, Chain: []model.LLMRouteStep{step, step}}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, orgID)
	if err != nil || got.Enabled || len(got.Chain) != 2 || got.Chain[0] != step {
		t.Fatalf("got %+v, %v", got, err)
	}
}
