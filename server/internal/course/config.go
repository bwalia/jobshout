package course

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the Course Generator's own configuration. It reads COURSE_*
// environment variables so the platform config does not grow a field per
// agent. Nothing here is a secret.
type Config struct {
	// Model overrides the LLM model for every stage. Empty uses the
	// provider's default.
	Model string
	// ChapterBudget is the runtime allowed per chapter. A run's deadline is
	// ChapterBudget × chapters + PlanBudget, so a long course is not killed by
	// a flat cap sized for a short one.
	ChapterBudget time.Duration
	// PlanBudget covers research and outlining, which happen once per run.
	PlanBudget time.Duration
	// MaxChapters caps what a brief may ask for.
	MaxChapters int
	// Images turns chapter illustrations and the course cover on or off.
	Images bool
	// OrphanTimeout is how stale a running row's heartbeat must be before
	// the reaper fails it.
	OrphanTimeout time.Duration
}

// LoadConfig reads COURSE_* variables with safe defaults.
func LoadConfig() Config {
	return Config{
		Model:         strings.TrimSpace(os.Getenv("COURSE_MODEL")),
		ChapterBudget: durationEnv("COURSE_MAX_RUNTIME", 20*time.Minute),
		PlanBudget:    durationEnv("COURSE_PLAN_RUNTIME", 20*time.Minute),
		MaxChapters:   intEnv("COURSE_MAX_CHAPTERS", 8, 1, 12),
		Images:        boolEnv("COURSE_IMAGES", true),
		OrphanTimeout: durationEnv("COURSE_ORPHAN_TIMEOUT", 10*time.Minute),
	}
}

// RunBudget is the deadline for a run of n chapters.
func (c Config) RunBudget(chapters int) time.Duration {
	if chapters < 1 {
		chapters = 1
	}
	return c.PlanBudget + time.Duration(chapters)*c.ChapterBudget
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

func intEnv(key string, def, lo, hi int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
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
