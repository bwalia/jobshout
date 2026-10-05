package secretsrot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A 401 on the data read must NOT be reported as "secret not found" — otherwise
// the rotation planner treats an auth failure as an empty path and proposes
// creating version 1 over a secret that may well exist.
func TestMetadataKV2_AuthFailureIsNotNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// metadata endpoint 404s (as WSLVault does), data endpoint 401s.
		if strings.Contains(r.URL.Path, "/metadata/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthenticated"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Addr: srv.URL, Token: "t"})
	_, err := c.MetadataKV2(context.Background(), "secret", "audit/x")
	if err == nil {
		t.Fatal("want an error on 401")
	}
	if errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("401 must not be reported as not-found: %v", err)
	}
}

// A genuine 404 on both endpoints is reported as not-found, so the planner can
// legitimately plan "create version 1".
func TestMetadataKV2_GenuineNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(Config{Addr: srv.URL, Token: "t"})
	_, err := c.MetadataKV2(context.Background(), "secret", "audit/x")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("genuine 404 should be ErrSecretNotFound, got %v", err)
	}
}
