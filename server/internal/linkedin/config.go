package linkedin

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the LinkedIn Poster's own configuration, read from LINKEDIN_*
// environment variables so the platform config does not grow a field per
// agent. The client secret and token key come from an extraSecretRefs Secret.
type Config struct {
	ClientID     string
	ClientSecret string
	// RedirectURL is the OAuth callback registered on the LinkedIn app.
	// Defaults to FRONTEND_BASE_URL + /api/v1/linkedin/oauth/callback.
	RedirectURL string
	// TokenKey seals stored access tokens (64 hex chars, or any passphrase).
	TokenKey        string
	FrontendBaseURL string
	// APIVersion is the LinkedIn-Version header for the REST Posts API
	// (YYYYMM). LinkedIn retires versions after about a year.
	APIVersion string
	// Model overrides the drafting model. Empty uses the provider default.
	Model string
	// Provider is the server's LLM provider (LLM_PROVIDER), set by main.
	Provider string
	// AutoDraft drafts posts for newly published articles without a launch,
	// for orgs that have connected LinkedIn.
	AutoDraft bool
	// PollInterval is how often the auto-drafter looks for new articles.
	PollInterval time.Duration
	// Lookback bounds auto-drafting to articles published this recently, so
	// connecting LinkedIn does not draft posts for every old article.
	Lookback time.Duration
	// DraftTimeout caps one article's drafting (both variants).
	DraftTimeout time.Duration
	// InsightsSiteURL is jobshout.com for this ring; with an article's
	// Insights slug it is the link a post carries. Set by main.
	InsightsSiteURL string
}

// LoadConfig reads LINKEDIN_* variables with safe defaults.
func LoadConfig() Config {
	c := Config{
		ClientID:        strings.TrimSpace(os.Getenv("LINKEDIN_CLIENT_ID")),
		ClientSecret:    strings.TrimSpace(os.Getenv("LINKEDIN_CLIENT_SECRET")),
		RedirectURL:     strings.TrimSpace(os.Getenv("LINKEDIN_OAUTH_REDIRECT_URL")),
		TokenKey:        strings.TrimSpace(os.Getenv("LINKEDIN_TOKEN_KEY")),
		FrontendBaseURL: strings.TrimSpace(os.Getenv("FRONTEND_BASE_URL")),
		APIVersion:      strings.TrimSpace(os.Getenv("LINKEDIN_API_VERSION")),
		Model:           strings.TrimSpace(os.Getenv("LINKEDIN_MODEL")),
		AutoDraft:       boolEnv("LINKEDIN_AUTO_DRAFT", true),
		PollInterval:    durationEnv("LINKEDIN_POLL_INTERVAL", 5*time.Minute),
		Lookback:        durationEnv("LINKEDIN_LOOKBACK", 72*time.Hour),
		DraftTimeout:    durationEnv("LINKEDIN_DRAFT_TIMEOUT", 10*time.Minute),
	}
	if c.FrontendBaseURL == "" {
		c.FrontendBaseURL = "http://localhost:3001"
	}
	if c.RedirectURL == "" {
		// Same host as the API behind the nginx gateway, as for Gmail.
		c.RedirectURL = strings.TrimRight(c.FrontendBaseURL, "/") + "/api/v1/linkedin/oauth/callback"
	}
	if c.APIVersion == "" {
		c.APIVersion = DefaultAPIVersion
	}
	return c
}

// Configured reports whether people can connect LinkedIn and post. Drafting
// works without it.
func (c Config) Configured() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.TokenKey != ""
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

func boolEnv(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
