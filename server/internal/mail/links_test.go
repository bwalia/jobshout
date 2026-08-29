package mail

import "testing"

func TestSenderLinksFindsProductLink(t *testing.T) {
	body := "Hi,\n\nWhat is the price of this machine? https://example.com/lathe-9000\n\nThanks"
	got := SenderLinks(body, MaxSenderLinks)
	if len(got) != 1 || got[0] != "https://example.com/lathe-9000" {
		t.Fatalf("got %v", got)
	}
}

func TestSenderLinksStripsTrailingPunctuation(t *testing.T) {
	got := SenderLinks("see https://example.com/lathe-9000.", MaxSenderLinks)
	if len(got) != 1 || got[0] != "https://example.com/lathe-9000" {
		t.Fatalf("got %v; trailing full stop should not be part of the URL", got)
	}
}

func TestSenderLinksDropsFooterBoilerplate(t *testing.T) {
	body := `Which is better? https://example.com/mill-4200

--
Acme Ltd | https://acme.example/unsubscribe | https://twitter.com/acme
https://acme.example/logo.png`
	got := SenderLinks(body, MaxSenderLinks)
	if len(got) != 1 || got[0] != "https://example.com/mill-4200" {
		t.Fatalf("got %v; want only the product link", got)
	}
}

func TestSenderLinksDedupesAndCaps(t *testing.T) {
	body := "https://a.example/1 https://a.example/1/ https://b.example/2 https://c.example/3 https://d.example/4"
	got := SenderLinks(body, MaxSenderLinks)
	if len(got) != 3 {
		t.Fatalf("got %d links (%v); want the cap of %d", len(got), got, MaxSenderLinks)
	}
	if got[0] != "https://a.example/1" || got[1] != "https://b.example/2" {
		t.Fatalf("order or dedupe wrong: %v", got)
	}
}

func TestSenderLinksIgnoresNonHTTP(t *testing.T) {
	if got := SenderLinks("mailto:someone@example.com and ftp://files.example/x", MaxSenderLinks); got != nil {
		t.Fatalf("got %v; want nil", got)
	}
}

func TestSenderLinksEmptyBody(t *testing.T) {
	if got := SenderLinks("no links here", MaxSenderLinks); got != nil {
		t.Fatalf("got %v; want nil", got)
	}
}
