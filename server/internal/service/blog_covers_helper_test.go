package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/imagestore"
)

// memCovers is an image store in memory, standing in for the ring's store so
// publishing can verify covers and file a fallback one.
type memCovers struct{ objects map[string][]byte }

func newMemCovers() *memCovers { return &memCovers{objects: map[string][]byte{}} }

func (m *memCovers) PutFile(_ context.Context, orgID uuid.UUID, data []byte, ext, _ string) (string, error) {
	key := fmt.Sprintf("%s/%d.%s", orgID, len(m.objects), ext)
	m.objects[key] = data
	return imagestore.URLPrefix + key, nil
}

func (m *memCovers) Get(_ context.Context, key string) ([]byte, error) {
	if d, ok := m.objects[key]; ok {
		return d, nil
	}
	return nil, errors.New("not found")
}

// publishingRunner is a runner for a ring that can file posts: a public origin
// for featured images and a store to verify and hold covers in, as in prod.
func publishingRunner(cms blog.CMSPublisher) *blog.Runner {
	return blog.NewRunner(blog.Config{PublicBaseURL: "https://jobshout.test"}, nil, cms, nil, zap.NewNop()).
		WithCoverStore(newMemCovers())
}
