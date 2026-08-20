package opsapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// testKey looks like a real opsapi API key so tests exercise the same shape
// operators will paste in.
const testKey = "opsk_0123456789abcdef0123456789abcdef0123456789abcdef"

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	c := NewClient(Config{BaseURL: baseURL, APIKey: testKey, Namespace: "acme"})
	if c == nil {
		t.Fatal("NewClient returned nil for a complete config")
	}
	return c
}

func TestNewClient_IncompleteConfigIsNil(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"no base URL", Config{APIKey: "k", Namespace: "n"}},
		{"no API key", Config{BaseURL: "https://x", Namespace: "n"}},
		{"no namespace", Config{BaseURL: "https://x", APIKey: "k"}},
		{"empty", Config{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewClient(tt.cfg); got != nil {
				t.Errorf("NewClient(%+v) = %v, want nil", tt.cfg, got)
			}
		})
	}
}

func TestCreatePost_SendsNamespaceAndAuth(t *testing.T) {
	var gotAuth, gotNamespace, gotContentType string
	var gotBody CreatePostRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/cms/posts" {
			t.Errorf("path = %q, want /api/v2/cms/posts", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		gotNamespace = r.Header.Get("X-Namespace-Slug")
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"uuid": "post-abc", "title": gotBody.Title,
				"slug": gotBody.Slug, "status": gotBody.Status,
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	post, err := c.CreatePost(context.Background(), CreatePostRequest{
		Title:       "Hello",
		Slug:        "hello",
		ContentHTML: "<p>Hi</p>",
		Status:      StatusDraft,
	})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	if gotAuth != "Bearer "+testKey {
		t.Errorf("Authorization = %q, want the API key sent verbatim as a bearer token", gotAuth)
	}
	if gotNamespace != "acme" {
		t.Errorf("X-Namespace-Slug = %q, want %q", gotNamespace, "acme")
	}
	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if post.UUID != "post-abc" {
		t.Errorf("post UUID = %q, want %q", post.UUID, "post-abc")
	}
}

// An unset status must not reach opsapi as "", which its validation rejects.
func TestCreatePost_DefaultsToDraft(t *testing.T) {
	var gotStatus string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body CreatePostRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotStatus = body.Status
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "data": map[string]any{"uuid": "u", "status": body.Status},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	if _, err := c.CreatePost(context.Background(), CreatePostRequest{Title: "T"}); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if gotStatus != StatusDraft {
		t.Errorf("status = %q, want %q", gotStatus, StatusDraft)
	}
}

// A 401 means the key was revoked or expired — the same key will be rejected
// again, so a retry only doubles the latency before the same failure. The
// error must tell an operator how to fix it.
func TestCreatePost_DoesNotRetryOn401(t *testing.T) {
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"API key has been revoked"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.CreatePost(context.Background(), CreatePostRequest{Title: "T"})
	if err == nil {
		t.Fatal("expected an error on 401")
	}
	if posts != 1 {
		t.Errorf("post attempts = %d, want 1 — a rejected key must not be retried", posts)
	}
	if !strings.Contains(err.Error(), "OPSAPI_API_KEY") {
		t.Errorf("401 message should name the setting an operator has to fix, got: %v", err)
	}
}

// A 403 means a permissions problem the same key will hit again.
func TestCreatePost_DoesNotRetryOn403(t *testing.T) {
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"Insufficient permissions"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.CreatePost(context.Background(), CreatePostRequest{Title: "T"})
	if err == nil {
		t.Fatal("expected an error on 403")
	}
	if posts != 1 {
		t.Errorf("post attempts = %d, want 1", posts)
	}
	// The message should point at the things an operator can actually fix.
	if !strings.Contains(err.Error(), "cms") || !strings.Contains(err.Error(), "acme") {
		t.Errorf("403 message should name the scope and namespace, got: %v", err)
	}
}

// opsapi answers 200 with success=false for some validation failures, which
// must not be read as a created post.
func TestCreatePost_HonoursSuccessFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false, "error": "title is required",
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.CreatePost(context.Background(), CreatePostRequest{})
	if err == nil {
		t.Fatal("expected an error when success is false")
	}
	if !strings.Contains(err.Error(), "title is required") {
		t.Errorf("error should carry opsapi's reason, got: %v", err)
	}
}

// Base URLs are configured by hand and a trailing slash is easy to leave on.
func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	c := NewClient(Config{BaseURL: "https://ops.example.com/", APIKey: "k", Namespace: "n"})
	if c.BaseURL() != "https://ops.example.com" {
		t.Errorf("BaseURL() = %q, want the trailing slash removed", c.BaseURL())
	}
}
