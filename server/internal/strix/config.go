package strix

import (
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Defaults for the workstation pentest service.
const (
	// DefaultTimeout bounds ONE HTTP call, not one scan. Conflating the two is
	// how a two-hour client timeout hides a dead service for two hours.
	DefaultTimeout = 60 * time.Second
	// DefaultPollInterval is how often a non-terminal run is re-checked.
	DefaultPollInterval = 15 * time.Second
	// DefaultMaxRuntime is when the reconciler gives up on a run the service
	// never finished. The service enforces its own ceiling; this is the backstop
	// for the case where it stops answering altogether.
	DefaultMaxRuntime = 2 * time.Hour
)

type Config struct {
	Enabled bool
	// BaseURL is the workstation pentest service, e.g.
	// https://strix.workstation.co.uk. Empty means the feature is off: a ring
	// that has not stood the service up should not spend a timeout discovering
	// that on every scan request.
	BaseURL string
	// JWTSecret signs requests to the service. Deliberately distinct from the
	// Ollama and image-service secrets — those front a language model and a GPU,
	// this fronts a vulnerability scanner, and a leak of one credential should
	// not hand over all three.
	JWTSecret    string
	Timeout      time.Duration
	PollInterval time.Duration
	MaxRuntime   time.Duration
	// TargetAllowlist is defence in depth. scope.py on the workstation is the
	// real boundary — it cannot be bypassed by anyone holding a token, because
	// it is the code path that decides whether a subprocess runs. This copy
	// refuses obviously out-of-scope work before it crosses the network.
	TargetAllowlist []string
	// EngagementRetries is how many times the reconciler will automatically
	// requeue a hollow run (finished without touching the target) before letting
	// it fail. One by default: a flaky local model produces the occasional false
	// hollow run, but a target that is genuinely unreachable must still fail
	// rather than loop.
	EngagementRetries int
}

// Configured reports whether scans can actually be dispatched.
func (c Config) Configured() bool { return c.Enabled && c.BaseURL != "" }

func LoadConfig(logger *zap.Logger) Config {
	cfg := Config{
		Enabled:           os.Getenv("STRIX_ENABLED") != "false",
		BaseURL:           strings.TrimRight(os.Getenv("STRIX_BASE_URL"), "/"),
		JWTSecret:         os.Getenv("STRIX_JWT_SECRET"),
		Timeout:           durationOrDefault("STRIX_TIMEOUT", DefaultTimeout, logger),
		PollInterval:      durationOrDefault("STRIX_POLL_INTERVAL", DefaultPollInterval, logger),
		MaxRuntime:        durationOrDefault("STRIX_MAX_RUNTIME", DefaultMaxRuntime, logger),
		TargetAllowlist:   splitList(os.Getenv("STRIX_TARGET_ALLOWLIST")),
		EngagementRetries: intOrDefault("STRIX_ENGAGEMENT_RETRIES", 1, logger),
	}
	return cfg
}

// intOrDefault reads a non-negative integer from the environment, falling back
// rather than failing to boot on a typo — the same forgiving posture as
// durationOrDefault.
func intOrDefault(key string, fallback int, logger *zap.Logger) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		if logger != nil {
			logger.Warn("ignoring unparseable integer, using default",
				zap.String("key", key), zap.String("value", raw),
				zap.Int("default", fallback))
		}
		return fallback
	}
	return parsed
}

func durationOrDefault(key string, fallback time.Duration, logger *zap.Logger) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		// Falls back rather than failing to boot: a malformed duration is a
		// typo in one setting, not a reason for the whole server to refuse to
		// start with every other feature working.
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
