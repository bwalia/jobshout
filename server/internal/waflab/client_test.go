package waflab

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientTokenRefreshOn401(t *testing.T) {
	var logins atomic.Int32
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/user/login" && r.Method == http.MethodPost:
			logins.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"accessToken": "tok-" + string(rune('A'+logins.Load()-1))},
			})
		case r.URL.Path == "/api/waf_rules":
			hits.Add(1)
			auth := r.Header.Get("Authorization")
			if hits.Load() == 1 {
				http.Error(w, `{"error":"expired","accessToken":"leak-me"}`, http.StatusUnauthorized)
				return
			}
			if !strings.HasPrefix(auth, "Bearer ") {
				t.Errorf("missing bearer on retry: %q", auth)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "r1"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{
		Enabled:  true,
		BaseURL:  srv.URL,
		Username: "u@example.com",
		Password: "secret",
		Platform: "openresty-admin-next",
	}, nil)
	rules, err := c.ListWAFRules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("rules=%v", rules)
	}
	if logins.Load() < 2 {
		t.Fatalf("expected login + refresh, got %d logins", logins.Load())
	}
}

func TestImportDataPercentEncodesAmpersandAndEquals(t *testing.T) {
	var gotBody string
	var gotCT string
	var gotPlatform string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects/import" {
			http.NotFound(w, r)
			return
		}
		gotPlatform = r.Header.Get("x-platform")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient(Config{
		Enabled:  true,
		BaseURL:  srv.URL,
		APIToken: "static-token",
		Platform: "openresty-admin-next",
	}, nil)
	err := c.ImportData(context.Background(), "waf_rules", []map[string]any{
		{"id": "cmdi", "pattern": "a && b = c"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPlatform != "openresty-admin-next" {
		t.Fatalf("x-platform=%q", gotPlatform)
	}
	if !strings.Contains(gotCT, "application/x-www-form-urlencoded") {
		t.Fatalf("content-type=%q", gotCT)
	}
	// Raw body must not contain literal & or = (would truncate form parse).
	if strings.Contains(gotBody, "&&") || strings.Contains(gotBody, " = ") {
		t.Fatalf("body still has literal &/=: %s", gotBody)
	}
	decoded, err := url.QueryUnescape(gotBody)
	if err != nil {
		t.Fatal(err)
	}
	// json.Marshal escapes & as \u0026; either form proves the payload survived.
	if !strings.Contains(decoded, "b = c") {
		t.Fatalf("decoded body missing pattern: %s", decoded)
	}
	if !strings.Contains(decoded, "cmdi") {
		t.Fatalf("decoded body missing rule id: %s", decoded)
	}
	if !strings.Contains(decoded, `"dataType":"waf_rules"`) {
		t.Fatalf("decoded body missing dataType: %s", decoded)
	}
	// Encoded form must escape & so form-parse cannot truncate.
	if !strings.Contains(gotBody, "%26") && !strings.Contains(gotBody, "%3D") {
		t.Fatalf("expected percent-encoded &/= in body: %s", gotBody)
	}
}

func TestXPlatformHeaderOnMutatingRequests(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("x-platform")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := NewClient(Config{Enabled: true, BaseURL: srv.URL, APIToken: "t", Platform: "openresty-admin-next"}, nil)
	_ = c.SeedWAFRules(context.Background(), "prod")
	if got != "openresty-admin-next" {
		t.Fatalf("x-platform=%q", got)
	}
}

func TestWAFTestRefusesNonAllowListedTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/waf/test/targets":
			_ = json.NewEncoder(w).Encode([]string{
				"https://payments-secure.fictionally.org",
				"https://payments-open.fictionally.org",
			})
		case "/api/waf/test":
			t.Fatal("relay must not be called for refused target")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewClient(Config{Enabled: true, BaseURL: srv.URL, APIToken: "t", Platform: "openresty-admin-next"}, nil)
	_, err := c.WAFTest(context.Background(), WAFTestRequest{
		Target: "https://evil.example",
		Method: "GET",
		Path:   "/",
	})
	if err == nil {
		t.Fatal("expected refusal")
	}
	if !strings.Contains(err.Error(), "not on the WAF test allow-list") {
		t.Fatalf("error=%v", err)
	}
	if !strings.Contains(err.Error(), "payments-secure.fictionally.org") {
		t.Fatalf("error should name allow-list: %v", err)
	}
}

func TestRedactSecretsInErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad","password":"supersecret","accessToken":"jwt.abc"} Authorization Bearer abc.def.ghi`, 500)
	}))
	defer srv.Close()
	c := NewClient(Config{Enabled: true, BaseURL: srv.URL, APIToken: "t", Platform: "openresty-admin-next"}, nil)
	err := c.SeedWAFRules(context.Background(), "prod")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "supersecret") || strings.Contains(msg, "jwt.abc") || strings.Contains(msg, "abc.def.ghi") {
		t.Fatalf("secrets leaked: %s", msg)
	}
	if !strings.Contains(msg, "[REDACTED]") {
		t.Fatalf("expected redaction markers: %s", msg)
	}
}

func TestEnabledRequiresBaseAndCreds(t *testing.T) {
	if NewClient(Config{Enabled: true, BaseURL: "https://x"}, nil).Enabled() {
		t.Fatal("should be disabled without creds")
	}
	if !NewClient(Config{Enabled: true, BaseURL: "https://x", APIToken: "t"}, nil).Enabled() {
		t.Fatal("token should enable")
	}
	if !NewClient(Config{Enabled: true, BaseURL: "https://x", Username: "a", Password: "b"}, nil).Enabled() {
		t.Fatal("user/pass should enable")
	}
	if NewClient(Config{Enabled: false, BaseURL: "https://x", APIToken: "t"}, nil).Enabled() {
		t.Fatal("flag false should disable")
	}
}

func TestHTMLErrorPageIsSummarised(t *testing.T) {
	// wslproxy answers some 5xx with a full styled error page. Verbatim, it put
	// kilobytes of CSS in the run's step detail and hid the actual failure.
	page := `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8" />
<title>Server Error | WSL Proxy</title>
<style>` + strings.Repeat(".btn-home { background: var(--gradient); }\n", 200) + `</style>
</head><body><div class="error-code">500</div></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	c := NewClient(Config{Enabled: true, BaseURL: srv.URL, APIToken: "t", Platform: "openresty-admin-next"}, nil)
	err := c.SeedWAFRules(context.Background(), "prod")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "background") || strings.Contains(msg, "<style>") {
		t.Fatalf("markup leaked into the error: %s", msg)
	}
	if !strings.Contains(msg, "Server Error | WSL Proxy") {
		t.Fatalf("title should survive: %s", msg)
	}
	if !strings.Contains(msg, "HTTP 500") || !strings.Contains(msg, "/api/waf_rules/seed") {
		t.Fatalf("status and path should survive: %s", msg)
	}
	if len(msg) > 300 {
		t.Fatalf("error is %d bytes, want one readable line: %s", len(msg), msg)
	}
}

func TestLongNonHTMLErrorIsTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.Repeat("stack frame ", 500), http.StatusBadGateway)
	}))
	defer srv.Close()

	c := NewClient(Config{Enabled: true, BaseURL: srv.URL, APIToken: "t", Platform: "openresty-admin-next"}, nil)
	err := c.SeedWAFRules(context.Background(), "prod")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "more bytes)") {
		t.Fatalf("expected a truncation note: %s", msg)
	}
	if len(msg) > 600 {
		t.Fatalf("error is %d bytes: %s", len(msg), msg)
	}
}
