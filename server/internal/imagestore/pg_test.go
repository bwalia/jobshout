package imagestore

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Runs against a real database when one is given:
//
//	IMAGESTORE_TEST_DATABASE_URL=postgres://postgres:pw@localhost:5432/postgres go test ./internal/imagestore -run PG
func TestPGStore_RoundTripAcrossConnections(t *testing.T) {
	dsn := os.Getenv("IMAGESTORE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("IMAGESTORE_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	migration, err := os.ReadFile("../../migrations/000070_image_blobs.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Migrations replay on every boot: applying twice must be harmless.
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}

	org := uuid.New()
	img := []byte("\x89PNG\r\n\x1a\n...a cover...")
	url, err := NewPGStore(pool).PutFile(ctx, org, img, "png", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	key, ok := strings.CutPrefix(url, URLPrefix)
	if !ok || !strings.HasPrefix(key, org.String()+"/") || !strings.HasSuffix(key, ".png") {
		t.Fatalf("url %q is not an org-scoped image URL", url)
	}

	// A second store on a separate pool stands in for the other API replica:
	// it must read what the first one wrote (the pod-local store could not).
	other, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	got, err := NewPGStore(other).Get(ctx, key)
	if err != nil || !bytes.Equal(got, img) {
		t.Fatalf("other replica read %q, %v", got, err)
	}

	if _, err := NewPGStore(pool).Get(ctx, org.String()+"/2026/10/missing.png"); err == nil {
		t.Fatal("a missing image must be an error")
	}
	if _, err := NewPGStore(pool).Get(ctx, "../etc/passwd"); err == nil {
		t.Fatal("a path-escaping key must be refused")
	}
	if _, err := NewPGStore(pool).PutFile(ctx, org, nil, "png", "image/png"); err == nil {
		t.Fatal("an empty image must be refused")
	}
}
