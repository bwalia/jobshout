package googleauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthURL(t *testing.T) {
	got := AuthURL("client-1", "http://localhost:8190/api/v1/auth/google/callback", "abc123")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "accounts.google.com" {
		t.Fatalf("host: %s", u.Host)
	}
	q := u.Query()
	if q.Get("client_id") != "client-1" {
		t.Fatalf("client_id: %s", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != "http://localhost:8190/api/v1/auth/google/callback" {
		t.Fatalf("redirect_uri: %s", q.Get("redirect_uri"))
	}
	if q.Get("response_type") != "code" {
		t.Fatalf("response_type: %s", q.Get("response_type"))
	}
	if q.Get("state") != "abc123" {
		t.Fatalf("state: %s", q.Get("state"))
	}
	if q.Get("prompt") != "select_account" {
		t.Fatalf("prompt: %s", q.Get("prompt"))
	}
	scope := q.Get("scope")
	for _, want := range []string{"openid", "email", "profile"} {
		if !strings.Contains(scope, want) {
			t.Fatalf("scope %q missing %s", scope, want)
		}
	}
	if q.Get("access_type") == "offline" {
		t.Fatal("login must not request a Google refresh token")
	}
}

func TestProfileFromCode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("token method %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("code") != "good-code" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "ya29.tok"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ya29.tok" {
			t.Errorf("auth header %s", got)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":            "google-sub-1",
			"email":          "Alex@Example.com",
			"name":           "Alex Example",
			"picture":        "https://example.com/a.png",
			"email_verified": true,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := Config{
		ClientID:     "cid",
		ClientSecret: "csecret",
		RedirectURL:  "http://localhost/callback",
	}
	c := NewClient(cfg, &http.Client{Transport: rewriteHost{base: srv.URL}})
	p, err := c.ProfileFromCode(context.Background(), "good-code")
	if err != nil {
		t.Fatal(err)
	}
	if p.Sub != "google-sub-1" {
		t.Fatalf("sub: %s", p.Sub)
	}
	if p.Email != "alex@example.com" {
		t.Fatalf("email should be lowercased, got %s", p.Email)
	}
	if p.Name != "Alex Example" {
		t.Fatalf("name: %s", p.Name)
	}
	if !p.EmailVerified {
		t.Fatal("expected verified")
	}
}

func TestProfileFromCodeUnverified(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":            "s",
			"email":          "a@b.com",
			"email_verified": false,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewClient(Config{ClientID: "c", ClientSecret: "s", RedirectURL: "http://x"}, &http.Client{Transport: rewriteHost{base: srv.URL}})
	_, err := c.ProfileFromCode(context.Background(), "code")
	if err != ErrEmailNotVerified {
		t.Fatalf("want ErrEmailNotVerified, got %v", err)
	}
}

// rewriteHost sends token and userinfo Google URLs to the httptest server.
type rewriteHost struct{ base string }

func (r rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(r.base)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.Contains(req.URL.Host, "googleapis.com") && strings.Contains(req.URL.Path, "token"):
		req.URL.Scheme, req.URL.Host, req.URL.Path = u.Scheme, u.Host, "/token"
	case strings.Contains(req.URL.Host, "googleapis.com") && strings.Contains(req.URL.Path, "userinfo"):
		req.URL.Scheme, req.URL.Host, req.URL.Path = u.Scheme, u.Host, "/userinfo"
	}
	req.Host = req.URL.Host
	return http.DefaultTransport.RoundTrip(req)
}
