package metalead

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is read from META_* env vars so the platform config does not grow a
// field per agent. App secret and token key come from Vault / extraSecretRefs.
type Config struct {
	AppID           string
	AppSecret       string
	WebhookVerify   string
	TokenKey        string
	GraphVersion    string
	FrontendBaseURL string
	// Model overrides the analysis model. Empty uses the provider default.
	Model string
	// Provider is set by main from agentLLM.
	Provider string
	// MaxLeadsPerLaunch caps how many leads one launch fetches.
	MaxLeadsPerLaunch int
	// AnalyseTimeout caps LLM work for one run.
	AnalyseTimeout time.Duration
}

// LoadConfig reads META_* with safe defaults.
func LoadConfig() Config {
	c := Config{
		AppID:             strings.TrimSpace(os.Getenv("META_APP_ID")),
		AppSecret:         strings.TrimSpace(os.Getenv("META_APP_SECRET")),
		WebhookVerify:     strings.TrimSpace(os.Getenv("META_WEBHOOK_VERIFY_TOKEN")),
		TokenKey:          strings.TrimSpace(os.Getenv("META_TOKEN_KEY")),
		GraphVersion:      strings.TrimSpace(os.Getenv("META_GRAPH_VERSION")),
		FrontendBaseURL:   strings.TrimSpace(os.Getenv("FRONTEND_BASE_URL")),
		Model:             strings.TrimSpace(os.Getenv("META_LEAD_MODEL")),
		MaxLeadsPerLaunch: intEnv("META_LEAD_MAX_LEADS", 50),
		AnalyseTimeout:    durationEnv("META_LEAD_ANALYSE_TIMEOUT", 8*time.Minute),
	}
	if c.GraphVersion == "" {
		c.GraphVersion = "v21.0"
	}
	if c.FrontendBaseURL == "" {
		c.FrontendBaseURL = "http://localhost:3001"
	}
	if c.MaxLeadsPerLaunch <= 0 {
		c.MaxLeadsPerLaunch = 50
	}
	if c.MaxLeadsPerLaunch > 200 {
		c.MaxLeadsPerLaunch = 200
	}
	return c
}

// Configured reports whether webhooks and token sealing can run. Drafting
// analysis still needs an LLM; Graph pulls need a Page connection token.
func (c Config) Configured() bool {
	return c.AppSecret != "" && c.TokenKey != "" && c.WebhookVerify != ""
}

// CanSealTokens is true when Page access tokens can be stored encrypted.
func (c Config) CanSealTokens() bool {
	return c.TokenKey != ""
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

func intEnv(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
