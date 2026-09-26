package googleauth

import (
	"os"
	"strings"
)

// Config is the platform Google login/signup OAuth client. Empty ClientID
// disables the flow — email/password auth is unaffected.
//
// This is not Gmail Mail Agent OAuth (GMAIL_*) and not org-scoped SSO.
type Config struct {
	ClientID        string
	ClientSecret    string
	RedirectURL     string
	FrontendBaseURL string
}

// LoadConfig reads GOOGLE_OAUTH_* and FRONTEND_BASE_URL from the environment.
func LoadConfig() Config {
	c := Config{
		ClientID:        strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_ID")),
		ClientSecret:    strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")),
		RedirectURL:     strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_REDIRECT_URL")),
		FrontendBaseURL: strings.TrimSpace(os.Getenv("FRONTEND_BASE_URL")),
	}
	if c.FrontendBaseURL == "" {
		c.FrontendBaseURL = "http://localhost:3001"
	}
	if c.RedirectURL == "" {
		c.RedirectURL = "http://localhost:8190/api/v1/auth/google/callback"
	}
	return c
}

// Configured reports whether an operator has supplied a Google OAuth client.
func (c Config) Configured() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.RedirectURL != ""
}
