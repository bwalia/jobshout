package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// DefaultAPIVersion is the LinkedIn-Version sent to the REST Posts API when
// LINKEDIN_API_VERSION is unset.
const DefaultAPIVersion = "202509"

// Scopes are what the connect flow asks for: OpenID Connect to learn who the
// member is, and w_member_social to post as them. All three come with the
// self-serve "Sign In with LinkedIn using OpenID Connect" and "Share on
// LinkedIn" products.
var Scopes = []string{"openid", "profile", "w_member_social"}

var (
	authURL     = "https://www.linkedin.com/oauth/v2/authorization"
	tokenURL    = "https://www.linkedin.com/oauth/v2/accessToken"
	userInfoURL = "https://api.linkedin.com/v2/userinfo"
	postsURL    = "https://api.linkedin.com/rest/posts"
)

// ErrTokenExpired means LinkedIn rejected the stored token. Members get a
// 60-day token with no refresh, so only connecting again restores posting.
var ErrTokenExpired = errors.New("linkedin: the LinkedIn connection has expired or was revoked — connect LinkedIn again")

// ErrNoAnswer means the request got no HTTP response (timeout, connection
// reset). For a post that is ambiguous: LinkedIn may have published it.
var ErrNoAnswer = errors.New("linkedin: no answer from LinkedIn")

// AuthURL is the LinkedIn consent page for the connect flow.
func AuthURL(clientID, redirectURL, state string) string {
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {clientID},
		"redirect_uri":  {redirectURL},
		"state":         {state},
		"scope":         {strings.Join(Scopes, " ")},
	}
	return authURL + "?" + q.Encode()
}

// Token is an access token and when it expires.
type Token struct {
	AccessToken string
	Expiry      time.Time
}

// Member is who a token belongs to.
type Member struct {
	// URN is urn:li:person:<sub>, the author of the member's posts.
	URN  string
	Name string
}

// Client calls LinkedIn. HTTP is injectable for tests.
type Client struct {
	HTTP       *http.Client
	APIVersion string
}

// NewClient returns a client with a sane timeout.
func NewClient(apiVersion string) *Client {
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, APIVersion: apiVersion}
}

// Exchange trades an authorization code for an access token.
func (c *Client) Exchange(ctx context.Context, clientID, clientSecret, redirectURL, code string) (Token, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURL},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, status, _, err := c.do(req)
	if err != nil {
		return Token{}, err
	}
	var tj struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tj); err != nil {
		return Token{}, fmt.Errorf("linkedin: token response: status %d", status)
	}
	if tj.Error != "" || status >= 400 || tj.AccessToken == "" {
		desc := tj.ErrorDesc
		if desc == "" {
			desc = tj.Error
		}
		if desc == "" {
			desc = fmt.Sprintf("status %d", status)
		}
		return Token{}, fmt.Errorf("linkedin: token exchange: %s", desc)
	}
	exp := time.Duration(tj.ExpiresIn) * time.Second
	if exp <= 0 {
		exp = 60 * 24 * time.Hour
	}
	return Token{AccessToken: tj.AccessToken, Expiry: time.Now().Add(exp)}, nil
}

// Me returns the member a token belongs to.
func (c *Client) Me(ctx context.Context, accessToken string) (Member, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return Member{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	body, status, _, err := c.do(req)
	if err != nil {
		return Member{}, err
	}
	if status == http.StatusUnauthorized {
		return Member{}, ErrTokenExpired
	}
	if status >= 400 {
		return Member{}, fmt.Errorf("linkedin: userinfo: status %d", status)
	}
	var u struct {
		Sub  string `json:"sub"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &u); err != nil || u.Sub == "" {
		return Member{}, errors.New("linkedin: userinfo: no member id in the reply")
	}
	return Member{URN: "urn:li:person:" + u.Sub, Name: u.Name}, nil
}

// Post is what CreatePost publishes.
type Post struct {
	AuthorURN  string
	Commentary string // plain text; escaped for LinkedIn here
	// Link, when set, is attached as an article card.
	LinkURL   string
	LinkTitle string
}

// CreatePost publishes a post and returns its URN.
func (c *Client) CreatePost(ctx context.Context, accessToken string, p Post) (string, error) {
	body := map[string]any{
		"author":     p.AuthorURN,
		"commentary": EscapeCommentary(p.Commentary),
		"visibility": "PUBLIC",
		"distribution": map[string]any{
			"feedDistribution":               "MAIN_FEED",
			"targetEntities":                 []any{},
			"thirdPartyDistributionChannels": []any{},
		},
		"lifecycleState":            "PUBLISHED",
		"isReshareDisabledByAuthor": false,
	}
	if p.LinkURL != "" {
		article := map[string]any{"source": p.LinkURL}
		if p.LinkTitle != "" {
			article["title"] = p.LinkTitle
		}
		body["content"] = map[string]any{"article": article}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, postsURL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("LinkedIn-Version", c.APIVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	resp, status, hdr, err := c.do(req)
	if err != nil {
		return "", err
	}
	if status == http.StatusUnauthorized {
		return "", ErrTokenExpired
	}
	if status >= 300 {
		return "", fmt.Errorf("linkedin: create post: status %d: %s", status, apiMessage(resp))
	}
	urn := hdr.Get("X-Restli-Id")
	if urn == "" {
		urn = hdr.Get("X-LinkedIn-Id")
	}
	return urn, nil
}

// PostURL is the public URL of a post URN.
func PostURL(urn string) string {
	if urn == "" {
		return ""
	}
	return "https://www.linkedin.com/feed/update/" + urn + "/"
}

func (c *Client) do(req *http.Request) ([]byte, int, http.Header, error) {
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("%w: %s %s: %v", ErrNoAnswer, req.Method, req.URL.Host, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, resp.Header, fmt.Errorf("linkedin: read response: %w", err)
	}
	return body, resp.StatusCode, resp.Header, nil
}

// apiMessage pulls LinkedIn's error message out of a reply, bounded so a
// stray HTML page does not end up in the UI.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &e) == nil && e.Message != "" {
		return e.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// littleTextReserved are the characters LinkedIn's "little text" commentary
// format treats as markup. Unescaped, they can cut a post short or turn text
// into a mention or a template.
const littleTextReserved = `\|{}@[]()<>#*_~`

// EscapeCommentary escapes plain text for a post's commentary. A #word that
// starts a word becomes LinkedIn's hashtag template, so it is a real hashtag
// rather than an escaped "#".
func EscapeCommentary(s string) string {
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '#' && (i == 0 || unicode.IsSpace(rs[i-1])) {
			j := i + 1
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j])) {
				j++
			}
			if j > i+1 {
				b.WriteString(`{hashtag|\#|` + string(rs[i+1:j]) + `}`)
				i = j - 1
				continue
			}
		}
		if strings.ContainsRune(littleTextReserved, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
