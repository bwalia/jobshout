package video

import (
	"os"
	"strings"
	"time"
)

// Config is the Video Producer's own configuration. It reads VIDEO_* variables
// so the platform config does not grow a field per agent. The renderer is the
// workstation image service, so its address and gateway secret are the image
// service's, set by main.
type Config struct {
	// Model overrides the LLM model that writes the script. Empty uses the
	// provider's default.
	Model    string
	Provider string
	// BaseURL and JWTSecret reach the workstation image service, which renders
	// video beside images: one GPU, one lock, one gateway host.
	BaseURL   string
	JWTSecret string
	// PublicBaseURL makes the finished video's link absolute on the board,
	// where a relative path is not clickable.
	PublicBaseURL string
	// MaxRuntime bounds a whole run, script to stored file. An animated
	// two-minute video is about an hour of GPU; a queue ahead of it adds more.
	MaxRuntime time.Duration
	// PollInterval is how often a render's progress is checked.
	PollInterval time.Duration
}

// LoadConfig reads VIDEO_* variables with safe defaults.
func LoadConfig() Config {
	return Config{
		Model:        strings.TrimSpace(os.Getenv("VIDEO_MODEL")),
		MaxRuntime:   durationEnv("VIDEO_MAX_RUNTIME", 4*time.Hour),
		PollInterval: durationEnv("VIDEO_POLL_INTERVAL", 20*time.Second),
	}
}

func durationEnv(key string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key)))
	if err != nil || d <= 0 {
		return def
	}
	return d
}
