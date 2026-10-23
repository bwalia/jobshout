package imagestore

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore keeps images in Postgres (table image_blobs, migration 000070).
//
// It is the store for a ring with no MinIO. The directory store it replaces
// wrote to the pod's own disk, so with two API replicas a cover drawn on one
// pod 404'd whenever the other served it, and every redeploy deleted them all —
// while the public website kept linking to them as blog featured images.
// Postgres is shared by every replica and outlives deploys.
//
// ponytail: bytes in Postgres grow the database (~1.5 MB per drawn cover, more
// for videos). Fine at a few articles a day; set MINIO_ENDPOINT to move a busy
// ring to object storage — the URLs are the same, only new images move.
type PGStore struct {
	db *pgxpool.Pool
}

// NewPGStore stores images in db.
func NewPGStore(db *pgxpool.Pool) *PGStore {
	return &PGStore{db: db}
}

// Put implements Store.
func (s *PGStore) Put(ctx context.Context, orgID uuid.UUID, png []byte) (string, error) {
	return s.PutFile(ctx, orgID, png, "png", "image/png")
}

// PutFile implements Store. The content type is not kept: the file route
// derives it from the key's extension, as it does for the directory store.
func (s *PGStore) PutFile(ctx context.Context, orgID uuid.UUID, data []byte, ext, _ string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("imagestore: refusing to store an empty file")
	}
	key := newKey(orgID, ext)
	if _, err := s.db.Exec(ctx, `INSERT INTO image_blobs (key, data) VALUES ($1, $2)`, key, data); err != nil {
		return "", fmt.Errorf("imagestore: store %s: %w", ext, err)
	}
	return URLPrefix + key, nil
}

// Get implements Store.
func (s *PGStore) Get(ctx context.Context, key string) ([]byte, error) {
	clean, err := safeKey(key)
	if err != nil {
		return nil, err
	}
	var data []byte
	err = s.db.QueryRow(ctx, `SELECT data FROM image_blobs WHERE key = $1`, clean).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("imagestore: image not found")
	}
	if err != nil {
		return nil, fmt.Errorf("imagestore: read image: %w", err)
	}
	return data, nil
}
