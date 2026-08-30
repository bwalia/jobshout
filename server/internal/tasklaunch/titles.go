package tasklaunch

import (
	"strings"

	"github.com/jobshout/server/internal/model"
)

// TitleFrom mirrors the Task Manager titleFrom / descriptionFrom rules.
func TitleFrom(kind string, v map[string]string) (title, description string) {
	switch kind {
	case model.BuiltinArticleWriter:
		t := strings.TrimSpace(v["topic"])
		if t == "" {
			t = "article"
		}
		title = "Write: " + t
		parts := []string{"Topic: " + t}
		if c := strings.TrimSpace(v["context"]); c != "" {
			parts = append(parts, c)
		}
		return title, strings.Join(parts, "\n\n")
	case model.BuiltinResearcher:
		t := strings.TrimSpace(v["topic"])
		if t == "" {
			t = "topic"
		}
		title = "Research: " + t
		parts := []string{"Topic: " + t}
		if c := strings.TrimSpace(v["context"]); c != "" {
			parts = append(parts, c)
		}
		return title, strings.Join(parts, "\n\n")
	case model.BuiltinPentester:
		target := strings.TrimSpace(v["target"])
		if target == "" {
			target = "target"
		}
		title = "Pentest: " + target
		parts := []string{"Target: " + target, "Mode: " + orDefault(v["scan_mode"], "quick")}
		if i := strings.TrimSpace(v["instruction"]); i != "" {
			parts = append(parts, i)
		}
		return title, strings.Join(parts, "\n")
	case model.BuiltinPRReviewer:
		repo := strings.TrimSpace(v["repo"])
		if repo == "" {
			repo = "repo"
		}
		pr := strings.TrimSpace(v["pr_number"])
		if pr == "" {
			pr = "?"
		}
		title = "Review: " + repo + "#" + pr
		desc := "Review " + repo + "#" + pr
		if v["dry_run"] == "true" {
			desc += " (preview only)"
		}
		return title, desc
	case model.BuiltinMail:
		if f := strings.TrimSpace(v["research_focus"]); f != "" {
			if len(f) > 80 {
				f = f[:80]
			}
			title = "Mail: " + f
		} else if strings.TrimSpace(v["knowledge_urls"]) != "" {
			title = "Mail: research pinned pages and draft"
		} else {
			title = "Mail: sync inbox and draft"
		}
		var parts []string
		if s := strings.TrimSpace(v["senders"]); s != "" {
			parts = append(parts, "Senders: "+s)
		}
		if u := strings.TrimSpace(v["knowledge_urls"]); u != "" {
			parts = append(parts, u)
		}
		if f := strings.TrimSpace(v["research_focus"]); f != "" {
			parts = append(parts, "Look for: "+f)
		}
		if r := strings.TrimSpace(v["reply_instructions"]); r != "" {
			parts = append(parts, "Reply style: "+r)
		}
		return title, strings.Join(parts, "\n\n")
	case model.BuiltinImages:
		p := strings.TrimSpace(v["prompt"])
		if p == "" {
			p = "image"
		}
		if len(p) > 80 {
			p = p[:80]
		}
		return "Image: " + p, p
	default:
		title = strings.TrimSpace(v["title"])
		if title == "" {
			title = strings.TrimSpace(v["prompt"])
		}
		if title == "" {
			title = "Untitled task"
		}
		return title, strings.TrimSpace(v["description"])
	}
}

func orDefault(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	return s
}
