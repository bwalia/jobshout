package agentpack

import (
	"regexp"
	"strings"
)

var secretKey = regexp.MustCompile(`(?i)(secret|token|password|api_key|apikey|credential|refresh)`)

var engineAllow = map[string]bool{
	"structured_model": true,
	"graph_definition": true,
}

func isSecretKey(k string) bool {
	return secretKey.MatchString(strings.TrimSpace(k))
}

// SanitizeEngineConfig drops secret-shaped keys. Allowlisted keys are kept
// even if a future allowlist name happened to match the secret regex.
func SanitizeEngineConfig(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if isSecretKey(k) && !engineAllow[k] {
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			out[k] = SanitizeEngineConfig(nested)
			continue
		}
		out[k] = v
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
		if nested, ok := v.(map[string]any); ok {
			out[k] = SanitizeMap(nested)
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return map[string]any{}
	}
	return out
}

// SanitizePackage mutates pkg in place: engine_config, skill configs, no ids.
func SanitizePackage(pkg *Package) {
	if pkg == nil {
		return
	}
	pkg.Agent.EngineConfig = SanitizeEngineConfig(pkg.Agent.EngineConfig)
	if len(pkg.Agent.SystemPrompt) > MaxSystemPrompt {
		pkg.Agent.SystemPrompt = pkg.Agent.SystemPrompt[:MaxSystemPrompt]
	}
	for i := range pkg.Skills {
		pkg.Skills[i].ConfigJSON = SanitizeMap(pkg.Skills[i].ConfigJSON)
		pkg.Skills[i].ConfigOverride = SanitizeMap(pkg.Skills[i].ConfigOverride)
	}
	if pkg.Tools == nil {
		pkg.Tools = []string{}
	}
}

func remainingSecretKeys(m map[string]any) []string {
	var found []string
	var walk func(map[string]any)
	walk = func(cur map[string]any) {
		for k, v := range cur {
			if isSecretKey(k) && !engineAllow[k] {
				found = append(found, k)
			}
			if nested, ok := v.(map[string]any); ok {
				walk(nested)
			}
		}
	}
	walk(m)
	return found
}
