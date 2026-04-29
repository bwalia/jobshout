package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreatePullRequest_Success(t *testing.T) {
	var received map[string]any
	var path, authz string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		authz = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":42,"html_url":"https://github.com/bwalia/workstation-website/pull/42","state":"open"}`))
	}))
	defer srv.Close()

	c := NewPullRequestClient("test-token")
	c.BaseURL = srv.URL

	pr, err := c.CreatePullRequest(context.Background(),
		"bwalia", "workstation-website",
		"AI Generated Blog: Kubernetes",
		"ai/blog-2026-04-29-abcd",
		"main",
		"Auto-generated.")
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("Number = %d, want 42", pr.Number)
	}
	if pr.HTMLURL == "" {
		t.Error("HTMLURL empty")
	}
	if path != "/repos/bwalia/workstation-website/pulls" {
		t.Errorf("path = %q", path)
	}
	if !strings.HasPrefix(authz, "token ") {
		t.Errorf("missing bearer auth header: %q", authz)
	}
	if received["title"] != "AI Generated Blog: Kubernetes" {
		t.Errorf("title round-trip failed: %v", received["title"])
	}
	if received["head"] != "ai/blog-2026-04-29-abcd" || received["base"] != "main" {
		t.Errorf("branch fields incorrect: %v", received)
	}
}

func TestCreatePullRequest_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"A pull request already exists"}`))
	}))
	defer srv.Close()

	c := NewPullRequestClient("t")
	c.BaseURL = srv.URL

	_, err := c.CreatePullRequest(context.Background(), "o", "r", "t", "h", "b", "body")
	if err == nil {
		t.Fatal("expected error on 422")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should echo server message: %v", err)
	}
}

func TestCreatePullRequest_NoToken(t *testing.T) {
	c := NewPullRequestClient("")
	_, err := c.CreatePullRequest(context.Background(), "o", "r", "t", "h", "b", "body")
	if err == nil {
		t.Fatal("expected error when token empty")
	}
}
