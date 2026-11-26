package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"

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
	"versus": {coverScene{Layout: layoutVersus, Eyebrow: "NON-RESIDENT LANDLORDS", Headline: "Gross rent or net: the choice",
		Labels: []string{"Full monthly rent", "Tax deducted upfront"},
		Notes:  map[string]string{"full monthly rent": "Paid gross once HMRC approves you", "tax deducted upfront": "Agent keeps 20% for HMRC"},
		Dek:    "Living abroad, your letting agent holds back basic-rate tax unless HMRC lets you take the rent gross."},
		"Tax on UK rental income when you live abroad"},
	"notes": {coverScene{Eyebrow: "NON-RESIDENT LANDLORD", Stat: "20%", Headline: "held back from every rent payment",
		Labels: []string{"Tenant pays agent", "Agent deducts tax", "You file Self Assessment"},
		Notes:  map[string]string{"tenant pays agent": "Full rent reaches your UK agent", "agent deducts tax": "Basic rate goes to HMRC quarterly"}},
		"Tax on UK rental income when you live abroad"},
	"facts": {coverScene{Layout: layoutVersus, Eyebrow: "NON-RESIDENT LANDLORDS", Headline: "Gross rent or net: the choice",
		Labels: []string{"Full monthly rent", "Tax deducted upfront"},
		Notes:  map[string]string{"full monthly rent": "Paid gross once HMRC approves you", "tax deducted upfront": "Agent keeps 20% for HMRC"},
		Dek:    "Living abroad, your letting agent holds back basic-rate tax unless HMRC lets you take the rent gross.",
		Facts:  []coverFact{{"20%", "basic-rate tax withheld"}, {"NRL1", "form to get rent gross"}, {"31 January", "Self Assessment deadline"}}},
		"Tax on UK rental income when you live abroad"},
	"prod":     {prodTaxScene, "Tax on UK rental income when you live abroad"},
	"title":    {coverScene{}, "A title with nothing else to go on"},
	"nonLatin": {coverScene{Headline: "Ταχύτερη αναζήτηση"}, "Αναζήτηση με GPU"},
}

// prodTaxScene is what Claude planned for prod's tax cover on v1.0.251, which
// cut its dek and both notes to "…".
var prodTaxScene = coverScene{Layout: layoutVersus, Eyebrow: "NON-RESIDENT LANDLORDS", Headline: "Two ways to receive UK rental income",
	Labels: []string{"Rent in full monthly", "Tax deducted upfront"},
	Notes: map[string]string{"rent in full monthly": "Approved via form NRL1i; landlord pays tax by 31 January",
		"tax deducted upfront": "Agent deducts basic-rate tax; submitted to HMRC quarterly"},
	Dek:   "Non-resident landlords can choose between receiving rent with tax deducted by their agent, or applying to HMRC for approval to receive the full amount",
	Facts: []coverFact{{"basic rate", "income tax deducted from rent"}, {"31 January", "Self Assessment return deadline"}}}

// The card's dek and a versus panel's notes are set whole, not cut to "…".
func TestDrawCard_SetsDekAndNotesWhole(t *testing.T) {
	if err := cardFonts(); err != nil {
		t.Fatal(err)
	}
	p := &pen{dc: gg.NewContext(1, 1), faces: map[string]font.Face{}, k: cardScale, bold: cardBold, medium: cardMedium}
	check := func(s string, w, max, min float64, n int) {
		if _, ls := p.fitLines(s, w, max, min, n, false); strings.HasSuffix(ls[len(ls)-1], "…") {
			t.Errorf("%q cut to %q", s, ls)
		}
	}
	check(prodTaxScene.Dek, 770, 36, 26, 3)
	for _, n := range prodTaxScene.Notes {
		check(n, chartR-chartL-80, 28, 20, 2)
	}
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
		if w, h := int(designW*cardScale), int(designH*cardScale); err != nil || img.Bounds().Dx() != w || img.Bounds().Dy() != h {
			t.Fatalf("%s: not a %vx%v PNG: %v", name, w, h, err)
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
	if a.CoverImageProvider != "diagram" || a.CoverImageModel != "card" || a.CoverImageWidth != int(designW*cardScale) ||
		!r.coverStored(context.Background(), a.CoverImageURL) {
		t.Fatalf("cover=%q provider=%q model=%q width=%d", a.CoverImageURL, a.CoverImageProvider, a.CoverImageModel, a.CoverImageWidth)
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
	// Notes come as a list beside the labels and are kept by label, so a
	// label dropped later cannot shift the rest onto the wrong notes.
	if err := json.Unmarshal([]byte(`{"layout":"versus","labels":["Gross rent","Net rent"],"notes":["HMRC approved","20% withheld"],"dek":"Two ways to be paid."}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.note("net rent") != "20% withheld" || s.note("Gross rent") != "HMRC approved" || s.Dek != "Two ways to be paid." {
		t.Fatalf("notes/dek lost: %+v", s)
	}
	stored, _ := json.Marshal(s)
	var back coverScene
	if err := json.Unmarshal(stored, &back); err != nil || back.note("Net rent") != "20% withheld" {
		t.Fatalf("notes lost on a round trip: %s -> %+v", stored, back)
	}
	if err := json.Unmarshal([]byte(`{"labels":["a","b"],"notes":["only one"]}`), &s); err != nil || s.Notes != nil {
		t.Fatalf("notes that do not pair with the labels were kept: %+v", s.Notes)
	}
	// A fact is drawn only when the article states its figure, and never
	// repeats the stat; they arrive as objects or as "figure: label".
	if err := json.Unmarshal([]byte(`{"layout":"versus","labels":["a","b"],"stat":"20%","facts":[
		{"figure":"20%","label":"basic rate"},{"value":"NRL1","caption":"form to get rent gross"},
		"31 January: filing deadline",{"figure":"£9,999","label":"made up"},{"figure":2026,"label":"tax year"}]}`), &s); err != nil {
		t.Fatal(err)
	}
	got := coverSceneFor(&GeneratedArticle{Title: "T", CoverScene: s,
		Markdown: "Agents keep 20% unless you send form NRL1. File by 31 January for the 2026 return."}).Facts
	want := []coverFact{{"NRL1", "form to get rent gross"}, {"31 January", "filing deadline"}, {"2026", "tax year"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("facts = %+v, want %+v", got, want)
	}
	short := coverSceneFor(&GeneratedArticle{Title: "T", CoverScene: coverScene{Layout: layoutFlow, Trend: []float64{1, 2}}})
	if short.Trend != nil {
		t.Fatalf("a two-point trend was kept: %v", short.Trend)
	}
}
