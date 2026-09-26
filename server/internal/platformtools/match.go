package platformtools

import (
	"strings"
)

// Match is the result of resolving a name against a list of labelled items.
type Match[T any] struct {
	Exact      T
	Found      bool
	Candidates []T
}

// ByName picks an item by exact case-insensitive name. Partial matches are
// never selected silently — they are returned as Candidates so the caller can
// ask a disambiguation question. Unique partials are still candidates: "Data"
// must not silently become "Database".
func ByName[T any](items []T, query string, nameOf func(T) string) Match[T] {
	q := strings.TrimSpace(query)
	var zero T
	if q == "" {
		return Match[T]{Exact: zero}
	}
	lower := strings.ToLower(q)

	var exact []T
	var partial []T
	for _, item := range items {
		n := nameOf(item)
		if strings.EqualFold(n, q) {
			exact = append(exact, item)
			continue
		}
		if strings.Contains(strings.ToLower(n), lower) {
			partial = append(partial, item)
		}
	}
	if len(exact) == 1 {
		return Match[T]{Exact: exact[0], Found: true}
	}
	if len(exact) > 1 {
		return Match[T]{Candidates: exact}
	}
	return Match[T]{Candidates: partial}
}

func labelsOf[T any](items []T, nameOf func(T) string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, nameOf(item))
	}
	return out
}
