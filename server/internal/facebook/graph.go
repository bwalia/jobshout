package facebook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Scopes are what the connect flow asks for. Meta reviews each of these for
// Pages the app's users do not own, so an unreviewed app only reaches Pages
// its own roles administer — that is Meta's rule, not something to work around.
var Scopes = []string{
	"pages_show_list",
	"pages_read_engagement",
	"pages_manage_posts",
	"pages_manage_engagement",
	"pages_manage_metadata",
}

var (
	graphHost  = "https://graph.facebook.com"
	dialogHost = "https://www.facebook.com"
)

var (
	// ErrTokenExpired means Meta rejected the stored token (error code 190):
	// it expired, was revoked, or the user changed their password.
	ErrTokenExpired = errors.New("facebook: the Facebook connection has expired or was revoked — connect Facebook again")
	// ErrNoAnswer means no HTTP response arrived. For a publish that is
	// ambiguous: Facebook may have created the post anyway.
	ErrNoAnswer = errors.New("facebook: no answer from Facebook")
	// ErrPermission means the token lacks a permission the call needs.
	ErrPermission = errors.New("facebook: this connection does not have the permission this needs — connect Facebook again and accept every permission")
)

// Client calls the Graph API. HTTP and Base are injectable for tests.
type Client struct {
	HTTP    *http.Client
	Version string
	// Base overrides the Graph host in tests.
	Base string
}

// NewClient returns a client with a sane timeout.
func NewClient(version string) *Client {
	if version == "" {
		version = DefaultGraphVersion
	}
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, Version: version}
}

func (c *Client) base() string {
	if c.Base != "" {
		return strings.TrimRight(c.Base, "/")
	}
	return graphHost
}

// AuthURL is the Facebook consent dialog for the connect flow.
func (c *Client) AuthURL(appID, redirectURL, state string) string {
	q := url.Values{
		"client_id":     {appID},
		"redirect_uri":  {redirectURL},
		"state":         {state},
		"scope":         {strings.Join(Scopes, ",")},
		"response_type": {"code"},
	}
	return dialogHost + "/" + c.Version + "/dialog/oauth?" + q.Encode()
}

// Token is an access token and when it expires (zero: no stated expiry).
type Token struct {
	AccessToken string
	Expiry      time.Time
}

// Exchange trades an authorization code for a user token, then for a
// long-lived one. Page tokens read with a long-lived user token do not expire.
func (c *Client) Exchange(ctx context.Context, appID, appSecret, redirectURL, code string) (Token, error) {
	short, err := c.token(ctx, url.Values{
		"client_id":     {appID},
		"client_secret": {appSecret},
		"redirect_uri":  {redirectURL},
		"code":          {code},
	})
	if err != nil {
		return Token{}, err
	}
	long, err := c.token(ctx, url.Values{
		"grant_type":        {"fb_exchange_token"},
		"client_id":         {appID},
		"client_secret":     {appSecret},
		"fb_exchange_token": {short.AccessToken},
	})
	if err != nil {
		// A short-lived token still lists Pages; keep it rather than fail.
		return short, nil
	}
	return long, nil
}

func (c *Client) token(ctx context.Context, q url.Values) (Token, error) {
	var tj struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := c.get(ctx, "/oauth/access_token", "", q, &tj); err != nil {
		return Token{}, err
	}
	if tj.AccessToken == "" {
		return Token{}, errors.New("facebook: token exchange returned no token")
	}
	t := Token{AccessToken: tj.AccessToken}
	if tj.ExpiresIn > 0 {
		t.Expiry = time.Now().Add(time.Duration(tj.ExpiresIn) * time.Second)
	}
	return t, nil
}

// User is who a user token belongs to.
type User struct{ ID, Name string }

// Me returns the Facebook user a token belongs to.
func (c *Client) Me(ctx context.Context, token string) (User, error) {
	var u struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.get(ctx, "/me", token, url.Values{"fields": {"id,name"}}, &u); err != nil {
		return User{}, err
	}
	return User{ID: u.ID, Name: u.Name}, nil
}

// Page is a Facebook Page the user administers, with its own access token.
type Page struct {
	ID          string
	Name        string
	Category    string
	About       string
	Description string
	Website     string
	Followers   int
	AccessToken string
}

