package mail

import (
	"net/url"
	"regexp"
	"strings"
)

// MaxSenderLinks caps how many links from one inbound message are handed to the
// Research Agent.
//
// Three is enough for the cases this exists to serve — "what does this cost",
// "do you have this", "which of these two" — and small enough that a message
// with a link-heavy footer cannot turn one reply into a crawl.
const MaxSenderLinks = 3

// senderLinkPattern matches http(s) URLs in message text.
//
// The trailing class excludes the punctuation that commonly abuts a URL in
// prose, so "see https://example.com/x." yields the URL without the sentence's
// full stop — a trailing dot would 404 and the finding would be lost.
var senderLinkPattern = regexp.MustCompile(`https?://[^\s<>"'\)\]]+[^\s<>"'\)\]\.,;:!?]`)

// noisyLinkHosts are hosts whose links are navigation, tracking or social
// chrome rather than the thing the sender is asking about. A link to one of
// these in a signature should not become the subject of the research.
var noisyLinkHosts = []string{
	"twitter.com", "x.com", "linkedin.com", "facebook.com", "instagram.com",
	"youtube.com", "list-manage.com", "mailchimp.com", "sendgrid.net",
	"doubleclick.net", "googletagmanager.com", "google-analytics.com",
}

// noisyLinkPaths are path fragments that mark a link as boilerplate. Kept
// separate from hosts because these appear under the sender's own domain.
var noisyLinkPaths = []string{
	"/unsubscribe", "/privacy", "/terms", "/legal", "/cookie",
	"/preferences", "/opt-out", "/optout",
}

// imageSuffixes are files the reader cannot turn into prose, so fetching one
// spends a research slot to learn nothing.
var imageSuffixes = []string{".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico"}

// SenderLinks extracts the content links a sender pasted into a message, in the
// order they appeared, de-duplicated and capped at limit.
//
// This is what lets the Mail Agent answer "what is the price of this machine?"
// by reading the machine's page: the URLs it returns are handed to the Research
// Agent as research.Request.URLs, which selects the direct-fetch path instead of
// an open-web search around the subject line.
//
// Boilerplate is filtered because the alternative is worse than fetching
// nothing: an unsubscribe link in a footer would otherwise become the only
// source the reply is allowed to cite.
//
// Note these URLs are attacker-controlled — anyone who can email the mailbox
// chooses them. That is safe today because research fetches go through the Jina
// reader rather than this server (see research.go's note on SSRF); a direct
// fetcher added later would need its own private-address guard before this
// input reached it.
func SenderLinks(body string, limit int) []string {
	if limit <= 0 {
		limit = MaxSenderLinks
	}
	found := senderLinkPattern.FindAllString(body, -1)
	if len(found) == 0 {
		return nil
	}

	out := make([]string, 0, limit)
	seen := make(map[string]struct{}, len(found))
	for _, raw := range found {
		clean := strings.TrimSpace(raw)
		u, err := url.Parse(clean)
		if err != nil || u.Host == "" {
			continue
		}
		if scheme := strings.ToLower(u.Scheme); scheme != "http" && scheme != "https" {
			continue
		}
		if isNoisyLink(u) {
			continue
		}
		key := strings.ToLower(strings.TrimSuffix(clean, "/"))
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, clean)
		if len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isNoisyLink(u *url.URL) bool {
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	for _, h := range noisyLinkHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	path := strings.ToLower(u.Path)
	for _, p := range noisyLinkPaths {
		if strings.Contains(path, p) {
			return true
		}
	}
	for _, ext := range imageSuffixes {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
