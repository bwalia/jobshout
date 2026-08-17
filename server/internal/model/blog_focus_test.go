package model

import (
	"encoding/json"
	"testing"
)

// Focus areas come from a text box, so blanks and repeats are ordinary input.
func TestNormalizeCleansFocusAreas(t *testing.T) {
	req := GenerateBlogRequest{
		Trending: true,
		Focus:    []string{" Postgres ", "", "kubernetes", "Postgres", "\t", "Kubernetes"},
	}

	req.Normalize()

	want := []string{"Postgres", "kubernetes"}
	if len(req.Focus) != len(want) {
		t.Fatalf("focus = %v, want %v", req.Focus, want)
	}
	for i := range want {
		if req.Focus[i] != want[i] {
			t.Errorf("focus[%d] = %q, want %q", i, req.Focus[i], want[i])
		}
	}
}

// Normalize runs on the way in and again in the service, so it has to be safe
// to run twice.
func TestNormalizeFocusIsIdempotent(t *testing.T) {
	req := GenerateBlogRequest{Trending: true, Focus: []string{"Postgres", " Postgres "}}

	req.Normalize()
	first := append([]string(nil), req.Focus...)
	req.Normalize()

	if len(req.Focus) != len(first) {
		t.Errorf("second Normalize changed focus: %v then %v", first, req.Focus)
	}
}

func TestValidateAcceptsFocusOnATrendingRun(t *testing.T) {
	req := GenerateBlogRequest{Trending: true, Focus: []string{"Postgres"}}
	req.Normalize()

	if err := req.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// Rejected rather than ignored: a silently dropped focus area would look like
// it was applied while the run wrote about anything at all.
func TestValidateRejectsFocusWithoutTrending(t *testing.T) {
	req := GenerateBlogRequest{
		Briefs: []BlogBrief{{Topic: "Something specific"}},
		Focus:  []string{"Postgres"},
	}
	req.Normalize()

	err := req.Validate()
	if err == nil {
		t.Fatal("Validate accepted focus areas on a non-trending run")
	}
}

// The scheduler stores the request as JSON and decodes it on every fire, so the
// new fields have to survive that round trip.
func TestGenerateBlogRequestRoundTripsThroughScheduledInput(t *testing.T) {
	stored := map[string]any{
		"trending":       true,
		"trending_count": 2,
		"focus":          []any{"Postgres", "Kubernetes networking"},
		"auto_publish":   true,
	}

	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	var req GenerateBlogRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !req.Trending {
		t.Error("trending was lost")
	}
	if req.TrendingCount != 2 {
		t.Errorf("trending_count = %d, want 2", req.TrendingCount)
	}
	if len(req.Focus) != 2 || req.Focus[0] != "Postgres" {
		t.Errorf("focus = %v", req.Focus)
	}
	if !req.AutoPublish {
		t.Error("auto_publish was lost")
	}

	req.Normalize()
	if err := req.Validate(); err != nil {
		t.Errorf("a stored schedule failed validation: %v", err)
	}
}

// A schedule created before these fields existed must still decode and run.
func TestGenerateBlogRequestAcceptsOlderStoredInput(t *testing.T) {
	raw := []byte(`{"trending":true,"trending_count":1}`)

	var req GenerateBlogRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	req.Normalize()

	if err := req.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if len(req.Focus) != 0 || req.AutoPublish {
		t.Errorf("absent fields should stay zero: focus=%v auto=%v", req.Focus, req.AutoPublish)
	}
}
