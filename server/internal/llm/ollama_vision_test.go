package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDescribeImage_SendsImageAndReadsAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Images []string `json:"images"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/api/chat" || body.Model != "vision-x" || len(body.Messages[0].Images) != 1 {
			t.Errorf("path=%s body=%+v", r.URL.Path, body)
		}
		_, _ = w.Write([]byte(`{"message":{"content":"  A glowing server rack.  "}}`))
	}))
	defer srv.Close()
	got, err := NewOllamaClient(srv.URL, "m").DescribeImage(context.Background(), "vision-x", "describe", []byte("png"))
	if err != nil || got != "A glowing server rack." {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestDescribeImage_Errors(t *testing.T) {
	c := NewOllamaClient("http://unused", "m")
	if _, err := c.DescribeImage(context.Background(), "", "p", []byte("x")); err == nil {
		t.Error("want error without a model")
	}
	if _, err := c.DescribeImage(context.Background(), "m", "p", nil); err == nil {
		t.Error("want error without an image")
	}
}
