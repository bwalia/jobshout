package opsapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreatePost_SendsCategoryUUIDs(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"success":true,"data":{"uuid":"p1"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	if _, err := c.CreatePost(context.Background(), CreatePostRequest{Title: "T", CategoryUUIDs: []string{"c1", "c2"}}); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if got, _ := json.Marshal(body["category_uuids"]); string(got) != `["c1","c2"]` {
		t.Errorf("category_uuids = %s", got)
	}

	body = nil
	if _, err := c.CreatePost(context.Background(), CreatePostRequest{Title: "T"}); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if _, sent := body["category_uuids"]; sent {
		t.Error("category_uuids sent with none set; opsapi would clear the post's categories on an update")
	}
}

func TestListCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/cms/categories" || r.Header.Get("X-Namespace-Slug") != "acme" {
			t.Errorf("request %s %s ns=%q", r.Method, r.URL.Path, r.Header.Get("X-Namespace-Slug"))
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"uuid":"c1","name":"Buy-to-let","slug":"buy-to-let","post_count":2}]}`))
	}))
	defer srv.Close()

	cats, err := newTestClient(t, srv.URL).ListCategories(context.Background())
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(cats) != 1 || cats[0].UUID != "c1" || cats[0].Slug != "buy-to-let" {
		t.Errorf("cats = %+v", cats)
	}
}

func TestUploadMedia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/cms/media" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("not multipart: %v", err)
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("no file part: %v", err)
		}
		data, _ := io.ReadAll(f)
		if hdr.Filename != "cover.png" || string(data) != "PNGDATA" {
			t.Errorf("file %q = %q", hdr.Filename, data)
		}
		if r.FormValue("title") != "Gulf investors" || r.FormValue("alt_text") != "A cover" {
			t.Errorf("title %q alt %q", r.FormValue("title"), r.FormValue("alt_text"))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"success":true,"data":{"uuid":"m1","url":"https://cdn.test/img/cms/acme/m1.png","mime_type":"image/png"}}`))
	}))
	defer srv.Close()

	m, err := newTestClient(t, srv.URL).UploadMedia(context.Background(), "cover.png", "image/png", []byte("PNGDATA"), "Gulf investors", "A cover")
	if err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
	if m.URL != "https://cdn.test/img/cms/acme/m1.png" {
		t.Errorf("url = %q", m.URL)
	}
}

func TestUploadMedia_NoStorageIsNotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"success":false,"error":"Image storage isn't set up on this server yet."}`))
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).UploadMedia(context.Background(), "x.png", "image/png", []byte("x"), "", "")
	if !errors.Is(err, ErrMediaNotConfigured) || !strings.Contains(err.Error(), "acme") {
		t.Fatalf("err = %v, want ErrMediaNotConfigured naming the namespace", err)
	}
}

func TestUploadMedia_TooLargeIsRefusedLocally(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://127.0.0.1:1", APIKey: testKey, Namespace: "acme"})
	if _, err := c.UploadMedia(context.Background(), "x.png", "", make([]byte, MaxMediaBytes+1), "", ""); err == nil {
		t.Fatal("an image over opsapi's limit was sent")
	}
}
