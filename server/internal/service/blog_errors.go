package service

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// maxRunErrorLen bounds what a failed run shows. The full error is still in
// the server log; the run card has three lines.
const maxRunErrorLen = 400

// upstreamStatusRe finds an LLM client's non-200 error, e.g.
// "ollama: unexpected status 504: Server Error | WSL Proxy …".
var upstreamStatusRe = regexp.MustCompile(`\b(ollama|openai|claude): unexpected status (\d{3})`)

// readableRunError turns a run's failure into the line recorded on the run
// and its failed step.
//
// A model gateway that times out answers with its own HTML error page. The
// LLM client strips the tags, but what is left — "Server Error | WSL Proxy WSL
// Proxy 500 Server Error Something went wrong on our end…" — reads as a bug in
// the article writer and says nothing about what to do. The status code is the
// useful part, so gateway failures are restated around it.
func readableRunError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if loc := upstreamStatusRe.FindStringSubmatchIndex(msg); loc != nil {
		provider := msg[loc[2]:loc[3]]
		code, _ := strconv.Atoi(msg[loc[4]:loc[5]])
		if code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout {
			out := fmt.Sprintf(
				"the %s model gateway answered HTTP %d %s — the model host is overloaded or unreachable. Retry the run once it is responding.",
				provider, code, http.StatusText(code))
			if prefix := strings.TrimSuffix(strings.TrimSpace(msg[:loc[0]]), ":"); prefix != "" {
				out = prefix + ": " + out
			}
			return out
		}
	}
	return truncateRunError(msg)
}

func truncateRunError(msg string) string {
	if len(msg) <= maxRunErrorLen {
		return msg
	}
	cut := msg[:maxRunErrorLen]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "…"
}
