package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestReadableRunError_RestatesGatewayTimeouts(t *testing.T) {
	err := fmt.Errorf("research: discover: %w", errors.New(
		"ollama: unexpected status 504: Server Error | WSL Proxy WSL Proxy 500 Server Error Something went wrong on our end."))

	got := readableRunError(err)

	if !strings.HasPrefix(got, "research: discover: the ollama model gateway answered HTTP 504 Gateway Timeout") {
		t.Errorf("got %q", got)
	}
	if strings.Contains(got, "WSL Proxy") {
		t.Errorf("gateway error page leaked into the run error: %q", got)
	}
}

func TestReadableRunError_LeavesOtherErrorsAlone(t *testing.T) {
	msg := "research: discover: found nothing worth writing about that has not been covered"
	if got := readableRunError(errors.New(msg)); got != msg {
		t.Errorf("got %q, want %q", got, msg)
	}
	// A 4xx from the model is the caller's problem, not the gateway's, and
	// keeps the upstream detail.
	bad := "ollama: unexpected status 400: model not found"
	if got := readableRunError(errors.New(bad)); got != bad {
		t.Errorf("got %q, want %q", got, bad)
	}
}

func TestReadableRunError_BoundsLength(t *testing.T) {
	got := readableRunError(errors.New(strings.Repeat("é", 1000)))
	if len(got) > maxRunErrorLen+len("…") {
		t.Errorf("length %d exceeds the bound", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated error is not marked: %q", got[len(got)-10:])
	}
}