type pageJSON struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	About          string `json:"about"`
	Description    string `json:"description"`
	Website        string `json:"website"`
	FollowersCount int    `json:"followers_count"`
	AccessToken    string `json:"access_token"`
}

func (p pageJSON) page() Page {
	return Page{ID: p.ID, Name: p.Name, Category: p.Category, About: p.About, Description: p.Description,
		Website: p.Website, Followers: p.FollowersCount, AccessToken: p.AccessToken}
}

const pageFields = "id,name,category,about,description,website,followers_count"

// Accounts lists the Pages the user manages, with their Page access tokens.
func (c *Client) Accounts(ctx context.Context, userToken string) ([]Page, error) {
	var out []Page
	next := url.Values{"fields": {pageFields + ",access_token"}, "limit": {"100"}}
	for i := 0; i < 5; i++ { // 500 Pages is more than any one org manages
		var r struct {
			Data   []pageJSON `json:"data"`
			Paging struct {
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := c.get(ctx, "/me/accounts", userToken, next, &r); err != nil {
			return nil, err
		}
		for _, p := range r.Data {
			out = append(out, p.page())
		}
		if r.Paging.Next == "" || r.Paging.Cursors.After == "" {
			break
		}
		next.Set("after", r.Paging.Cursors.After)
	}
	return out, nil
}

// PageInfo reads a Page's current details.
func (c *Client) PageInfo(ctx context.Context, pageID, pageToken string) (Page, error) {
	var p pageJSON
	if err := c.get(ctx, "/"+pageID, pageToken, url.Values{"fields": {pageFields}}, &p); err != nil {
		return Page{}, err
	}
	return p.page(), nil
}

// PageUpdate is the Page details Meta lets an app change. Name, profile and
// cover images are not editable through the Graph API for most Pages; those
// stay a manual step in Meta Business Suite.
type PageUpdate struct {
	About       *string
	Description *string
	Website     *string
}

// UpdatePage changes a Page's editable details.
func (c *Client) UpdatePage(ctx context.Context, pageID, pageToken string, u PageUpdate) error {
	f := url.Values{}
	if u.About != nil {
		f.Set("about", *u.About)
	}
	if u.Description != nil {
		f.Set("description", *u.Description)
	}
	if u.Website != nil {
		f.Set("website", *u.Website)
	}
	if len(f) == 0 {
		return nil
	}
	return c.post(ctx, "/"+pageID, pageToken, f, nil)
}

// Publish is what a Page publishes.
type Publish struct {
	Message string
	// ImageURL, when set, is published as a photo post. Facebook fetches it,
	// so it must be publicly reachable.
	ImageURL string
}

// CreatePost publishes to a Page's feed and returns the Facebook post ID.
func (c *Client) CreatePost(ctx context.Context, pageID, pageToken string, p Publish) (string, error) {
	f := url.Values{"published": {"true"}}
	path := "/" + pageID + "/feed"
	if p.ImageURL != "" {
		path = "/" + pageID + "/photos"
		f.Set("url", p.ImageURL)
		f.Set("caption", p.Message)
	} else {
		f.Set("message", p.Message)
	}
	var r struct {
		ID     string `json:"id"`
		PostID string `json:"post_id"`
	}
	if err := c.post(ctx, path, pageToken, f, &r); err != nil {
		return "", err
	}
	// Photo posts return the photo ID as id and the feed post as post_id.
	id := r.PostID
	if id == "" {
		id = r.ID
	}
	if id == "" {
		return "", errors.New("facebook: publish returned no post ID")
	}
	return id, nil
}

// EditPost changes a published post's text.
func (c *Client) EditPost(ctx context.Context, postID, pageToken, message string) error {
	return c.post(ctx, "/"+postID, pageToken, url.Values{"message": {message}}, nil)
}

// DeletePost removes a published post.
func (c *Client) DeletePost(ctx context.Context, postID, pageToken string) error {
	return c.do(ctx, http.MethodDelete, "/"+postID, pageToken, nil, nil)
}

// PostStats is a post's engagement counts.
type PostStats struct {
	Reactions int
	Comments  int
	Shares    int
}

// PostStats reads a post's reaction, comment and share counts.
func (c *Client) PostStats(ctx context.Context, postID, pageToken string) (PostStats, error) {
	var r struct {
		Shares    struct{ Count int } `json:"shares"`
		Reactions struct {
			Summary struct {
				TotalCount int `json:"total_count"`
			} `json:"summary"`
		} `json:"reactions"`
		Comments struct {
			Summary struct {
				TotalCount int `json:"total_count"`
			} `json:"summary"`
		} `json:"comments"`
	}
	q := url.Values{"fields": {"shares,reactions.summary(true).limit(0),comments.summary(true).limit(0)"}}
	if err := c.get(ctx, "/"+postID, pageToken, q, &r); err != nil {
		return PostStats{}, err
	}
	return PostStats{Reactions: r.Reactions.Summary.TotalCount, Comments: r.Comments.Summary.TotalCount, Shares: r.Shares.Count}, nil
}

// Comment is one comment on a post.
type Comment struct {
	ID      string    `json:"id"`
	From    string    `json:"from"`
	Message string    `json:"message"`
	Created time.Time `json:"created_time"`
}

// Comments lists the newest comments on a post.
func (c *Client) Comments(ctx context.Context, postID, pageToken string, limit int) ([]Comment, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	var r struct {
		Data []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Created string `json:"created_time"`
			From    struct {
				Name string `json:"name"`
			} `json:"from"`
		} `json:"data"`
	}
	q := url.Values{"fields": {"id,message,created_time,from"}, "order": {"reverse_chronological"}, "limit": {strconv.Itoa(limit)}}
	if err := c.get(ctx, "/"+postID+"/comments", pageToken, q, &r); err != nil {
		return nil, err
	}
	out := make([]Comment, 0, len(r.Data))
	for _, d := range r.Data {
		t, _ := time.Parse("2006-01-02T15:04:05-0700", d.Created)
		out = append(out, Comment{ID: d.ID, From: d.From.Name, Message: d.Message, Created: t})
	}
	return out, nil
}

// ReplyToComment posts a Page reply under a comment and returns its ID.
func (c *Client) ReplyToComment(ctx context.Context, commentID, pageToken, message string) (string, error) {
	var r struct {
		ID string `json:"id"`
	}
	if err := c.post(ctx, "/"+commentID+"/comments", pageToken, url.Values{"message": {message}}, &r); err != nil {
		return "", err
	}
	return r.ID, nil
}

// PostURL is the public URL of a Facebook post ID (pageID_postID).
func PostURL(postID string) string {
	if postID == "" {
		return ""
	}
	return "https://www.facebook.com/" + postID
}

func (c *Client) get(ctx context.Context, path, token string, q url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, token, q, out)
}

