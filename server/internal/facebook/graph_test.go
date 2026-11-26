package facebook

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// roundTrip serves requests from a handler in memory, so the tests need no
// sockets.
type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testClient(h http.HandlerFunc) (*Client, func()) {
	c := NewClient("v99.0")
	c.Base = "https://graph.test"
	c.HTTP = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec.Result(), nil
	})}
	return c, func() {}
}

func TestAuthURLCarriesScopesAndState(t *testing.T) {
	u := NewClient("v99.0").AuthURL("app1", "https://x.test/cb", "st8")
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if !strings.Contains(parsed.Path, "/v99.0/dialog/oauth") || q.Get("state") != "st8" || q.Get("client_id") != "app1" {
		t.Errorf("url = %s", u)
	}
	for _, s := range []string{"pages_manage_posts", "pages_show_list"} {
		if !strings.Contains(q.Get("scope"), s) {
			t.Errorf("scope %q missing from %q", s, q.Get("scope"))
		}
	}
}

func TestCreatePostFeedAndPhoto(t *testing.T) {
	var path, auth, form string
	c, done := testClient(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		form = string(b)
		if strings.HasSuffix(r.URL.Path, "/photos") {
			_, _ = w.Write([]byte(`{"id":"photo1","post_id":"123_456"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"123_789"}`))
	})
	defer done()

	id, err := c.CreatePost(context.Background(), "123", "page-token-x", Publish{Message: "hello"})
	if err != nil || id != "123_789" {
		t.Fatalf("feed: id=%q err=%v", id, err)
	}
	if path != "/v99.0/123/feed" || auth != "Bearer page-token-x" || !strings.Contains(form, "message=hello") {
		t.Errorf("feed request: path=%s auth=%s form=%s", path, auth, form)
	}
	if strings.Contains(form, "page-token-x") {
		t.Error("token leaked into the request body")
	}

	id, err = c.CreatePost(context.Background(), "123", "page-token-x", Publish{Message: "hi", ImageURL: "https://img.test/a.png"})
	if err != nil || id != "123_456" {
		t.Fatalf("photo: id=%q err=%v (the feed post_id, not the photo id)", id, err)
	}
	if path != "/v99.0/123/photos" || !strings.Contains(form, "caption=hi") || !strings.Contains(form, "url=https") {
		t.Errorf("photo request: path=%s form=%s", path, form)
	}
}

func TestErrorsMapToSentinels(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   error
	}{
		{400, `{"error":{"message":"Error validating access token","code":190}}`, ErrTokenExpired},
		{403, `{"error":{"message":"(#200) requires pages_manage_posts","code":200}}`, ErrPermission},
		{401, `nope`, ErrTokenExpired},
	}
	for _, tc := range cases {
		c, done := testClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})
		_, err := c.CreatePost(context.Background(), "1", "tok", Publish{Message: "m"})
		done()
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: err = %v, want %v", tc.status, err, tc.want)
		}
	}

	c, done := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":{"message":"boom","code":2}}`))
	})
	defer done()
	if _, err := c.CreatePost(context.Background(), "1", "tok", Publish{Message: "m"}); err == nil || errors.Is(err, ErrTokenExpired) {
		t.Errorf("generic failure mapped wrongly: %v", err)
	}
}

func TestNoAnswerIsDistinctAndRedactsToken(t *testing.T) {
	c := NewClient("v99.0")
	c.HTTP = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("connection reset while sending secret-token-value")
	})}
	_, err := c.CreatePost(context.Background(), "1", "secret-token-value", Publish{Message: "m"})
	if !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "secret-token-value") {
		t.Error("token in error text")
	}
}

func TestExchangeFallsBackToShortToken(t *testing.T) {
	calls := 0
	c, done := testClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"access_token":"short","expires_in":3600}`))
			return
		}
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"no","code":1}}`))
	})
	defer done()
	tok, err := c.Exchange(context.Background(), "app", "sec", "https://x/cb", "code")
	if err != nil || tok.AccessToken != "short" || tok.Expiry.IsZero() {
		t.Errorf("tok=%+v err=%v", tok, err)
	}
}

func TestPostStatsAndComments(t *testing.T) {
	c, done := testClient(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/comments") {
			_, _ = w.Write([]byte(`{"data":[{"id":"1_2_3","message":"nice","created_time":"2026-01-02T15:04:05+0000","from":{"name":"Ann"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"shares":{"count":2},"reactions":{"summary":{"total_count":9}},"comments":{"summary":{"total_count":4}}}`))
	})
	defer done()
	st, err := c.PostStats(context.Background(), "1_2", "t")
	if err != nil || st != (PostStats{Reactions: 9, Comments: 4, Shares: 2}) {
		t.Errorf("stats=%+v err=%v", st, err)
	}
	cs, err := c.Comments(context.Background(), "1_2", "t", 10)
	if err != nil || len(cs) != 1 || cs[0].From != "Ann" || cs[0].Created.IsZero() {
		t.Errorf("comments=%+v err=%v", cs, err)
	}
}

func TestRedactLeavesShortTokensAlone(t *testing.T) {
	if got := redact("a test string", "t"); got != "a test string" {
		t.Errorf("redact mangled the text: %q", got)
	}
	if got := redact("with secret-token-value inside", "secret-token-value"); got != "with [token] inside" {
		t.Errorf("redact = %q", got)
	}
}
