package platformtools

import (
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func TestImageRef_CarriesURL(t *testing.T) {
	ref := imageRef("abc", "/api/v1/images/file/xyz")
	if ref.Kind != model.EntityImage {
		t.Fatalf("kind = %q", ref.Kind)
	}
	if ref.URL != "/api/v1/images/file/xyz" {
		t.Fatalf("url = %q", ref.URL)
	}
	if ref.Href != "/images" {
		t.Fatalf("href = %q", ref.Href)
	}
}

func TestReviewRef_LinksToReviewPage(t *testing.T) {
	run := model.ReviewRun{ID: uuid.UUID{1}, Repo: "bwalia/jobshout", PRNumber: 84}
	ref := reviewRef(run)
	if ref.Kind != model.EntityReviewRun {
		t.Fatalf("kind = %q", ref.Kind)
	}
	if ref.Label != "bwalia/jobshout#84" {
		t.Fatalf("label = %q", ref.Label)
	}
	if ref.Href != "/agents/review" {
		t.Fatalf("href = %q", ref.Href)
	}
}
