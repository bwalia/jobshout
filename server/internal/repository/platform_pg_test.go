package repository

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/database"
)

// platformPG returns a pool on a throwaway database that has every migration
// applied — twice, so the replay-on-boot idempotency the server relies on is
// exercised too. Shared by the OpsAPI-platform tests (API keys, webhooks,
// routing, knowledge tenancy) in this package:
//
//	PLATFORM_TEST_DATABASE_URL=postgres://postgres:pw@localhost:5432/postgres go test ./internal/repository -run PG
var (
	platformOnce sync.Once
	platformPool *pgxpool.Pool
	platformErr  error
)

func platformPG(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL not set")
	}
	platformOnce.Do(func() {
		ctx := context.Background()
		admin, err := pgxpool.New(ctx, dsn)
		if err != nil {
			platformErr = err
			return
		}
		defer admin.Close()
		name := "platform_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
		if _, err := admin.Exec(ctx, `CREATE DATABASE `+name); err != nil {
			platformErr = err
			return
		}
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			platformErr = err
			return
		}
		cfg.ConnConfig.Database = name
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			platformErr = err
			return
		}
		for i := 0; i < 2; i++ {
			if err := database.RunMigrations(ctx, pool, "../../migrations", zap.NewNop()); err != nil {
				platformErr = err
				return
			}
		}
		platformPool = pool
	})
	if platformErr != nil {
		t.Fatalf("platform test database: %v", platformErr)
	}
	return platformPool
}

// seedOrgAgent inserts an org and one agent in it, returning both ids.
func seedOrgAgent(t *testing.T, pool *pgxpool.Pool) (orgID, agentID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	orgID, agentID = uuid.New(), uuid.New()
	slug := "org-" + orgID.String()[:8]
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name, slug) VALUES ($1, $2, $3)`, orgID, slug, slug); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO agents (id, org_id, name, role) VALUES ($1, $2, 'a', 'r')`, agentID, orgID); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	return orgID, agentID
}

func TestPlatformPG_MigrationsReplay(t *testing.T) {
	_ = platformPG(t)
}
