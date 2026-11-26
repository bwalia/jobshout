package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// cardCases are the shapes a card takes: a figure with steps, a figure with a
// trend, a headline alone, a title alone, and a title Poppins' Latin cut
// cannot set. CARD_PREVIEW_DIR=/some/dir writes them out to look at.
var cardCases = map[string]struct {
	scene coverScene
	title string
}{
	"steps": {coverScene{Eyebrow: "NON-RESIDENT LANDLORD", Stat: "100%", Headline: "of the rent, paid gross",
		Labels: []string{"Tenant pays agent", "Agent deducts tax", "You receive net rent", "File Self Assessment"}},
		"Tax on UK rental income when you live abroad"},
	"trend": {coverScene{Eyebrow: "12–23 NOVEMBER 2026", Stat: "$35 billion", Headline: "dollar baby",
		Trend: []float64{100, 96, 92, 91, 89, 87, 88, 89, 90, 91, 92, 120}, TrendFrom: "12 Nov · resigns", TrendTo: "23 Nov · back as CEO"},
		"Jensen Huang's 11 days away from NVIDIA"},
	"headline": {coverScene{Eyebrow: "KUBERNETES", Headline: "Gateway API replaces Ingress",
		Labels: []string{"Ingress", "HTTPRoute", "Gateway", "GatewayClass"}}, "Gateway API Replaces Ingress"},
	"title":    {coverScene{}, "A title with nothing else to go on"},
	"nonLatin": {coverScene{Headline: "Ταχύτερη αναζήτηση"}, "Αναζήτηση με GPU"},
}

func TestDrawCard_DrawsEveryShape(t *testing.T) {
	dir := os.Getenv("CARD_PREVIEW_DIR")
	seen := map[string]string{}
	for name, c := range cardCases {
		b, err := drawCard(c.scene, c.title, "amber")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil || img.Bounds().Dx() != int(designW) || img.Bounds().Dy() != int(designH) {
			t.Fatalf("%s: not a %vx%v PNG: %v", name, designW, designH, err)
		}
		if other, dup := seen[string(b)]; dup {
			t.Errorf("%s drew the same card as %s", name, other)
		}
		seen[string(b)] = name
		if dir != "" {
			_ = os.WriteFile(filepath.Join(dir, "card-"+name+".png"), b, 0o644)
		}
	}
}

// A card cover is typeset, so no image model is asked for anything: a model
// that letters a slip or a calendar is what sent prod's tax cover out bare.
func TestDrawCover_CardNeedsNoImageModel(t *testing.T) {
	ill := &fakeIllustrator{enabled: true}
	r := newTestRunner(&fakeCMS{})
	r.cfg.CoverStyle = CoverStyleCard
	r.WithIllustrator(ill)
	a := &GeneratedArticle{Title: "Tax on UK rental income when you live abroad", Slug: "tax",
		CoverScene: cardCases["steps"].scene}
	a.CoverScene.Layout = layoutFlow

	if err := r.drawCover(context.Background(), uuid.New(), a); err != nil {
		t.Fatal(err)
	}
	if len(ill.calls) != 0 {
		t.Fatalf("a card asked the image model for %d pictures", len(ill.calls))
	}
	if a.CoverImageProvider != "diagram" || a.CoverImageModel != "card" || !r.coverStored(context.Background(), a.CoverImageURL) {
		t.Fatalf("cover=%q provider=%q model=%q", a.CoverImageURL, a.CoverImageProvider, a.CoverImageModel)
	}
}

// The plan's card fields arrive in whatever shape the model gives them; a
// trend too short to draw a line is dropped rather than drawn as a dot.
func TestCoverScene_ReadsTheCardFields(t *testing.T) {
	var s coverScene
	if err := json.Unmarshal([]byte(`{"layout":"flow","stat":"31%","headline":"best day since 2003",
		"trend":[100,"96","$1,200","31%","n/a"],"trend_from":"12 Nov","trend_to":"23 Nov"}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Headline != "best day since 2003" || s.TrendFrom != "12 Nov" || s.TrendTo != "23 Nov" {
		t.Fatalf("card fields lost: %+v", s)
	}
	if want := []float64{100, 96, 1200, 31}; len(s.Trend) != len(want) || s.Trend[2] != 1200 || s.Trend[3] != 31 {
		t.Fatalf("trend = %v, want %v", s.Trend, want)
	}
	short := coverSceneFor(&GeneratedArticle{Title: "T", CoverScene: coverScene{Layout: layoutFlow, Trend: []float64{1, 2}}})
	if short.Trend != nil {
		t.Fatalf("a two-point trend was kept: %v", short.Trend)
	}
}