func (c *Client) post(ctx context.Context, path, token string, form url.Values, out any) error {
	return c.do(ctx, http.MethodPost, path, token, form, out)
}

// do sends the request. The token travels in a bearer header, never the URL,
// so it cannot leak into logs or error text.
func (c *Client) do(ctx context.Context, method, path, token string, params url.Values, out any) error {
	u := c.base() + "/" + c.Version + path
	var body io.Reader
	if method == http.MethodGet || method == http.MethodDelete {
		if len(params) > 0 {
			u += "?" + params.Encode()
		}
	} else {
		body = strings.NewReader(params.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s %s: %v", ErrNoAnswer, method, req.URL.Host, redact(err.Error(), token))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("facebook: read response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return apiError(resp.StatusCode, raw)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("facebook: decode response: %w", err)
	}
	return nil
}

func redact(s, token string) string {
	// A real token is long; replacing a short string would mangle the text.
	if len(token) < 8 {
		return s
	}
	return strings.ReplaceAll(s, token, "[token]")
}

// apiError maps a Graph error body onto the sentinel errors callers act on.
func apiError(status int, raw []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	switch {
	case e.Error.Code == 190 || status == http.StatusUnauthorized:
		return ErrTokenExpired
	case e.Error.Code == 10 || (e.Error.Code >= 200 && e.Error.Code <= 299):
		return fmt.Errorf("%w (%s)", ErrPermission, trim(e.Error.Message))
	}
	msg := e.Error.Message
	if msg == "" {
		msg = strings.TrimSpace(string(raw))
	}
	return fmt.Errorf("facebook: status %d: %s", status, trim(msg))
}

func trim(s string) string {
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
