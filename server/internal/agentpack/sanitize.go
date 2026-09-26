package agentpack

import (
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

var secretKey = regexp.MustCompile(`(?i)(secret|token|password|api[_-]?key|apikey|credential|refresh|private[_-]?key|access[_-]?key|authorization)`)

var engineAllow = map[string]bool{
	"structured_model": true,
	"graph_definition": true,
}

func isSecretKey(k string) bool {
	return secretKey.MatchString(strings.TrimSpace(k))
}

// SanitizeEngineConfig keep-lists structured_model and graph_definition, then
// strips secret-shaped keys from whatever remains (including arrays).
func SanitizeEngineConfig(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(engineAllow))
	for k, v := range in {
		if !engineAllow[k] {
			continue
		}
		out[k] = sanitizeValue(v)
	}
	return out
}

// SanitizeMap strips secret-shaped keys from an arbitrary JSON object
// (skill config, overrides).
func SanitizeMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if isSecretKey(k) {
			continue
		}
		out[k] = sanitizeValue(v)
	}
	if len(out) == 0 {
		return map[string]any{}
	}
	return out
}

func sanitizeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return SanitizeMap(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = sanitizeValue(item)
		}
		return out
	default:
		return v
	}
}

// SanitizePackage mutates pkg in place: engine_config, skill configs, no ids.
func SanitizePackage(pkg *Package) {
	if pkg == nil {
		return
	}
	pkg.Agent.EngineConfig = SanitizeEngineConfig(pkg.Agent.EngineConfig)
	if len(pkg.Agent.SystemPrompt) > MaxSystemPrompt {
		pkg.Agent.SystemPrompt = truncateBytes(pkg.Agent.SystemPrompt, MaxSystemPrompt)
	}
	for i := range pkg.Skills {
		pkg.Skills[i].ConfigJSON = SanitizeMap(pkg.Skills[i].ConfigJSON)
		pkg.Skills[i].ConfigOverride = SanitizeMap(pkg.Skills[i].ConfigOverride)
	}
	if pkg.Tools == nil {
		pkg.Tools = []string{}
	}
	for i := range pkg.Knowledge {
		pkg.Knowledge[i].Filename = SafeFilename(pkg.Knowledge[i].Filename)
	}
}

// SafeFilename is a single path segment, so knowledge names cannot traverse.
func SafeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" || name == ".." {
		return "file"
	}
	if len(name) > 255 {
		name = truncateBytes(name, 255)
	}
	return name
}

func truncateBytes(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func remainingSecretKeys(m map[string]any) []string {
	var found []string
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, child := range t {
				if isSecretKey(k) && !engineAllow[k] {
					found = append(found, k)
				}
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(m)
	return found
}

// HeaderSafe strips CR/LF so values can go in HTTP headers.
func HeaderSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, s)
}
