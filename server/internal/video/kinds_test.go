package video

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/imagestore"
	"github.com/jobshout/server/internal/model"
)

func TestScriptPromptVariesByKind(t *testing.T) {
	base := Brief{Topic: "wireless earbuds", Seconds: 60, Format: "landscape", Voice: "af_heart"}

	product := base
	product.Kind = model.VideoKindProduct
	product.ProductName = "Acme Buds"
	p := scriptPrompt(product)
	if !strings.Contains(p, "promotional") || !strings.Contains(p, "Acme Buds") || !strings.Contains(p, `"product": true`) {
		t.Fatalf("product prompt missing product cues:\n%s", p)
	}

	anime := base
	anime.Kind = model.VideoKindAnime
	a := scriptPrompt(anime)
	if !strings.Contains(strings.ToLower(a), "anime") {
		t.Fatalf("anime prompt missing anime cue:\n%s", a)
	}

	e := scriptPrompt(base)
	if !strings.Contains(e, "narrated video about") {
		t.Fatalf("explainer prompt changed:\n%s", e)
	}
}

func TestParseScriptReadsProductFlag(t *testing.T) {
	reply := `{"title":"Buds","shots":[
		{"narration":"Meet the buds.","visual":"the earbuds on white","product":true},
		{"narration":"On a morning run.","visual":"a runner at dawn","shows_product":false},
		{"narration":"Crisp sound.","visual":"close up of the earbud","is_product":true},
		{"narration":"Plain.","visual":"a desk"}]}`
	script, err := ParseScript(reply)
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{true, false, true, false}
	if len(script.Shots) != len(want) {
		t.Fatalf("got %d shots, want %d", len(script.Shots), len(want))
	}
	for i, w := range want {
		if script.Shots[i].ShowsProduct != w {
			t.Errorf("shot %d ShowsProduct=%v, want %v", i, script.Shots[i].ShowsProduct, w)
		}
	}
}

func TestBriefFromValuesResolvesKindAndStyle(t *testing.T) {
	// Anime with no style falls back to the anime default.
	b := BriefFromValues(map[string]string{"topic": "robots", "kind": "anime"})
	if b.Kind != model.VideoKindAnime || b.Style != DefaultStyle(model.VideoKindAnime) {
		t.Fatalf("anime default style not applied: kind=%q style=%q", b.Kind, b.Style)
	}
	// An explicit style wins over the preset default.
	b = BriefFromValues(map[string]string{"topic": "robots", "kind": "anime", "style": "noir"})
	if b.Style != "noir" {
		t.Fatalf("explicit style overridden: %q", b.Style)
	}
	// Product keeps its name and photo URL; unknown kind becomes explainer.
	b = BriefFromValues(map[string]string{"topic": "buds", "kind": "product",
		"product_name": "Acme", "product_image": "/api/v1/images/file/x.png"})
	if b.Kind != model.VideoKindProduct || b.ProductName != "Acme" || b.ProductImage != "/api/v1/images/file/x.png" {
		t.Fatalf("product fields not resolved: %+v", b)
	}
	if b = BriefFromValues(map[string]string{"topic": "t", "kind": "nonsense"}); b.Kind != model.VideoKindExplainer {
		t.Fatalf("unknown kind not defaulted: %q", b.Kind)
	}
}

func TestProductImageIsOrgScoped(t *testing.T) {
	org := uuid.New()
	key := org.String() + "/2026/10/pic.jpg"
	store := &fakeStore{objects: map[string][]byte{key: []byte("JPEGBYTES")}}
	svc := NewService(fakeWriter{}, &fakeMedia{}, store, testConfig(), nil)

	// This org's image comes back base64-encoded.
	got, err := svc.productImage(context.Background(), org, imagestore.URLPrefix+key)
	if err != nil {
		t.Fatalf("own image: %v", err)
	}
	if got != base64.StdEncoding.EncodeToString([]byte("JPEGBYTES")) {
		t.Fatalf("unexpected encoding: %q", got)
	}

	// Another org's key is refused before any read.
	other := uuid.New().String() + "/2026/10/pic.jpg"
	store.objects[other] = []byte("SECRET")
	if _, err := svc.productImage(context.Background(), org, imagestore.URLPrefix+other); err == nil {
		t.Fatal("expected a cross-org key to be refused")
	}

	// A value that is not a stored-image URL is refused.
	if _, err := svc.productImage(context.Background(), org, "https://evil.example/x.png"); err == nil {
		t.Fatal("expected a non-store URL to be refused")
	}
}

// Each keyframe is drawn on its own, so a character's look is written into
// every shot that names them, not left to the writer to repeat.
func TestParseScriptWritesTheCastIntoEveryShot(t *testing.T) {
	in := `{"title": "Last Beacon",
	  "cast": [{"name": "Mira", "look": "a young woman engineer, short black bob, yellow raincoat."},
	           {"name": "Luna", "look": "a small silver robot fox with glowing blue eyes"}],
	  "shots": [
	    {"narration": "Mira reached the island.", "visual": "Wide shot of Mira on a stormy pier."},
	    {"narration": "Her fox kept watch.", "visual": "Close-up of LUNA beside mira at the door."},
	    {"narration": "The light came back.", "visual": "The lantern blazes over the sea."}]}`
	s, err := ParseScript(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Wide shot of Mira (a young woman engineer, short black bob, yellow raincoat) on a stormy pier.",
		"Close-up of LUNA (a small silver robot fox with glowing blue eyes) beside mira (a young woman engineer, short black bob, yellow raincoat) at the door.",
		"The lantern blazes over the sea.",
	}
	for i, w := range want {
		if s.Shots[i].Visual != w {
			t.Errorf("shot %d:\n got %q\nwant %q", i, s.Shots[i].Visual, w)
		}
	}
}

// A shot's narration target is its share of the length: 30 s across 5 shots
// is about 15 words a shot, not anywhere in a fixed 8-20.
func TestScriptPromptGivesEachShotItsShareOfTheWords(t *testing.T) {
	p := scriptPrompt(Brief{Topic: "lighthouses", Seconds: 30, Kind: model.VideoKindAnime, Voice: "af_heart"})
	if !strings.Contains(p, "12 to 18 words") || strings.Contains(p, "{WORDS}") || !strings.Contains(p, `"cast"`) {
		t.Fatalf("prompt:\n%s", p)
	}
}
