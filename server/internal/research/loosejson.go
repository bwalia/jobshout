package research

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Model JSON is loosely shaped. A small model asked for [0, 4] and true will
// sometimes send ["0","4"], "4", or "true" — and one strict field failing to
// decode fails the whole reply, which fails a scheduled run over a hint. These
// types take what they can read and drop the rest.

// looseInts decodes a number, a numeric string, or an array of either.
type looseInts []int

func (l *looseInts) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	var out []int
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case float64:
			out = append(out, int(t))
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
				out = append(out, n)
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	*l = out
	return nil
}

// looseBool decodes a bool or a "true"/"yes" string. Anything else is false.
type looseBool bool

func (l *looseBool) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch t := v.(type) {
	case bool:
		*l = looseBool(t)
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		*l = looseBool(s == "true" || s == "yes")
	default:
		*l = false
	}
	return nil
}
