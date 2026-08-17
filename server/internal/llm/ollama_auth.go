package llm

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/jobshout/server/internal/gatewayauth"
)

// AppName identifies this application in the JWT presented to the Ollama
// gateway. It is the value of the "app" claim.
//
// Ollama is no longer the only workstation service JobShout authenticates to —
// the image service uses the same scheme — so the signing itself now lives in
// gatewayauth and this package re-exports the name it has always exported.
const AppName = gatewayauth.AppName

// ollamaTokenTTL is how long a minted gateway token stays valid. Short on
// purpose: a token is used for exactly one request, so it never needs to
// outlive that request by much.
const ollamaTokenTTL = gatewayauth.TokenTTL

// ollamaAuth signs requests to an Ollama deployment that sits behind a
// JWT-verifying gateway.
//
// A nil *ollamaAuth (or one with no secret) is valid and means "talk to Ollama
// directly" — a plain local Ollama has no gateway and rejects nothing, so the
// same client works for both without a second code path.
//
// It is a thin shell over gatewayauth.Signer, kept so this package's callers
// keep their unexported spelling while the scheme has one implementation.
type ollamaAuth struct {
	*gatewayauth.Signer
}

// newOllamaAuth returns nil when no secret is configured, so callers can hold
// a possibly-nil *ollamaAuth and call apply on it unconditionally.
func newOllamaAuth(secret string) *ollamaAuth {
	signer := gatewayauth.New(secret)
	if signer == nil {
		return nil
	}
	return &ollamaAuth{Signer: signer}
}

// enabled reports whether requests will be signed.
//
// Written against a nil receiver rather than promoted from the embedded Signer
// because a nil *ollamaAuth has no embedded value to promote through.
func (a *ollamaAuth) enabled() bool { return a != nil && a.Signer.Enabled() }

// apply mints a fresh token and attaches it to req.
//
// A new token is minted for every single request and never cached: the tokens
// are cheap to produce, and reusing one across requests is what turns a single
// clock skew or rotation into a stream of failures rather than one.
func (a *ollamaAuth) apply(req *http.Request) error {
	if !a.enabled() {
		return nil
	}
	if err := a.Signer.Apply(req); err != nil {
		return fmt.Errorf("ollama: %w", err)
	}
	return nil
}

// authError turns a gateway rejection into an error that says what to fix.
//
// 401/403 from the gateway means the signature did not verify — a wrong or
// missing shared secret, or a clock far enough out that exp/iat look invalid.
// Retrying is pointless: the same secret produces the same verdict, and a
// fresh token would fail identically. Returning a terminal error keeps callers
// from burning a retry budget on it.
func authError(status int, body []byte) error {
	return fmt.Errorf(
		"ollama: gateway rejected the request (status %d) — the JWT signature did not verify. "+
			"Check OLLAMA_JWT_SECRET matches the gateway's secret and that this host's clock is accurate. "+
			"Response: %s",
		status, upstreamSnippet(body),
	)
}

// isAuthStatus reports whether a status code is a gateway auth rejection.
func isAuthStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

// upstreamSnippetLimit bounds how much of an upstream response body is carried
// into an error. Long enough to identify the failure, short enough that the
// message stays readable where it surfaces — a blog run stores it and the UI
// shows it on a card.
const upstreamSnippetLimit = 200

var (
	htmlTagRegex    = regexp.MustCompile(`(?s)<[^>]*>`)
	whitespaceRegex = regexp.MustCompile(`\s+`)
)

// upstreamSnippet reduces an upstream response body to something worth showing
// a human.
//
// Gateways answer with an HTML error page, not JSON. Embedding one verbatim put
// a full document — doctype, inline stylesheet and all — into an error message
// that a user reads on a card in the UI, burying the one sentence that said
// what went wrong. Tags are stripped, whitespace collapsed, and the result
// truncated on a word boundary.
func upstreamSnippet(body []byte) string {
	s := string(body)
	if strings.Contains(s, "<") && strings.Contains(s, ">") {
		// Drop <style>/<script> contents outright: they are never diagnostic,
		// and stripping only their tags would leave CSS rules as "text".
		s = scriptStyleRegex.ReplaceAllString(s, " ")
		s = htmlTagRegex.ReplaceAllString(s, " ")
	}
	s = strings.TrimSpace(whitespaceRegex.ReplaceAllString(s, " "))
	if s == "" {
		return "(empty response)"
	}
	if len(s) <= upstreamSnippetLimit {
		return s
	}
	cut := s[:upstreamSnippetLimit]
	if idx := strings.LastIndexByte(cut, ' '); idx > 0 {
		cut = cut[:idx]
	}
	return cut + "…"
}

var scriptStyleRegex = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)>`)
