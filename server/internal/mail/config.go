package mail

import (
	"os"
	"strings"
	"time"
)

// Config is the Mail Agent's process settings. Empty ClientID/Secret/TokenKey
// means Gmail OAuth is off — the builtin agent still seeds, Connect fails
// clearly.
type Config struct {
	ClientID          string
	ClientSecret      string
	RedirectURL       string
	TokenKey          string
	FrontendBaseURL   string
	PollInterval      time.Duration
	ReconcileInterval time.Duration
	// Simulate is local-only: a fake inbox, no Google. Set MAIL_SIMULATE=1.
	Simulate bool
}

// LoadConfig reads GMAIL_* / MAIL_* / FRONTEND_BASE_URL from the environment.
func LoadConfig() Config {
	c := Config{
		ClientID:        strings.TrimSpace(os.Getenv("GMAIL_CLIENT_ID")),
		ClientSecret:    strings.TrimSpace(os.Getenv("GMAIL_CLIENT_SECRET")),
		RedirectURL:     strings.TrimSpace(os.Getenv("GMAIL_OAUTH_REDIRECT_URL")),
		TokenKey:        strings.TrimSpace(os.Getenv("GMAIL_TOKEN_KEY")),
		FrontendBaseURL: strings.TrimSpace(os.Getenv("FRONTEND_BASE_URL")),
	}
	if c.FrontendBaseURL == "" {
		c.FrontendBaseURL = "http://localhost:3001"
	}
	if c.RedirectURL == "" {
		// Same host as the API in local docker / cluster nginx gateway.
		c.RedirectURL = strings.TrimRight(c.FrontendBaseURL, "/") + "/api/v1/mail/connection/oauth/callback"
	}
	c.PollInterval = envDuration("MAIL_POLL_INTERVAL", 5*time.Minute)
	c.ReconcileInterval = envDuration("MAIL_RECONCILE_INTERVAL", 15*time.Second)
	c.Simulate = SimulateEnabled()
	if c.Simulate {
		if c.ClientID == "" {
			c.ClientID = "simulate"
		}
		if c.ClientSecret == "" {
			c.ClientSecret = "simulate"
		}
		if c.TokenKey == "" {
			c.TokenKey = "mail-simulate-local-only-not-for-production"
		}
	}
	return c
}

// SimulateEnabled is true when MAIL_SIMULATE is 1/true/yes.
func SimulateEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MAIL_SIMULATE")))
	return v == "1" || v == "true" || v == "yes"
}

// Configured reports whether an operator has supplied the OAuth client and
// token-encryption key. Without these, Connect cannot start.
func (c Config) Configured() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.TokenKey != "" && c.RedirectURL != ""
}

func envDuration(key string, fallback time.Duration) time.Duration {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
