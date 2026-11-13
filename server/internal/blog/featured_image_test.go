package blog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"strings"
	"testing"
	"time"

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

// Repairing a coverless post must leave it with a featured image even when
// pictures are unavailable — the state every prod post was in. The fallback
// is the article's diagram, which needs no image service.
func TestRegenerateCover_CoverlessFallsBackWhenDrawingUnavailable(t *testing.T) {
	r := newTestRunner(&fakeCMS{}) // no illustrator: pictures are off
	r.cfg.CoverStyle = CoverStylePicture
	a := &GeneratedArticle{Title: "T", Slug: "t", Topic: "t"}

	if err := r.RegenerateCover(context.Background(), uuid.New(), a); err != nil {
		t.Fatalf("a coverless article must get the fallback, got %v", err)
	}
	if a.CoverImageProvider != "diagram" || !r.coverStored(context.Background(), a.CoverImageURL) {
		t.Fatalf("no stored fallback cover: provider=%q url=%q", a.CoverImageProvider, a.CoverImageURL)
	}
}

// A redraw whose picture fails (ERNIE's lettering still wrong) draws the same
// scene as a diagram rather than keeping the old cover, typos and all.
func TestRegenerateCover_FailedPictureBecomesItsDiagram(t *testing.T) {
	r := newTestRunner(&fakeCMS{})
	r.cfg.CoverStyle = CoverStylePicture
	r.WithIllustrator(&fakeIllustrator{enabled: true, failAfter: -1})
	defer func(f func(int) time.Duration) { coverRetryWaitFn = f }(coverRetryWaitFn)
	coverRetryWaitFn = func(int) time.Duration { return 0 }
	old := "/api/v1/images/file/org/2026/10/real.png"
	r.covers.(*memCovers).objects["org/2026/10/real.png"] = []byte("png")
	a := &GeneratedArticle{Title: "T", Slug: "t", CoverImageURL: old}

	if err := r.RegenerateCover(context.Background(), uuid.New(), a); err != nil {
		t.Fatalf("the diagram should stand in: %v", err)
	}
	if a.CoverImageURL == old || a.CoverImageProvider != "diagram" || !r.coverStored(context.Background(), a.CoverImageURL) {
		t.Fatalf("cover=%q provider=%q", a.CoverImageURL, a.CoverImageProvider)
	}
	if !strings.Contains(a.CoverNote, "diagram alone") {
		t.Fatalf("no note says why the cover is a plain diagram: %q", a.CoverNote)
	}
}

// ERNIE misspelt a five-step cover three times in three on int. With no model
// that letters labels, it draws only the picture, with no words, and the
// cover's diagram is lettered over it here.
func TestDrawCover_ErnieDrawsThePictureUnderTheDiagram(t *testing.T) {
	ernie := &captionIllustrator{fakeIllustrator{enabled: true, provider: "mflux", model: "ernie-image-turbo"}}
	r := newTestRunner(&fakeCMS{})
	r.cfg.CoverStyle = CoverStylePicture
	r.WithIllustrator(ernie)
	a := &GeneratedArticle{Title: "Preemptive security for ingress", Slug: "ingress",
		CoverScene: coverScene{Layout: layoutFlow, Labels: []string{"HTTP request", "auth", "rate limit", "proxy to pod"}, Eyebrow: "API security"}}

	if err := r.drawCover(context.Background(), uuid.New(), a); err != nil {
		t.Fatal(err)
	}
	if len(ernie.calls) != 1 || !ernie.calls[0].NoText || len(ernie.calls[0].ExpectText) != 0 {
		t.Fatalf("want one picture with no words, got %+v", ernie.calls)
	}
	if !strings.Contains(ernie.calls[0].Prompt, "api security: http request, auth, rate limit, proxy to pod") {
		t.Fatalf("the picture is not of the cover's things:\n%s", ernie.calls[0].Prompt)
	}
	if a.CoverImageProvider != "mflux" || a.CoverImageModel != "ernie-image-turbo+scene:flow" || a.CoverNote != "" ||
		a.CoverImageWidth != int(designW) || !r.coverStored(context.Background(), a.CoverImageURL) {
		t.Fatalf("cover=%q provider=%q model=%q note=%q", a.CoverImageURL, a.CoverImageProvider, a.CoverImageModel, a.CoverNote)
	}
}
