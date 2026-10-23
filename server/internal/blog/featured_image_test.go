package blog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"testing"

	"github.com/google/uuid"

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

func TestFallbackCover_IsACoverSizedPNG(t *testing.T) {
	b, err := fallbackCover("agentic-ai-in-production", "violet")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	if got := img.Bounds().Size(); got.X != coverWidth || got.Y != coverHeight {
		t.Fatalf("size = %v, want %dx%d", got, coverWidth, coverHeight)
	}
}

// Same article, same picture (republishing is stable); different articles,
// different pictures (no two listing cards alike); unknown accents still draw.
func TestFallbackCover_DeterministicAndUniquePerArticle(t *testing.T) {
	a1, _ := fallbackCover("blast-radius-analytics", "emerald")
	a2, _ := fallbackCover("blast-radius-analytics", "emerald")
	b, _ := fallbackCover("gateway-api-shifts-cluster-networking", "emerald")
	if !bytes.Equal(a1, a2) {
		t.Error("same seed drew different covers")
	}
	if bytes.Equal(a1, b) {
		t.Error("two different articles drew the same cover")
	}
	if _, err := fallbackCover("x", "no-such-accent"); err != nil {
		t.Errorf("unknown accent: %v", err)
	}
}
