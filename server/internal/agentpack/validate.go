package agentpack

import (
	"fmt"
	"strings"
)

// DecodeErrors are user-facing problems with an uploaded file.
func ValidateKind(pkg *Package) error {
	if pkg == nil {
		return fmt.Errorf("package is required")
	}
	if pkg.Kind != Kind {
		return fmt.Errorf("not a JobShout agent package (kind %q)", pkg.Kind)
	}
	if pkg.SchemaVersion < MinSchemaVersion {
		return fmt.Errorf("schema_version %d is too old", pkg.SchemaVersion)
	}
	if pkg.SchemaVersion > SchemaVersion {
		return fmt.Errorf("schema_version %d is newer than this JobShout supports (max %d)", pkg.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(pkg.Agent.Name) == "" {
		return fmt.Errorf("agent name is required")
	}
	if strings.TrimSpace(pkg.Agent.Role) == "" {
		pkg.Agent.Role = "Agent"
	}
	return nil
}

// DefaultName returns the create-mode name, with a clash suffix when needed.
func DefaultName(pkg *Package, taken bool) string {
	return UniqueName(pkg, taken, 1)
}

// UniqueName is "{name} (imported)" then "{name} (imported 2)" …
func UniqueName(pkg *Package, taken bool, attempt int) string {
	name := strings.TrimSpace(pkg.Agent.Name)
	if !taken {
		return name
	}
	if attempt <= 1 {
		return name + " (imported)"
	}
	return fmt.Sprintf("%s (imported %d)", name, attempt)
}

func skipSet(skip []string, includeGated bool, tools []string) map[string]bool {
	out := map[string]bool{}
	for _, s := range skip {
		if includeGated && isGated(s) {
			continue
		}
		out[s] = true
	}
	if !includeGated {
		for _, t := range tools {
			if isGated(t) {
				out[t] = true
			}
		}
	}
	return out
}

func filterTools(tools []string, skip map[string]bool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		if skip[t] {
			continue
		}
		out = append(out, t)
	}
	return out
}
