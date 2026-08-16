package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// Several stages of the agent pipelines ask a model for JSON and are willing to
// fail the whole article if they do not get it. That made model choice hostage
// to one fragile parse: a model that writes well but occasionally emits a
// trailing comma, or runs into its token limit mid-object, could not be used at
// all. Benchmarking one locally produced exactly that — "invalid character ']'
// after object key:value pair" on a long prompt, from a model that returned
// clean JSON on short ones.
//
// The faults worth handling are mechanical and few: the reply is wrapped in
// prose or a fence, it has a trailing comma, or it was cut off mid-structure.
// None of them require asking a model to fix anything, and repairing them here
// costs nothing when the reply was valid to begin with.
//
// Nothing here invents data. Repair only ever removes a stray separator or
// closes what the model left open, so a truncated reply decodes to the part
// that did arrive rather than to nothing at all.

// trailingComma matches a comma that is followed by a closing bracket, which is
// valid in JavaScript and in most models' habits but not in JSON.
var trailingComma = regexp.MustCompile(`,(\s*[}\]])`)

// DecodeJSON unmarshals a model's reply into v.
//
// It tries the reply as extracted first and a repaired form second, so a
// well-behaved model takes exactly the same path it always did. The error
// returned on failure is the one from the extracted form, since that is the one
// that describes what the model actually wrote.
func DecodeJSON(reply string, v any) error {
	extracted := ExtractJSON(reply)

	err := json.Unmarshal([]byte(extracted), v)
	if err == nil {
		return nil
	}

	if repaired := repairJSON(extracted); repaired != extracted {
		if json.Unmarshal([]byte(repaired), v) == nil {
			return nil
		}
	}
	return err
}

// ExtractJSON returns the JSON value embedded in a model's reply, discarding a
// code fence and any prose around it.
//
// The closing bracket is found by scanning rather than by taking the last one
// in the string: a model that follows its JSON with a sentence containing a
// brace would otherwise produce a span that is not a value at all. When the
// brackets never balance — the usual sign of a truncated reply — everything
// from the opening bracket onwards is returned, so repairJSON can close it.
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	start := strings.IndexAny(s, "{[")
	if start == -1 {
		return s
	}
	if end := matchingBracket(s, start); end != -1 {
		return s[start : end+1]
	}
	return s[start:]
}

// matchingBracket returns the index of the bracket closing the one at start, or
// -1 if the string ends first. Brackets inside JSON strings are skipped.
func matchingBracket(s string, start int) int {
	var depth int
	var inString, escaped bool

	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
			// Brackets inside a string are data.
		case c == '{' || c == '[':
			depth++
		case c == '}' || c == ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// repairJSON applies the mechanical fixes that make a nearly-valid reply valid.
func repairJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = trailingComma.ReplaceAllString(s, "$1")
	return closeUnterminated(s)
}

// closeUnterminated finishes a value the model left open.
//
// A reply that runs into its token limit stops mid-structure, which decodes to
// nothing even though most of the content arrived. Closing the open string and
// the open brackets recovers what was written. A dangling key or separator is
// dropped first, because "…,"title":" cannot be closed into anything valid.
func closeUnterminated(s string) string {
	var stack []byte
	var inString, escaped bool

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
		case c == '{' || c == '[':
			stack = append(stack, c)
		case c == '}' || c == ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if !inString && len(stack) == 0 {
		return s
	}

	var b strings.Builder
	b.WriteString(s)
	if inString {
		b.WriteByte('"')
	}
	// Drop a separator or key left dangling at the end, which nothing can close.
	out := strings.TrimRight(b.String(), " \t\r\n")
	for strings.HasSuffix(out, ",") || strings.HasSuffix(out, ":") {
		out = strings.TrimRight(strings.TrimSuffix(out, ","), " \t\r\n")
		if strings.HasSuffix(out, ":") {
			out = trimDanglingKey(out)
		}
	}

	var tail strings.Builder
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == '{' {
			tail.WriteByte('}')
		} else {
			tail.WriteByte(']')
		}
	}
	return out + tail.String()
}

// trimDanglingKey removes a trailing `"key":` that has no value after it.
func trimDanglingKey(s string) string {
	s = strings.TrimRight(strings.TrimSuffix(s, ":"), " \t\r\n")
	if !strings.HasSuffix(s, `"`) {
		return s
	}
	// Walk back over the key to its opening quote.
	for i := len(s) - 2; i >= 0; i-- {
		if s[i] == '"' && (i == 0 || s[i-1] != '\\') {
			s = strings.TrimRight(s[:i], " \t\r\n")
			return strings.TrimRight(strings.TrimSuffix(s, ","), " \t\r\n")
		}
	}
	return s
}

// JSONRetryInstruction is appended to a prompt when a first reply could not be
// decoded, so the retry is told what went wrong rather than merely repeated.
const JSONRetryInstruction = "\n\nYour previous reply could not be parsed as JSON. " +
	"Reply with the JSON value only — no explanation, no code fence, no trailing text — " +
	"and make sure every bracket and quote is closed."

// GenerateJSON asks for JSON and decodes it, retrying once with a corrective
// instruction when the reply cannot be decoded even after repair.
//
// The caller supplies its own generate function so this does not need to know
// about token budgets, models or clients — each pipeline already has a wrapper
// that fixes those, and routing the retry through it keeps the second attempt
// identical to the first in every respect but the added instruction.
//
// One retry, not more. A model that cannot produce parseable JSON twice is not
// going to manage it on the third attempt, and each try costs a full
// generation on a pipeline that already makes several calls per article.
func GenerateJSON(
	ctx context.Context,
	stage, prompt string,
	v any,
	generate func(ctx context.Context, prompt string) (string, error),
	onRetry func(reply string, err error),
) error {
	reply, err := generate(ctx, prompt)
	if err != nil {
		return err
	}
	decodeErr := DecodeJSON(reply, v)
	if decodeErr == nil {
		return nil
	}
	if onRetry != nil {
		onRetry(reply, decodeErr)
	}

	// The failed attempt may have written some fields before giving up, and
	// json.Unmarshal does not clear what it does not overwrite. Start clean so
	// the retry cannot inherit half a value from the reply that failed.
	zero(v)

	retry, err := generate(ctx, prompt+JSONRetryInstruction)
	if err != nil {
		return err
	}
	if err := DecodeJSON(retry, v); err != nil {
		return DecodeError(stage, err, retry)
	}
	return nil
}

// zero resets the value behind a pointer to its zero value.
func zero(v any) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return
	}
	rv.Elem().Set(reflect.Zero(rv.Elem().Type()))
}

// DecodeError describes a reply that could not be decoded even after repair.
func DecodeError(stage string, err error, reply string) error {
	return fmt.Errorf("%s: parse response: %w (reply began: %s)", stage, err, snippet(reply, 120))
}

func snippet(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
