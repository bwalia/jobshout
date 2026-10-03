package linkedin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestEscapeCommentary(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain text.", "plain text."},
		{"use (parens) & [brackets]", `use \(parens\) & \[brackets\]`},
		{"email me@example.com", `email me\@example.com`},
		{"a_b*c~d", `a\_b\*c\~d`},
		{"#Kubernetes #DevOps", `{hashtag|\#|Kubernetes} {hashtag|\#|DevOps}`},
		{"line\n#AI", "line\n{hashtag|\\#|AI}"},
		{"issue #42 here", `issue {hashtag|\#|42} here`},
		{"C# and a#b", `C\# and a\#b`},
		{"# alone", `\# alone`},
	}
	for _, c := range cases {
		if got := EscapeCommentary(c.in); got != c.want {
			t.Errorf("EscapeCommentary(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAuthURLAsksForPostingScope(t *testing.T) {
	u, err := url.Parse(AuthURL("cid", "https://int.example/api/v1/linkedin/oauth/callback", "st"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("client_id") != "cid" || q.Get("state") != "st" || q.Get("response_type") != "code" {
		t.Errorf("query = %v", q)
	}
	if !strings.Contains(q.Get("scope"), "w_member_social") || !strings.Contains(q.Get("scope"), "openid") {
		t.Errorf("scope = %q", q.Get("scope"))
	}
}

// withServer points the package's LinkedIn URLs at a test server.
func withServer(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	saved := []string{tokenURL, userInfoURL, postsURL}
	tokenURL, userInfoURL, postsURL = srv.URL+"/token", srv.URL+"/userinfo", srv.URL+"/rest/posts"
	t.Cleanup(func() { tokenURL, userInfoURL, postsURL = saved[0], saved[1], saved[2] })
	return &Client{HTTP: srv.Client(), APIVersion: "202509"}
}

func TestCreatePostSendsVersionedRequestAndReturnsURN(t *testing.T) {
	var got map[string]any
	var hdr http.Header
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		hdr = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("X-Restli-Id", "urn:li:share:123")
		w.WriteHeader(http.StatusCreated)
	})
	urn, err := c.CreatePost(context.Background(), "tok", Post{
		AuthorURN: "urn:li:person:abc", Commentary: "Hello (world) #Go",
		LinkURL: "https://jobshout.com/insights/x", LinkTitle: "X",
	})
	if err != nil {
		t.Fatal(err)
	}
	if urn != "urn:li:share:123" {
		t.Errorf("urn = %q", urn)
	}
	if hdr.Get("Authorization") != "Bearer tok" || hdr.Get("LinkedIn-Version") != "202509" || hdr.Get("X-Restli-Protocol-Version") != "2.0.0" {
		t.Errorf("headers = %v", hdr)
	}
	if got["author"] != "urn:li:person:abc" || got["commentary"] != `Hello \(world\) {hashtag|\#|Go}` || got["visibility"] != "PUBLIC" {
		t.Errorf("body = %v", got)
	}
	article := got["content"].(map[string]any)["article"].(map[string]any)
	if article["source"] != "https://jobshout.com/insights/x" || article["title"] != "X" {
		t.Errorf("article = %v", article)
	}
	if PostURL(urn) != "https://www.linkedin.com/feed/update/urn:li:share:123/" {
		t.Errorf("PostURL = %q", PostURL(urn))
	}
}

func TestCreatePostWithoutLinkHasNoContent(t *testing.T) {
	var got map[string]any
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusCreated)
	})
	if _, err := c.CreatePost(context.Background(), "tok", Post{AuthorURN: "urn:li:person:a", Commentary: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["content"]; ok {
		t.Errorf("content sent without a link: %v", got["content"])
	}
}

func TestCreatePostErrors(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer expired" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"Content is a duplicate"}`))
	})
	_, err := c.CreatePost(context.Background(), "expired", Post{AuthorURN: "a", Commentary: "x"})
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("401: err = %v, want ErrTokenExpired", err)
	}
	_, err = c.CreatePost(context.Background(), "tok", Post{AuthorURN: "a", Commentary: "x"})
	if err == nil || !strings.Contains(err.Error(), "Content is a duplicate") || errors.Is(err, ErrNoAnswer) {
		t.Errorf("422: err = %v", err)
	}
}

func TestNoAnswerIsDistinguishable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	saved := postsURL
	postsURL = srv.URL
	srv.Close() // nothing listening any more
	t.Cleanup(func() { postsURL = saved })
	_, err := (&Client{APIVersion: "202509"}).CreatePost(context.Background(), "tok", Post{AuthorURN: "a", Commentary: "x"})
	if !errors.Is(err, ErrNoAnswer) {
		t.Errorf("err = %v, want ErrNoAnswer", err)
	}
}

func TestExchangeAndMe(t *testing.T) {
	c := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("code") != "good" || r.Form.Get("grant_type") != "authorization_code" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_request","error_description":"bad code"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"at","expires_in":5184000}`))
		case "/userinfo":
			_, _ = w.Write([]byte(`{"sub":"abc123","name":"Ada Lovelace"}`))
		}
	})
	tok, err := c.Exchange(context.Background(), "id", "secret", "https://cb", "good")
	if err != nil || tok.AccessToken != "at" {
		t.Fatalf("Exchange = %+v, %v", tok, err)
	}
	if _, err := c.Exchange(context.Background(), "id", "secret", "https://cb", "bad"); err == nil || !strings.Contains(err.Error(), "bad code") {
		t.Errorf("bad code: err = %v", err)
	}
	m, err := c.Me(context.Background(), "at")
	if err != nil || m.URN != "urn:li:person:abc123" || m.Name != "Ada Lovelace" {
		t.Errorf("Me = %+v, %v", m, err)
	}
}
