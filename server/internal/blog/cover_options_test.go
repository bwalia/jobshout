package blog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/imagestore"
)

type altCovers struct{ data map[string][]byte }

func (m altCovers) PutFile(context.Context, uuid.UUID, []byte, string, string) (string, error) {
	return "", nil
}
func (m altCovers) Get(_ context.Context, key string) ([]byte, error) {
	if b, ok := m.data[key]; ok {
		return b, nil
	}
	return nil, errors.New("missing")
}

type fakeDescriber struct {
	out string
	err error
}

func (f fakeDescriber) DescribeImage(context.Context, string, []byte) (string, error) {
	return f.out, f.err
}

func altRunner(d CoverDescriber) *Runner {
	return &Runner{
		covers:    altCovers{data: map[string][]byte{"k1": []byte("png")}},
		describer: d,
	}
}

func TestDescribeCover_UsesVisionText(t *testing.T) {
	a := &GeneratedArticle{Title: "T", CoverImageURL: imagestore.URLPrefix + "k1"}
	altRunner(fakeDescriber{out: ` "A glowing server rack at dusk."  `}).describeCover(context.Background(), a)
	if a.CoverImageAlt != "A glowing server rack at dusk." {
		t.Errorf("alt = %q", a.CoverImageAlt)
	}
}

func TestDescribeCover_FallsBackToTitle(t *testing.T) {
	for name, r := range map[string]*Runner{
		"vision fails":  altRunner(fakeDescriber{err: errors.New("down")}),
		"no describer":  altRunner(nil),
		"empty answer":  altRunner(fakeDescriber{out: "  "}),
		"cover missing": altRunner(fakeDescriber{out: "x"}),
	} {
		a := &GeneratedArticle{Title: "Zero Trust", CoverImageURL: imagestore.URLPrefix + "k1"}
		if name == "cover missing" {
			a.CoverImageURL = imagestore.URLPrefix + "gone"
		}
		r.describeCover(context.Background(), a)
		if a.CoverImageAlt != "Cover illustration for: Zero Trust" {
			t.Errorf("%s: alt = %q", name, a.CoverImageAlt)
		}
	}
}

func TestDescribeCover_Truncates(t *testing.T) {
	a := &GeneratedArticle{Title: "T", CoverImageURL: imagestore.URLPrefix + "k1"}
	altRunner(fakeDescriber{out: strings.Repeat("a", 500)}).describeCover(context.Background(), a)
	if len([]rune(a.CoverImageAlt)) != maxAltRunes {
		t.Errorf("len = %d", len(a.CoverImageAlt))
	}
}

func TestImageChoice_RoundTrip(t *testing.T) {
	if _, ok := ImageChoiceFrom(WithImageChoice(context.Background(), ImageChoice{})); ok {
		t.Error("an empty choice must not be stored")
	}
	c, ok := ImageChoiceFrom(WithImageChoice(context.Background(), ImageChoice{Provider: "flux", Model: "m"}))
	if !ok || c.Provider != "flux" || c.Model != "m" {
		t.Errorf("got %+v, %v", c, ok)
	}
}

func TestArtPromptStyled_PresetReplacesHouseStyleButKeepsNoText(t *testing.T) {
	scene := coverScene{Picture: "a radiologist reading scans"}
	def := artPrompt(scene, "copper")
	if !strings.Contains(def, "isometric 3D") {
		t.Fatalf("default lost the house style: %s", def)
	}
	got := artPromptStyled(scene, "copper", "flat illustration, warm colours, space on the left for a title.")
	if strings.Contains(got, "isometric") || !strings.Contains(got, "flat illustration, warm colours") {
		t.Errorf("preset not applied: %s", got)
	}
	if !strings.Contains(got, "No text") || !strings.Contains(got, "radiologist") {
		t.Errorf("preset dropped the no-text rule or subject: %s", got)
	}
}
