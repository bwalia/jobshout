package course

import (
	"encoding/json"
	"sort"
	"strings"
)

// Model JSON is loosely shaped. These types decode optional and hint fields
// tolerantly so one odd shape cannot fail a whole run. They are local to this
// package on purpose: the Article Writer has its own, and the two agents do
// not share internals.

// looseString accepts a string, a number, an array, or an object, collecting
// every string it finds (map keys sorted for determinism), joined by ", ".
type looseString string

func (s *looseString) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*s = looseString(strings.Join(collectStrings(v), ", "))
	return nil
}

// looseStrings accepts an array of strings or objects, or a single string
// (split on newlines), and yields a list of non-empty strings.
type looseStrings []string

func (s *looseStrings) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	var out []string
	switch t := v.(type) {
	case string:
		for _, line := range strings.Split(t, "\n") {
			if l := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*•")); l != "" {
				out = append(out, l)
			}
		}
	case []any:
		for _, item := range t {
			if joined := strings.Join(collectStrings(item), ", "); joined != "" {
				out = append(out, joined)
			}
		}
	default:
		out = collectStrings(v)
	}
	*s = out
	return nil
}

// looseInt accepts a number or a numeric string. Anything else is -1, which
// callers treat as invalid.
type looseInt int

func (n *looseInt) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*n = -1
	switch t := v.(type) {
	case float64:
		*n = looseInt(int(t))
	case string:
		var f float64
		if err := json.Unmarshal([]byte(strings.TrimSpace(t)), &f); err == nil {
			*n = looseInt(int(f))
		}
	}
	return nil
}

func collectStrings(v any) []string {
	var out []string
	switch t := v.(type) {
	case string:
		if s := strings.TrimSpace(t); s != "" {
			out = append(out, s)
		}
	case []any:
		for _, item := range t {
			out = append(out, collectStrings(item)...)
		}
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, collectStrings(t[k])...)
		}
	}
	return out
}
