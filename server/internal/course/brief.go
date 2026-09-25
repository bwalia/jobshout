package course

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jobshout/server/internal/model"
)

// localeRe accepts BCP 47-style tags such as en, hi, pa, pt-BR, zh-Hant.
var localeRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8}){0,2}$`)

// Brief limits that keep one launch from asking for unbounded work.
const (
	maxTopicLen   = 300
	maxContextLen = 4000
	maxSeedURLs   = 8
	maxFocus      = 8
)

// BriefFromValues builds a brief from launch-form values.
func BriefFromValues(vals map[string]string) model.CourseBrief {
	n, _ := strconv.Atoi(strings.TrimSpace(vals["chapters"]))
	return model.CourseBrief{
		Topic:        strings.TrimSpace(vals["topic"]),
		Audience:     strings.TrimSpace(vals["audience"]),
		Level:        strings.TrimSpace(vals["level"]),
		ChapterCount: n,
		Locale:       strings.TrimSpace(vals["locale"]),
		Context:      strings.TrimSpace(vals["context"]),
		SeedURLs:     splitList(vals["seed_urls"]),
		Focus:        splitList(vals["focus"]),
		Model:        strings.TrimSpace(vals["model"]),
	}
}

// NormalizeBrief applies defaults and validates. maxChapters is the
// deployment's cap (Config.MaxChapters).
func NormalizeBrief(b model.CourseBrief, maxChapters int) (model.CourseBrief, error) {
	b.Topic = strings.TrimSpace(b.Topic)
	if b.Topic == "" {
		return b, fmt.Errorf("topic is required")
	}
	if len(b.Topic) > maxTopicLen {
		return b, fmt.Errorf("topic must be at most %d characters", maxTopicLen)
	}
	if len(b.Context) > maxContextLen {
		return b, fmt.Errorf("context must be at most %d characters", maxContextLen)
	}

	b.Level = strings.ToLower(strings.TrimSpace(b.Level))
	if b.Level == "" {
		b.Level = "beginner"
	}
	if !slices.Contains(model.CourseLevels, b.Level) {
		return b, fmt.Errorf("level must be one of %s", strings.Join(model.CourseLevels, ", "))
	}

	if maxChapters < model.CourseMinChapters || maxChapters > model.CourseMaxChapters {
		maxChapters = model.CourseMaxChapters
	}
	if b.ChapterCount == 0 {
		b.ChapterCount = min(model.CourseDefaultChapters, maxChapters)
	}
	if b.ChapterCount < model.CourseMinChapters || b.ChapterCount > maxChapters {
		return b, fmt.Errorf("chapters must be between %d and %d", model.CourseMinChapters, maxChapters)
	}

	b.Locale = strings.TrimSpace(b.Locale)
	if b.Locale == "" {
		b.Locale = model.CourseDefaultLocale
	}
	if !localeRe.MatchString(b.Locale) {
		return b, fmt.Errorf("locale must be a language tag such as en, hi or pt-BR")
	}

	if len(b.SeedURLs) > maxSeedURLs {
		return b, fmt.Errorf("at most %d seed URLs", maxSeedURLs)
	}
	for _, raw := range b.SeedURLs {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return b, fmt.Errorf("seed URL %q must be an http(s) URL", raw)
		}
	}
	if len(b.Focus) > maxFocus {
		b.Focus = b.Focus[:maxFocus]
	}
	return b, nil
}

// splitList splits comma- or newline-separated input, dropping blanks.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
