package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// A schedule sets the reader once, on the run. Every brief it produces has to
// inherit it, or discovery chooses topics for managers and the writer writes
// them up for engineers.
func TestNormalizeInheritsTheRunsReaderOntoEveryBrief(t *testing.T) {
	req := GenerateBlogRequest{
		Audience: "business",
		Industry: "  NHS trusts  ",
		Briefs:   []BlogBrief{{Topic: "Gateway API"}, {Topic: "Postgres 18"}},
		Topics:   []string{"Legacy topic"},
	}
	req.Normalize()

	if req.Industry != "NHS trusts" {
		t.Errorf("industry = %q; want it trimmed", req.Industry)
	}
	if len(req.Briefs) != 3 {
		t.Fatalf("briefs = %d; want 3", len(req.Briefs))
	}
	for _, b := range req.Briefs {
		if b.Audience != "business" || b.Industry != "NHS trusts" {
			t.Errorf("brief %q did not inherit the run: audience=%q industry=%q",
				b.Topic, b.Audience, b.Industry)
		}
	}
}

// A brief may name its own reader, which is what makes "write this story for
// both audiences" one request rather than two.
func TestBriefOverridesTheRunsReader(t *testing.T) {
	req := GenerateBlogRequest{
		Audience: "business",
		Briefs: []BlogBrief{
			{Topic: "Gateway API"},
			{Topic: "Gateway API for operators", Audience: "technote", Industry: "3PL logistics"},
		},
	}
	req.Normalize()

	if req.Briefs[0].Audience != "business" {
		t.Errorf("brief 1 audience = %q; want the run's", req.Briefs[0].Audience)
	}
	if req.Briefs[1].Audience != "technote" || req.Briefs[1].Industry != "3PL logistics" {
		t.Errorf("brief 2 did not keep its own reader: %+v", req.Briefs[1])
	}
}

// The default profile is stored as empty, so a run written today with the
// default chosen matches every run written before audiences existed.
func TestTheDefaultReaderIsStoredAsEmpty(t *testing.T) {
	req := GenerateBlogRequest{Audience: "developer", Briefs: []BlogBrief{{Topic: "t"}}}
	req.Normalize()
	if req.Audience != "" || req.Briefs[0].Audience != "" {
		t.Errorf("the default was stored as %q / %q; want empty",
			req.Audience, req.Briefs[0].Audience)
	}
}

// Normalize is called on the way in and again on the retry path, so it has to
// be idempotent for the new fields as well as the old ones.
func TestNormalizeIsIdempotentWithAReader(t *testing.T) {
	req := GenerateBlogRequest{Audience: "business", Briefs: []BlogBrief{{Topic: "t"}}}
	req.Normalize()
	first := req.Briefs
	req.Normalize()
	if len(req.Briefs) != len(first) {
		t.Fatalf("second Normalize changed the brief count: %d then %d", len(first), len(req.Briefs))
	}
	if req.Briefs[0].Audience != "business" {
		t.Errorf("second Normalize dropped the reader: %+v", req.Briefs[0])
	}
}

// A typo'd audience is rejected. Quietly writing a developer article instead
// would look exactly like a schedule working correctly, and nobody checks a
// nightly job that is producing articles.
func TestValidateRejectsAnUnknownReader(t *testing.T) {
	req := GenerateBlogRequest{Audience: "manager", Briefs: []BlogBrief{{Topic: "t"}}}
	err := req.Validate()
	if err == nil {
		t.Fatal("an unknown audience was accepted")
	}
	// The message has to say what is allowed — it is the only thing the owner
	// of a paused schedule has to go on.
	if !strings.Contains(err.Error(), "business") {
		t.Errorf("error does not list the valid readers: %v", err)
	}

	perBrief := GenerateBlogRequest{Briefs: []BlogBrief{{Topic: "t", Audience: "exec"}}}
	if perBrief.Validate() == nil {
		t.Error("an unknown audience on a brief was accepted")
	}
}

// Validate runs before Normalize on the HTTP path as well as after, so a known
// reader in any casing must pass.
func TestValidateAcceptsKnownReaders(t *testing.T) {
	for _, key := range []string{"", "developer", "technote", "BUSINESS"} {
		req := GenerateBlogRequest{Audience: key, Briefs: []BlogBrief{{Topic: "t"}}}
		if err := req.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v; want nil", key, err)
		}
	}
}

// The reader is carried on the run's options, which is what a retry replays —
// and for a trending run, whose briefs do not exist until discovery has run,
// the options are the only record of who asked for what.
func TestRunOptionsCarryTheReader(t *testing.T) {
	req := GenerateBlogRequest{Trending: true, Audience: "business", Industry: "NHS trusts"}
	req.Normalize()
	opts := req.RunOptions()
	if opts.Audience != "business" || opts.Industry != "NHS trusts" {
		t.Errorf("run options = %+v; want the reader and the sector", opts)
	}

	// And they survive the round trip through the stored jsonb column.
	raw, err := json.Marshal(opts)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back BlogRunOptions
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Audience != "business" || back.Industry != "NHS trusts" {
		t.Errorf("round trip lost the reader: %+v", back)
	}
}

// A schedule is stored as input_json and decoded straight into the request, so
// the wire names are part of the contract.
func TestScheduledInputCarriesTheReader(t *testing.T) {
	const input = `{"trending":true,"trending_count":2,"focus":["Kubernetes"],` +
		`"audience":"business","industry":"NHS trusts"}`
	var req GenerateBlogRequest
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	req.Normalize()
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if req.Audience != "business" || req.Industry != "NHS trusts" {
		t.Errorf("schedule input did not carry the reader: %+v", req)
	}
}

// A brief naming the default outright is a choice, not silence — so it beats
// the run's reader rather than inheriting it.
func TestBriefCanChooseTheDefaultAgainstTheRun(t *testing.T) {
	req := GenerateBlogRequest{
		Audience: "business",
		Briefs:   []BlogBrief{{Topic: "t", Audience: "developer"}},
	}
	req.Normalize()
	if req.Briefs[0].Audience != "" {
		t.Errorf("brief audience = %q; want the default, not the run's business",
			req.Briefs[0].Audience)
	}
}

// Normalize runs before Validate on both the HTTP path and the scheduler path,
// so a typo has to survive it or nothing ever rejects the schedule.
func TestATypoSurvivesNormalizeAndIsRejected(t *testing.T) {
	req := GenerateBlogRequest{Audience: "manger", Briefs: []BlogBrief{{Topic: "t"}}}
	req.Normalize()
	if err := req.Validate(); err == nil {
		t.Fatal("a typo'd audience survived Normalize and then Validate accepted it")
	}
}
