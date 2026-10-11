package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/secretbox"
)

func TestWebhookRepoPG(t *testing.T) {
	pool := platformPG(t)
	ctx := context.Background()
	box, _ := secretbox.New("k")
	repo := NewWebhookRepository(pool, box)
	orgA, _ := seedOrgAgent(t, pool)
	orgB, _ := seedOrgAgent(t, pool)

	secret := strings.Join([]string{"whsec", "abc123456789"}, "_")
	runs := &model.WebhookEndpoint{OrgID: orgA, URL: "https://a.example/hook", Secret: secret,
		Events: []string{model.WebhookEventRunCompleted}, Enabled: true}
	all := &model.WebhookEndpoint{OrgID: orgA, URL: "https://a.example/all", Secret: secret, Events: []string{"*"}, Enabled: true}
	off := &model.WebhookEndpoint{OrgID: orgA, URL: "https://a.example/off", Secret: secret, Events: []string{"*"}, Enabled: false}
	for _, e := range []*model.WebhookEndpoint{runs, all, off} {
		if err := repo.CreateEndpoint(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var stored string
	_ = pool.QueryRow(ctx, `SELECT secret FROM org_webhook_endpoints WHERE id = $1`, runs.ID).Scan(&stored)
	if !secretbox.IsSealed(stored) {
		t.Fatal("secret stored unsealed")
	}

	sub, err := repo.ListSubscribed(ctx, orgA, model.WebhookEventRunCompleted)
	if err != nil || len(sub) != 2 || sub[0].Secret != secret {
		t.Fatalf("subscribed run.completed = %d %v", len(sub), err)
	}
	if sub, _ := repo.ListSubscribed(ctx, orgA, model.WebhookEventApprovalDecided); len(sub) != 1 || sub[0].ID != all.ID {
		t.Fatalf("subscribed approval.decided = %+v", sub)
	}
	if sub, _ := repo.ListSubscribed(ctx, orgB, model.WebhookEventRunCompleted); len(sub) != 0 {
		t.Fatal("org B sees org A's endpoints")
	}
	if _, err := repo.GetEndpoint(ctx, orgB, runs.ID); !errors.Is(err, ErrWebhookNotFound) {
		t.Fatalf("cross-org get err = %v", err)
	}
	if err := repo.DeleteEndpoint(ctx, orgB, runs.ID); !errors.Is(err, ErrWebhookNotFound) {
		t.Fatalf("cross-org delete err = %v", err)
	}
	runs.OrgID = orgB
	if err := repo.UpdateEndpoint(ctx, runs); !errors.Is(err, ErrWebhookNotFound) {
		t.Fatalf("cross-org update err = %v", err)
	}
	runs.OrgID = orgA

	// Delivery lease: a due delivery is claimed once, not again within the lease.
	past := time.Now().Add(-time.Second)
	d := &model.WebhookDelivery{EndpointID: runs.ID, OrgID: orgA, EventID: uuid.New(), EventType: model.WebhookEventRunCompleted,
		Payload: []byte(`{"x":1}`), NextAttemptAt: &past}
	if err := repo.CreateDelivery(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ClaimDue(ctx, 10, time.Minute)
	if err != nil || len(got) != 1 || got[0].ID != d.ID {
		t.Fatalf("claim = %+v, %v", got, err)
	}
	if again, _ := repo.ClaimDue(ctx, 10, time.Minute); len(again) != 0 {
		t.Fatal("leased delivery claimed twice")
	}
	code := 200
	if err := repo.RecordAttempt(ctx, d.ID, model.WebhookDeliverySucceeded, 1, &code, nil, nil); err != nil {
		t.Fatal(err)
	}
	list, _ := repo.ListDeliveries(ctx, orgA, runs.ID, 10)
	if len(list) != 1 || list[0].Status != model.WebhookDeliverySucceeded || list[0].DeliveredAt == nil {
		t.Fatalf("deliveries = %+v", list)
	}
	if list, _ := repo.ListDeliveries(ctx, orgB, runs.ID, 10); len(list) != 0 {
		t.Fatal("org B sees org A's deliveries")
	}
}
