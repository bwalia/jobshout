package opsapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// UpdatePost is PUT /api/v2/cms/posts/{uuid} with the content fields only:
// status and slug are left to the editor (opsapi applies only fields sent).
func TestUpdatePost_PutsContentButNotStatusOrSlug(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/cms/posts/p-1" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Namespace-Slug") != "acme" || r.Header.Get("Authorization") != "Bearer "+testKey {
			t.Errorf("headers %v", r.Header)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"success":true,"data":{"uuid":"p-1","slug":"kept","status":"published"}}`))
	}))
	defer srv.Close()

	post, err := newTestClient(t, srv.URL).UpdatePost(context.Background(), "p-1", UpdatePostRequest{
		Title: "T", ContentHTML: "<p>x</p>", FeaturedImageURL: "https://img", Tags: []string{"a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if post.UUID != "p-1" || post.Status != StatusPublished {
		t.Fatalf("post = %+v", post)
	}
	for _, k := range []string{"status", "slug"} {
		if _, ok := body[k]; ok {
			t.Errorf("update sent %q; it must keep the editor's", k)
		}
	}
	if body["title"] != "T" || body["content_html"] != "<p>x</p>" || body["featured_image_url"] != "https://img" {
		t.Errorf("body = %v", body)
	}
}

func TestUpdatePost_DeletedPostIsErrPostNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"error":"Post not found"}`))
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).UpdatePost(context.Background(), "gone", UpdatePostRequest{Title: "T"})
	if !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("err = %v, want ErrPostNotFound", err)
	}
}
