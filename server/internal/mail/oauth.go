package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
)

// AuthURL builds the Google consent URL. access_type=offline + prompt=consent
// are required to receive a refresh token on every connect.
func AuthURL(clientID, redirectURL, state string) string {
	q := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURL},
		"response_type": {"code"},
		"scope":         {strings.Join(RequestedScopes(), " ")},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}
	return googleAuthURL + "?" + q.Encode()
}

// ErrGrantRevoked means Google rejected the stored refresh token (OAuth
// error invalid_grant): it expired or was revoked. Retrying cannot succeed;
// only a fresh consent (Reconnect Gmail) restores the mailbox. Refresh tokens
// of a Google OAuth app left in "Testing" expire after 7 days.
var ErrGrantRevoked = errors.New("mail: Google revoked this mailbox's access — reconnect Gmail to resume syncing")

type tokenJSON struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func parseTokenResponse(body []byte, status int) (TokenSet, error) {
	var tj tokenJSON
	if err := json.Unmarshal(body, &tj); err != nil {
		return TokenSet{}, fmt.Errorf("mail: token response: status %d", status)
	}
	if tj.Error == "invalid_grant" {
		return TokenSet{}, ErrGrantRevoked
	}
	if tj.Error != "" || status >= 400 {
		desc := tj.ErrorDesc
		if desc == "" {
			desc = tj.Error
		}
		if desc == "" {
			desc = fmt.Sprintf("status %d", status)
		}
		return TokenSet{}, fmt.Errorf("mail: token exchange: %s", Redact(desc))
	}
	if tj.AccessToken == "" {
		return TokenSet{}, fmt.Errorf("mail: token exchange: empty access token")
	}
	exp := time.Now().Add(time.Duration(tj.ExpiresIn) * time.Second)
	if tj.ExpiresIn <= 0 {
		exp = time.Now().Add(50 * time.Minute)
	}
	return TokenSet{
		AccessToken:  tj.AccessToken,
		RefreshToken: tj.RefreshToken,
		Expiry:       exp,
	}, nil
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
		return nil, 0, RedactErr(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}
