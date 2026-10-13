package imagestore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// DirStore keeps generated images on the local filesystem.
//
// Used when MinIO is not configured (a laptop without Docker). The serve path
// is the same as MinIOStore, so the article renderer and /images/file handler
// do not care which backend wrote the bytes.
type DirStore struct {
	root string
}

// NewDirStore writes under root, creating it on first Put.
func NewDirStore(root string) *DirStore {
	return &DirStore{root: root}
}

// Put implements Store.
func (s *DirStore) Put(ctx context.Context, orgID uuid.UUID, png []byte) (string, error) {
	return s.PutFile(ctx, orgID, png, "png", "image/png")
}

// PutFile implements Store. The content type is not kept: the file route
// derives it from the extension.
func (s *DirStore) PutFile(_ context.Context, orgID uuid.UUID, data []byte, ext, _ string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("imagestore: refusing to store an empty file")
	}
	key := newKey(orgID, ext)
	path := filepath.Join(s.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("imagestore: create dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("imagestore: write %s: %w", ext, err)
	}
	return URLPrefix + key, nil
}

// Get implements Store.
func (s *DirStore) Get(_ context.Context, key string) ([]byte, error) {
	clean, err := safeKey(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(clean)))
	if err != nil {
		return nil, fmt.Errorf("imagestore: read image: %w", err)
	}
	return data, nil
}

func safeKey(key string) (string, error) {
	key = strings.TrimPrefix(key, "/")
	if key == "" || strings.Contains(key, "..") {
		return "", fmt.Errorf("imagestore: invalid key")
	}
	return key, nil
}
