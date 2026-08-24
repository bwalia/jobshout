package platformtools

import (
	"testing"

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
