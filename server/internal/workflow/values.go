package workflow

import (
	"fmt"
	"strings"
)

// ParseLaunchValues merges global workflow input with key=value / key: value
// lines from the rendered step prompt so specialist Launch schemas get fields.
func ParseLaunchValues(prompt string, globalInput map[string]any) map[string]string {
	vals := map[string]string{}
	for k, v := range globalInput {
		if k == "" || v == nil {
			continue
		}
		vals[k] = strings.TrimSpace(fmt.Sprint(v))
	}
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sep := ""
		if i := strings.Index(line, ":"); i > 0 && (strings.Index(line, "=") < 0 || i < strings.Index(line, "=")) {
			sep = ":"
		} else if strings.Contains(line, "=") {
			sep = "="
		}
		if sep == "" {
			continue
		}
		var key, val string
		if sep == ":" {
			parts := strings.SplitN(line, ":", 2)
			key, val = parts[0], parts[1]
		} else {
			parts := strings.SplitN(line, "=", 2)
			key, val = parts[0], parts[1]
		}
		key = strings.TrimSpace(strings.ToLower(key))
		key = strings.ReplaceAll(key, " ", "_")
		val = strings.TrimSpace(val)
		if key != "" && val != "" {
			vals[key] = val
		}
	}
	if strings.TrimSpace(vals["instruction"]) == "" && strings.TrimSpace(prompt) != "" {
		// Keep free-text prompt as instruction for AbsorbPrompt helpers.
		if _, hasPath := vals["path"]; !hasPath {
			if _, hasHosts := vals["hosts"]; !hasHosts {
				vals["instruction"] = strings.TrimSpace(prompt)
			}
		}
	}
	return vals
}
