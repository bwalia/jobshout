package facebook

import (
	"os"
	"strings"
	"time"
)

// Config is the Facebook Page Manager's own configuration, read from
// FACEBOOK_* variables so the platform config does not grow a field per
// agent. The app secret and token key come from an extraSecretRefs Secret
// (Vault jobshout/<ring>/facebook), never from a literal.
type Config struct {
	AppID     string
	AppSecret string
	// RedirectURL is the OAuth callback registered on the Meta app. Defaults
	// to FRONTEND_BASE_URL + /api/v1/facebook/oauth/callback.
	RedirectURL string
	// TokenKey seals stored Page access tokens (64 hex chars, or a passphrase).
	TokenKey        string
	FrontendBaseURL string
	// GraphVersion is the Graph API version in request paths, e.g. v23.0.
	GraphVersion string
	// Model overrides the drafting model. Empty uses the provider default.
	Model string
	// Provider is the server's LLM provider for this agent, set by main.
	Provider string
	// PollInterval is how often due scheduled posts are published.
	PollInterval time.Duration
	// DraftTimeout caps one calendar's drafting.
	DraftTimeout time.Duration
}

// DefaultGraphVersion is used when FACEBOOK_GRAPH_VERSION is unset.
const DefaultGraphVersion = "v23.0"

// LoadConfig reads FACEBOOK_* variables with safe defaults.
func LoadConfig() Config {
	c := Config{
		AppID:           strings.TrimSpace(os.Getenv("FACEBOOK_APP_ID")),
		AppSecret:       strings.TrimSpace(os.Getenv("FACEBOOK_APP_SECRET")),
		RedirectURL:     strings.TrimSpace(os.Getenv("FACEBOOK_OAUTH_REDIRECT_URL")),
		TokenKey:        strings.TrimSpace(os.Getenv("FACEBOOK_TOKEN_KEY")),
		FrontendBaseURL: strings.TrimSpace(os.Getenv("FRONTEND_BASE_URL")),
		GraphVersion:    strings.TrimSpace(os.Getenv("FACEBOOK_GRAPH_VERSION")),
		Model:           strings.TrimSpace(os.Getenv("FACEBOOK_MODEL")),
		PollInterval:    durationEnv("FACEBOOK_POLL_INTERVAL", time.Minute),
		DraftTimeout:    durationEnv("FACEBOOK_DRAFT_TIMEOUT", 10*time.Minute),
	}
	if c.FrontendBaseURL == "" {
		c.FrontendBaseURL = "http://localhost:3001"
	}
	if c.RedirectURL == "" {
		c.RedirectURL = strings.TrimRight(c.FrontendBaseURL, "/") + "/api/v1/facebook/oauth/callback"
	}
	if c.GraphVersion == "" {
		c.GraphVersion = DefaultGraphVersion
	}
	return c
}

// Configured reports whether people can connect Facebook and publish.
// Drafting works without it.
func (c Config) Configured() bool {
	return c.AppID != "" && c.AppSecret != "" && c.TokenKey != ""
}

func durationEnv(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
