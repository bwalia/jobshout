package opsapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreatePage_SendsEmailPage(t *testing.T) {
	var got map[string]any
	var gotAuth, gotNS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/cms/pages" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		gotAuth, gotNS = r.Header.Get("Authorization"), r.Header.Get("X-Namespace-Slug")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"success":true,"data":{"uuid":"p-1","title":"Hi","slug":"hi","status":"published","template":"email"}}`))
	}))
	defer srv.Close()

	off := false
	page, err := newTestClient(t, srv.URL).CreatePage(context.Background(), CreatePageRequest{
		Title: "Hi", Slug: "hi", ContentHTML: "<p>x</p>", Status: StatusPublished,
		Template: "email", ShowInNav: &off, SEOTitle: "Hi", SEODescription: "d",
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.UUID != "p-1" || page.Template != "email" {
		t.Errorf("page = %+v", page)
	}
	if gotAuth != "Bearer "+testKey || gotNS != "acme" {
		t.Errorf("auth = %q ns = %q", gotAuth, gotNS)
	}
	if got["template"] != "email" || got["status"] != "published" || got["show_in_nav"] != false {
		t.Errorf("body = %v", got)
	}
}

func TestFindPageBySlug_ExactMatchUpsert(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/cms/pages" || !strings.HasPrefix("spring-sale", r.URL.Query().Get("search")) {
			t.Errorf("request = %s %s", r.Method, r.URL.String())
		}
		// Fuzzy search: a near-miss comes first, the exact slug second.
		_, _ = w.Write([]byte(`{"success":true,"data":[
			{"uuid":"near","slug":"spring-sale-2","status":"draft","template":"email"},
			{"uuid":"exact","slug":"spring-sale","status":"published","template":"email"}]}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL)

	page, err := c.FindPageBySlug(context.Background(), "spring-sale")
	if err != nil || page == nil || page.UUID != "exact" {
		t.Fatalf("page = %+v err = %v", page, err)
	}
	missing, err := c.FindPageBySlug(context.Background(), "spring")
	if err != nil || missing != nil {
		// "spring" matches neither slug exactly.
		t.Fatalf("missing = %+v err = %v", missing, err)
	}
}

func TestFindPageBySlug_PaginatedShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"uuid":"a","slug":"x","status":"draft"}],"total":1}}`))
	}))
	defer srv.Close()
	page, err := newTestClient(t, srv.URL).FindPageBySlug(context.Background(), "x")
	if err != nil || page == nil || page.UUID != "a" {
		t.Fatalf("page = %+v err = %v", page, err)
	}
}

func TestUpdatePage_PartialAndArchive(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/cms/pages/p-1" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"success":true,"data":{"uuid":"p-1","slug":"hi","status":"archived","template":"email"}}`))
	}))
	defer srv.Close()
	page, err := newTestClient(t, srv.URL).SetPageStatus(context.Background(), "p-1", StatusArchived)
	if err != nil || page.Status != StatusArchived || got["status"] != "archived" {
		t.Fatalf("page = %+v err = %v body = %v", page, err, got)
	}
}

func TestUpdatePage_ForbiddenNamesScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"error":"missing scope"}`))
	}))
	defer srv.Close()
	_, err := newTestClient(t, srv.URL).UpdatePage(context.Background(), "p-1", map[string]string{"status": "published"})
	if err == nil || !IsUpdateForbidden(err) || !errors.Is(err, ErrUpdateForbidden) {
		t.Fatalf("err = %v, want ErrUpdateForbidden", err)
	}
	if !strings.Contains(err.Error(), "cms:update") {
		t.Errorf("error should name cms:update: %v", err)
	}
}

func TestCreatePage_ForbiddenNamesCreateScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := newTestClient(t, srv.URL).CreatePage(context.Background(), CreatePageRequest{Title: "t"})
	if err == nil || !strings.Contains(err.Error(), "cms:create") || IsUpdateForbidden(err) {
		t.Fatalf("err = %v", err)
	}
}
