package googleauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
)

// Profile is the Google identity we persist. Sub is stable across email changes.
type Profile struct {
	Sub           string
	Email         string
	Name          string
	Picture       string
	EmailVerified bool
}

// Identity talks to Google for the login/signup OAuth code flow.
type Identity interface {
	AuthURL(state string) string
	ProfileFromCode(ctx context.Context, code string) (Profile, error)
}

type client struct {
	cfg  Config
	http *http.Client
}

// NewClient talks to Google over HTTP. Tests inject a client pointed at httptest.
func NewClient(cfg Config, httpClient *http.Client) Identity {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &client{cfg: cfg, http: httpClient}
}

// AuthURL builds the Google consent URL. select_account lets the user pick
// which Google identity to use. We do not request offline access: JobShout
// issues its own JWTs and never stores a Google refresh token for login.
func AuthURL(clientID, redirectURL, state string) string {
	q := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURL},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"prompt":        {"select_account"},
		"state":         {state},
	}
	return googleAuthURL + "?" + q.Encode()
}

func (c *client) AuthURL(state string) string {
	return AuthURL(c.cfg.ClientID, c.cfg.RedirectURL, state)
}

func (c *client) ProfileFromCode(ctx context.Context, code string) (Profile, error) {
	accessToken, err := c.exchangeCode(ctx, code)
	if err != nil {
		return Profile{}, err
	}
	return c.userInfo(ctx, accessToken)
}

func (c *client) exchangeCode(ctx context.Context, code string) (string, error) {
	body, status, err := postForm(ctx, c.http, googleTokenURL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.cfg.RedirectURL},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
	})
	if err != nil {
		return "", err
	}
	var tj struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tj); err != nil {
		return "", fmt.Errorf("googleauth: token response: status %d", status)
	}
	if tj.Error != "" || status >= 400 {
		desc := tj.ErrorDesc
		if desc == "" {
			desc = tj.Error
		}
		if desc == "" {
			desc = fmt.Sprintf("status %d", status)
		}
		return "", fmt.Errorf("googleauth: token exchange: %s", desc)
	}
	if tj.AccessToken == "" {
		return "", fmt.Errorf("googleauth: token exchange: empty access token")
	}
	return tj.AccessToken, nil
}

func (c *client) userInfo(ctx context.Context, accessToken string) (Profile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return Profile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return Profile{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Profile{}, err
	}
	if resp.StatusCode >= 400 {
		return Profile{}, fmt.Errorf("googleauth: userinfo: status %d", resp.StatusCode)
	}
	var raw struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Profile{}, fmt.Errorf("googleauth: userinfo: %w", err)
	}
	p := Profile{
		Sub:           strings.TrimSpace(raw.Sub),
		Email:         strings.ToLower(strings.TrimSpace(raw.Email)),
		Name:          strings.TrimSpace(raw.Name),
		Picture:       strings.TrimSpace(raw.Picture),
		EmailVerified: raw.EmailVerified,
	}
	if p.Sub == "" || p.Email == "" {
		return Profile{}, fmt.Errorf("googleauth: userinfo missing sub or email")
	}
	if !p.EmailVerified {
		return Profile{}, ErrEmailNotVerified
	}
	return p, nil
}

func postForm(ctx context.Context, client *http.Client, rawURL string, form url.Values) ([]byte, int, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// ErrEmailNotVerified is returned when Google says the address is unverified.
var ErrEmailNotVerified = fmt.Errorf("googleauth: email is not verified")
