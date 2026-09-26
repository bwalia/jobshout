package reviewbot

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	DefaultTimeout      = 60 * time.Second
	DefaultPollInterval = 15 * time.Second
	DefaultMaxRuntime   = 40 * time.Minute
)

type Config struct {
	Enabled bool
	// BaseURL is the in-cluster sidecar, e.g. http://jobshout-review-bot:8765.
	// Empty means the feature is off for this ring.
	BaseURL string
	// Token is REVIEW_BOT_TOKEN, sent as Authorization: Bearer. Distinct from
	// OLLAMA_JWT_SECRET — that signs workstation model calls; this is the
	// sidecar's own shared secret on the cluster network.
	Token        string
	Timeout      time.Duration
	PollInterval time.Duration
	MaxRuntime   time.Duration
	AllowedRepos []string
}

func (c Config) Configured() bool { return c.Enabled && c.BaseURL != "" }

func LoadConfig(logger *zap.Logger) Config {
	enabled := os.Getenv("REVIEW_BOT_ENABLED")
	return Config{
		Enabled:      enabled == "true" || enabled == "1",
		BaseURL:      strings.TrimRight(os.Getenv("REVIEW_BOT_BASE_URL"), "/"),
		Token:        strings.TrimSpace(os.Getenv("REVIEW_BOT_TOKEN")),
		Timeout:      durationOrDefault("REVIEW_BOT_TIMEOUT", DefaultTimeout, logger),
		PollInterval: durationOrDefault("REVIEW_BOT_POLL_INTERVAL", DefaultPollInterval, logger),
		MaxRuntime:   durationOrDefault("REVIEW_BOT_MAX_RUNTIME", DefaultMaxRuntime, logger),
		AllowedRepos: splitList(os.Getenv("REVIEW_BOT_ALLOWED_REPOS")),
	}
}

func RepoAllowed(repo string, allowlist []string) bool {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return false
	}
	for _, allowed := range allowlist {
		if strings.EqualFold(strings.TrimSpace(allowed), repo) {
			return true
		}
	}
	return false
}

func durationOrDefault(key string, fallback time.Duration, logger *zap.Logger) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		if logger != nil {
			logger.Warn("ignoring unparseable duration, using default",
				zap.String("key", key), zap.String("value", raw),
				zap.Duration("default", fallback))
		}
		return fallback
	}
	return parsed
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
