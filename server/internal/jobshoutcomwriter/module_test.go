package jobshoutcomwriter

import (
	"testing"

	"github.com/jobshout/server/internal/audience"
	"github.com/jobshout/server/internal/model"
)

func TestRequestWithoutTopicDiscoversTrending(t *testing.T) {
	req := Request(map[string]string{})
	req.Normalize()
	if err := req.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !req.Trending || req.TrendingCount != 1 {
		t.Fatalf("want one trending article, got trending=%v count=%d", req.Trending, req.TrendingCount)
	}
	if len(req.Focus) == 0 {
		t.Fatal("want the default Insights focus areas")
	}
	if req.Writer != model.BuiltinJobShoutComWriter || req.Audience != audience.InsightsKey {
		t.Fatalf("want writer %q audience %q, got %q %q", model.BuiltinJobShoutComWriter, audience.InsightsKey, req.Writer, req.Audience)
	}
	if !req.AutoPublish {
		t.Fatal("a run should file its CMS draft on its own")
	}
}

func TestRequestWithTopicWritesThatTopic(t *testing.T) {
	req := Request(map[string]string{"topic": " Jev and decision models ", "context": "start from the TypeSafe post", "focus": "ignored"})
	req.Normalize()
	if err := req.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if req.Trending {
		t.Fatal("a named topic is not a trending run")
	}
	if len(req.Briefs) != 1 || req.Briefs[0].Topic != "Jev and decision models" || req.Briefs[0].Audience != audience.InsightsKey {
		t.Fatalf("unexpected briefs: %+v", req.Briefs)
	}
}

func TestInsightsProfileIsHiddenFromPickers(t *testing.T) {
	for _, o := range audience.Options() {
		if o.Value == audience.InsightsKey {
			t.Fatal("the Insights profile must not appear in the Article Writer's picker")
		}
	}
	if !audience.Known(audience.InsightsKey) || audience.For(audience.InsightsKey).Key != audience.InsightsKey {
		t.Fatal("the Insights profile must still resolve")
	}
}

func TestSeedIsMarkedBuiltin(t *testing.T) {
	a := Seed([16]byte{1})
	if a.Metadata[model.MetadataKeyBuiltin] != model.BuiltinJobShoutComWriter || a.Name != model.AgentNameJobShoutComWriter {
		t.Fatalf("unexpected seed: %+v", a)
	}
}
