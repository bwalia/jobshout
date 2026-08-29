package mail

import (
	"errors"
	"fmt"
	"testing"
)

func TestSanitizeKnowledgeURLsKeepsHTTPAndDropsEmpty(t *testing.T) {
	got, err := SanitizeKnowledgeURLs([]string{
		"  https://example.com/pricing  ",
		"",
		"   ",
		"http://docs.example.com/sla",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d urls, want 2: %v", len(got), got)
	}
	if got[0] != "https://example.com/pricing" {
		t.Errorf("got %q", got[0])
	}
}

func TestSanitizeKnowledgeURLsRejectsJavascriptAndData(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "data:text/html,hi"} {
		_, err := SanitizeKnowledgeURLs([]string{raw})
		if !errors.Is(err, ErrInvalidKnowledgeURL) {
			t.Errorf("%q: want ErrInvalidKnowledgeURL, got %v", raw, err)
		}
	}
}

func TestSanitizeKnowledgeURLsRejectsOtherSchemes(t *testing.T) {
	_, err := SanitizeKnowledgeURLs([]string{"ftp://files.example.com/a"})
	if !errors.Is(err, ErrInvalidKnowledgeURL) {
		t.Fatalf("ftp should be rejected, got %v", err)
	}
}

func TestSanitizeKnowledgeURLsCapsAt20(t *testing.T) {
	in := make([]string, 25)
	for i := range in {
		in[i] = fmt.Sprintf("https://example.com/p/%d", i)
	}
	got, err := SanitizeKnowledgeURLs(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxKnowledgeURLs {
		t.Fatalf("got %d, want %d", len(got), MaxKnowledgeURLs)
	}
}

func TestSanitizeKnowledgeURLsDedupe(t *testing.T) {
	got, err := SanitizeKnowledgeURLs([]string{
		"https://example.com/a",
		"https://example.com/a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}

func TestExtractInboundURLsKeepsProductLink(t *testing.T) {
	got := ExtractInboundURLs(
		"Price of this machine?",
		"Do you have this in stock? https://vendor.example/machine-x Thanks.",
	)
	if len(got) != 1 || got[0] != "https://vendor.example/machine-x" {
		t.Fatalf("got %v", got)
	}
}

func TestExtractInboundURLsSkipsTracking(t *testing.T) {
	got := ExtractInboundURLs("", "Click https://list-manage.com/unsub/abc or https://vendor.example/p")
	if len(got) != 1 || got[0] != "https://vendor.example/p" {
		t.Fatalf("got %v", got)
	}
}

func TestMergeKnowledgeURLsPlaybookFirst(t *testing.T) {
	got := MergeKnowledgeURLs(
		[]string{"https://example.com/pricing"},
		[]string{"https://vendor.example/machine-x", "https://example.com/pricing"},
	)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if got[0] != "https://example.com/pricing" || got[1] != "https://vendor.example/machine-x" {
		t.Fatalf("order %v", got)
	}
}
