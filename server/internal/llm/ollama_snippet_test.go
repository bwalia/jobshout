package llm

import (
	"strings"
	"testing"
)

// The gateway answers with an HTML page. This is the one that reached the UI
// and filled an error card with a stylesheet.
const gatewayErrorPage = `<!DOCTYPE html> <html lang="en"> <head> <meta charset="utf-8">
<title>Access denied</title> <style> body { font-family: -apple-system, BlinkMacSystemFont,
"Segoe UI", Roboto, sans-serif; max-width: 600px; margin: 4em auto; padding: 2em;
color: #1f2937; line-height: 1.5; } h1 { font-size: 1.75em; color: #b91c1c; } </style> </head>
<body> <h1>403 - Access denied</h1> <p>This endpoint requires authentication. Please provide a
valid API key in the request.</p> <p><small>Powered by <a href="https://wslproxy.com/">wslproxy</a>.</small></p>
</body> </html>`

func TestUpstreamSnippet_HTMLErrorPage(t *testing.T) {
	got := upstreamSnippet([]byte(gatewayErrorPage))

	if len(got) > upstreamSnippetLimit+1 { // +1 for the ellipsis
		t.Errorf("snippet is %d chars, want at most %d", len(got), upstreamSnippetLimit+1)
	}
	// The one sentence a reader needs must survive.
	if !strings.Contains(got, "403 - Access denied") {
		t.Errorf("snippet lost the actual error: %q", got)
	}
	for _, junk := range []string{"<", ">", "font-family", "BlinkMacSystemFont", "DOCTYPE", "#1f2937"} {
		if strings.Contains(got, junk) {
			t.Errorf("snippet still carries markup/CSS (%q): %q", junk, got)
		}
	}
}

func TestUpstreamSnippet_PlainAndEmpty(t *testing.T) {
	if got := upstreamSnippet([]byte(`{"error":"model not found"}`)); got != `{"error":"model not found"}` {
		t.Errorf("a short JSON body should pass through, got %q", got)
	}
	if got := upstreamSnippet(nil); got != "(empty response)" {
		t.Errorf("empty body = %q, want a placeholder", got)
	}
	if got := upstreamSnippet([]byte("   \n\t  ")); got != "(empty response)" {
		t.Errorf("whitespace-only body = %q, want a placeholder", got)
	}
}

// The full error must stay actionable: it names the variable to check.
func TestAuthError_NamesTheFix(t *testing.T) {
	err := authError(403, []byte(gatewayErrorPage))
	msg := err.Error()
	if !strings.Contains(msg, "OLLAMA_JWT_SECRET") {
		t.Errorf("error should name the setting to fix: %q", msg)
	}
	if strings.Contains(msg, "<style>") || strings.Contains(msg, "font-family") {
		t.Errorf("error still embeds the page's stylesheet: %q", msg)
	}
}
