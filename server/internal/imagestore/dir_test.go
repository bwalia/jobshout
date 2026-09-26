package imagestore

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDirStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewDirStore(dir)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a}

	url, err := s.Put(context.Background(), uuid.New(), png)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, URLPrefix) {
		t.Fatalf("url %q missing prefix %q", url, URLPrefix)
	}

	got, err := s.Get(context.Background(), strings.TrimPrefix(url, URLPrefix))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, png) {
		t.Fatalf("got %v want %v", got, png)
	}
}

func TestDirStore_RejectsTraversal(t *testing.T) {
	s := NewDirStore(t.TempDir())
	if _, err := s.Get(context.Background(), "../secret.png"); err == nil {
		t.Fatal("expected invalid key")
	}
}
