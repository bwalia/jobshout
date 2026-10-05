package companyweb

import (
	"context"
	"testing"

	"github.com/jobshout/server/internal/research"
)

type stubSearch struct {
	hits []research.Source
	err  error
}

func (s stubSearch) Available() bool { return true }

func (s stubSearch) Search(context.Context, string, int) ([]research.Source, error) {
	return s.hits, s.err
}

func TestLookupRanksHomepageOverLinkedIn(t *testing.T) {
	hits := []research.Source{
		{URL: "https://www.linkedin.com/company/acme", Title: "Acme | LinkedIn", Site: "linkedin.com"},
		{URL: "https://www.crunchbase.com/organization/acme", Title: "Acme - Crunchbase", Site: "crunchbase.com"},
		{URL: "https://acme.com/", Title: "Acme — Official Site", Site: "acme.com"},
		{URL: "https://news.example.com/acme-raises-funding", Title: "Acme raises funding", Site: "news.example.com"},
	}
	results, err := Lookup(context.Background(), stubSearch{hits: hits}, "Acme Ltd")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Site != "acme.com" {
		t.Fatalf("top site = %q, want acme.com (got %#v)", results[0].Site, results)
	}
	for _, r := range results {
		if r.Site == "linkedin.com" || r.Site == "crunchbase.com" {
			t.Fatalf("directory/social site leaked through: %s", r.Site)
		}
	}
}

func TestLookupRequiresName(t *testing.T) {
	_, err := Lookup(context.Background(), stubSearch{}, "  ")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestTokensDropLegalSuffix(t *testing.T) {
	got := tokens("The Acme Corporation Ltd")
	for _, tkn := range got {
		if tkn == "ltd" || tkn == "corporation" || tkn == "the" {
			t.Fatalf("unexpected token %q in %v", tkn, got)
		}
	}
	if len(got) == 0 || got[0] != "acme" {
		t.Fatalf("tokens = %v, want acme first", got)
	}
}
