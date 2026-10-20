package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/imagegen"
	"github.com/jobshout/server/internal/imagestore"
	"github.com/jobshout/server/internal/service"
)

func uploadRequest(t *testing.T, field string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	w, err := mw.CreateFormFile(field, "photo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/images/upload", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return withOrg(r, uuid.New().String())
}

func newUploadHandler(t *testing.T) *ImageHandler {
	t.Helper()
	router := imagegen.NewTestRouter("", map[string]imagegen.Client{})
	return NewImageHandler(service.NewImageService(router, imagestore.NewDirStore(t.TempDir()), nil, zap.NewNop()))
}

func TestUpload_StoresImageAndReturnsURL(t *testing.T) {
	h := newUploadHandler(t)
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 64)...)

	rec := httptest.NewRecorder()
	h.Upload(rec, uploadRequest(t, "file", png))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.URL == "" {
		t.Fatal("no url returned")
	}
}

func TestUpload_RejectsNonImage(t *testing.T) {
	h := newUploadHandler(t)

	rec := httptest.NewRecorder()
	h.Upload(rec, uploadRequest(t, "file", []byte("this is plainly text, not an image at all")))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestContentTypeForKey(t *testing.T) {
	cases := map[string]string{
		"a/b/x.png":  "image/png",
		"a/b/x.jpg":  "image/jpeg",
		"a/b/x.jpeg": "image/jpeg",
		"a/b/x.webp": "image/webp",
		"a/b/x.gif":  "image/gif",
		"a/b/x.mp4":  "video/mp4",
	}
	for key, want := range cases {
		if got := contentTypeForKey(key); got != want {
			t.Errorf("contentTypeForKey(%q) = %q, want %q", key, got, want)
		}
	}
}
