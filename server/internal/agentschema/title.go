package agentschema

import "strings"

// TitleFrom builds the board-task title and description from schema rules.
func TitleFrom(s Schema, v map[string]string) (title, description string) {
	if len(s.TitleRules) == 0 {
		title = strings.TrimSpace(v["title"])
		if title == "" {
			title = strings.TrimSpace(v["prompt"])
		}
		if title == "" {
			title = "Untitled task"
		}
		return title, strings.TrimSpace(v["description"])
	}
	title = applyTitle(s.TitleRules, v)
	if title == "" {
		title = "Untitled task"
	}
	description = applyDesc(s.DescRules, v)
	return title, description
}

func applyTitle(rules []TitleRule, v map[string]string) string {
	for _, r := range rules {
		if r.IfKey != "" && strings.TrimSpace(v[r.IfKey]) == "" {
			continue
		}
		if r.Literal != "" {
			return r.Literal
		}
		var b strings.Builder
		b.WriteString(r.Prefix)
		part := firstValue(v, r.FromKey, r.FromKeys, r.Fallback)
		if r.Truncate > 0 && len(part) > r.Truncate {
			part = part[:r.Truncate]
		}
		if r.Format != "" {
			b.WriteString(expand(r.Format, v))
		} else {
			b.WriteString(part)
		}
		if r.SuffixIf != "" && (v[r.SuffixIf] == "true" || (v[r.SuffixIf] == "" && r.SuffixIf == "dry_run")) {
			b.WriteString(r.Suffix)
		}
		out := b.String()
		if strings.TrimSpace(out) != r.Prefix || part != "" || r.Format != "" {
			return out
		}
	}
	return ""
}

func applyDesc(rules []DescRule, v map[string]string) string {
	var parts []string
	for _, r := range rules {
		if r.Literal != "" {
			parts = append(parts, r.Literal)
			continue
		}
		if r.Format != "" {
			line := expand(r.Format, v)
			if r.SuffixIf != "" && (v[r.SuffixIf] == "true" || (v[r.SuffixIf] == "" && r.SuffixIf == "dry_run")) {
				line += r.Suffix
			}
			parts = append(parts, line)
			continue
		}
		raw := strings.TrimSpace(v[r.Key])
		if raw == "" {
			continue
		}
		if r.Truncate > 0 && len(raw) > r.Truncate {
			raw = raw[:r.Truncate]
		}
		if r.Prefix != "" {
			parts = append(parts, r.Prefix+raw)
		} else {
			parts = append(parts, raw)
		}
	}
	return strings.Join(parts, "\n\n")
}

func firstValue(v map[string]string, key string, keys []string, fallback string) string {
	if key != "" {
		if s := strings.TrimSpace(v[key]); s != "" {
			return s
		}
	}
	for _, k := range keys {
		if s := strings.TrimSpace(v[k]); s != "" {
			return s
		}
	}
	return fallback
}

func expand(format string, v map[string]string) string {
	out := format
	for k, val := range v {
		out = strings.ReplaceAll(out, "{"+k+"}", strings.TrimSpace(val))
	}
	return out
}
