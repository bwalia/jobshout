package opsapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// testToken mints an unsigned-secret HS256 token with the given expiry. Only
// the exp claim matters here — the client never verifies signatures, since it
// does not hold opsapi's key.
func testToken(t *testing.T, exp time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":      exp.Unix(),
		"iss":      "opsapi",
		"userinfo": map[string]any{"uuid": "user-1"},
	})
	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("mint test token: %v", err)
	}
	return signed
}

func newTestClient(t *testing.T, baseURL, token string) *Client {
	t.Helper()
	c := NewClient(Config{BaseURL: baseURL, Token: token, Namespace: "acme"})
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
		{"no base URL", Config{Token: "t", Namespace: "n"}},
		{"no token", Config{BaseURL: "https://x", Namespace: "n"}},
		{"no namespace", Config{BaseURL: "https://x", Token: "t"}},
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
	token := testToken(t, time.Now().Add(time.Hour))

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

	c := newTestClient(t, srv.URL, token)
	post, err := c.CreatePost(context.Background(), CreatePostRequest{
		Title:       "Hello",
		Slug:        "hello",
		ContentHTML: "<p>Hi</p>",
		Status:      StatusDraft,
	})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	if gotAuth != "Bearer "+token {
		t.Errorf("Authorization = %q, want the bearer token", gotAuth)
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

	c := newTestClient(t, srv.URL, testToken(t, time.Now().Add(time.Hour)))
	if _, err := c.CreatePost(context.Background(), CreatePostRequest{Title: "T"}); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if gotStatus != StatusDraft {
		t.Errorf("status = %q, want %q", gotStatus, StatusDraft)
	}
}

// A token near expiry is exchanged before it is used, so a publish does not
// spend a round trip discovering it is stale.
func TestCreatePost_RefreshesTokenBeforeExpiry(t *testing.T) {
	expiring := testToken(t, time.Now().Add(time.Minute)) // inside refreshMargin
	fresh := testToken(t, time.Now().Add(time.Hour))

	var refreshes, posts int32
	var postAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/refresh":
			atomic.AddInt32(&refreshes, 1)
			if got := r.Header.Get("Authorization"); got != "Bearer "+expiring {
				t.Errorf("refresh presented %q, want the current token", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"token": fresh})
		case "/api/v2/cms/posts":
			atomic.AddInt32(&posts, 1)
			postAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true, "data": map[string]any{"uuid": "u"},
			})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, expiring)
	if _, err := c.CreatePost(context.Background(), CreatePostRequest{Title: "T"}); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	if refreshes != 1 {
		t.Errorf("refreshes = %d, want 1", refreshes)
	}
	if postAuth != "Bearer "+fresh {
		t.Errorf("post used %q, want the refreshed token", postAuth)
	}

	// The refreshed token is cached: a second call must not refresh again.
	if _, err := c.CreatePost(context.Background(), CreatePostRequest{Title: "T2"}); err != nil {
		t.Fatalf("second CreatePost: %v", err)
	}
	if refreshes != 1 {
		t.Errorf("refreshes = %d after two posts, want 1 — the token should be cached", refreshes)
	}
	if posts != 2 {
		t.Errorf("posts = %d, want 2", posts)
	}
}

// Our expiry reading is a guess made without the signing secret. When opsapi
// disagrees, refresh and try once more rather than failing the publish.
func TestCreatePost_RetriesOnceAfter401(t *testing.T) {
	stale := testToken(t, time.Now().Add(time.Hour)) // looks fine to us
	fresh := testToken(t, time.Now().Add(time.Hour))

	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/refresh" {
			_ = json.NewEncoder(w).Encode(map[string]any{"token": fresh})
			return
		}
		n := atomic.AddInt32(&posts, 1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Invalid or expired token"}`))
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+fresh {
			t.Errorf("retry used %q, want the refreshed token", got)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "data": map[string]any{"uuid": "u"},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, stale)
	post, err := c.CreatePost(context.Background(), CreatePostRequest{Title: "T"})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if post.UUID != "u" {
		t.Errorf("post UUID = %q, want %q", post.UUID, "u")
	}
	if posts != 2 {
		t.Errorf("post attempts = %d, want 2 (one rejected, one retried)", posts)
	}
}

// A 403 means a permissions problem the same token will hit again. Retrying
// only doubles the latency before the same failure.
func TestCreatePost_DoesNotRetryOn403(t *testing.T) {
	var posts, refreshes int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/refresh" {
			atomic.AddInt32(&refreshes, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "x"})
			return
		}
		atomic.AddInt32(&posts, 1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"Insufficient permissions"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, testToken(t, time.Now().Add(time.Hour)))
	_, err := c.CreatePost(context.Background(), CreatePostRequest{Title: "T"})
	if err == nil {
		t.Fatal("expected an error on 403")
	}
	if posts != 1 {
		t.Errorf("post attempts = %d, want 1", posts)
	}
	if refreshes != 0 {
		t.Errorf("refreshes = %d, want 0 — a 403 is not an auth-expiry problem", refreshes)
	}
	// The message should point at the two things an operator can actually fix.
	if !strings.Contains(err.Error(), "cms") || !strings.Contains(err.Error(), "acme") {
		t.Errorf("403 message should name the module and namespace, got: %v", err)
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

	c := newTestClient(t, srv.URL, testToken(t, time.Now().Add(time.Hour)))
	_, err := c.CreatePost(context.Background(), CreatePostRequest{})
	if err == nil {
		t.Fatal("expected an error when success is false")
	}
	if !strings.Contains(err.Error(), "title is required") {
		t.Errorf("error should carry opsapi's reason, got: %v", err)
	}
}

// Past the grace window nothing but a new seed token helps, and the error
// should say so rather than looking like a transient failure.
func TestRefresh_PastGraceWindowExplainsItself(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Invalid or expired token"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, testToken(t, time.Now().Add(-graceWindow-24*time.Hour)))
	_, err := c.CreatePost(context.Background(), CreatePostRequest{Title: "T"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "OPSAPI_TOKEN") || !strings.Contains(err.Error(), "grace window") {
		t.Errorf("error should tell an operator to reseed the token, got: %v", err)
	}
}

// A token we cannot parse reads as already expired, so the client refreshes on
// first use rather than sending something opsapi will reject.
func TestTokenExpiry_Unparseable(t *testing.T) {
	for _, token := range []string{"", "not-a-jwt", "a.b.c"} {
		if got := tokenExpiry(token); !got.IsZero() {
			t.Errorf("tokenExpiry(%q) = %v, want the zero time", token, got)
		}
	}
}

func TestTokenExpiry_ReadsExpClaim(t *testing.T) {
	want := time.Now().Add(42 * time.Minute).Truncate(time.Second)
	if got := tokenExpiry(testToken(t, want)); !got.Equal(want) {
		t.Errorf("tokenExpiry() = %v, want %v", got, want)
	}
}

// Base URLs are configured by hand and a trailing slash is easy to leave on.
func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	c := NewClient(Config{BaseURL: "https://ops.example.com/", Token: "t", Namespace: "n"})
	if c.BaseURL() != "https://ops.example.com" {
		t.Errorf("BaseURL() = %q, want the trailing slash removed", c.BaseURL())
	}
}
