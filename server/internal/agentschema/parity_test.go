package agentschema

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The agent input contract exists twice: here, and in TypeScript at
// web/nextjs/lib/agents/input-schemas.ts, because the Task Manager renders the
// form before any request is made. A comment asking people to keep the two in
// step is a defect with a deadline, and the deadline had passed — the Mail
// Agent had six fields on the TypeScript side and none here, which is why it
// worked well from the Task Manager and barely worked from chat.
//
// This test is the regression net. It parses the TypeScript rather than
// requiring a running server, so it costs nothing in CI and fails on the commit
// that introduces the drift.

const tsSchemaPath = "../../../web/nextjs/lib/agents/input-schemas.ts"

// tsField is one field as declared in the TypeScript contract.
type tsField struct {
	key        string
	required   bool
	defaultVal string
}

func TestGoAndTypeScriptSchemasAgree(t *testing.T) {
	src, err := os.ReadFile(filepath.Clean(tsSchemaPath))
	if err != nil {
		// The server is testable on its own; a missing web tree is not a
		// server defect.
		t.Skipf("TypeScript contract not present (%v); skipping parity check", err)
	}
	ts := parseTSSchemas(t, string(src))

	for _, builtin := range Builtins {
		builtin := builtin
		t.Run(builtin, func(t *testing.T) {
			tsFields, ok := ts[builtin]
			if !ok {
				t.Fatalf("%s is missing from %s — the Task Manager cannot render a form for it",
					builtin, tsSchemaPath)
			}
			goFields := ForBuiltin(builtin).Fields

			if len(goFields) != len(tsFields) {
				t.Fatalf("field count differs: Go has %d %v, TypeScript has %d %v",
					len(goFields), keysOf(goFields), len(tsFields), tsKeysOf(tsFields))
			}
			// Order matters: it is the order the interview asks in, and chat
			// walks it one slot at a time.
			for i := range goFields {
				if goFields[i].Key != tsFields[i].key {
					t.Fatalf("field %d differs: Go %q, TypeScript %q (order is the interview order)",
						i, goFields[i].Key, tsFields[i].key)
				}
				if goFields[i].Required != tsFields[i].required {
					t.Errorf("%s.required differs: Go %v, TypeScript %v",
						goFields[i].Key, goFields[i].Required, tsFields[i].required)
				}
			}
		})
	}
}

// TestKnownDefaultDivergence pins the one field the two copies disagree on, so
// the disagreement is recorded rather than rediscovered.
//
// pr_reviewer.dry_run defaults to preview-only here and to posting in
// TypeScript, so the same agent writes on a public pull request from the Task
// Manager and stays silent from chat. Reconciling it changes whether an agent
// posts in public, which is a product decision: migration 031
// (pr_reviewer_post_by_default) reseeded the system prompt to say it posts by
// default, while platformtools/review.go defaults dry := true.
//
// When that decision is made, make the defaults match and delete this test.
func TestKnownDefaultDivergence_PRReviewerDryRun(t *testing.T) {
	src, err := os.ReadFile(filepath.Clean(tsSchemaPath))
	if err != nil {
		t.Skipf("TypeScript contract not present (%v)", err)
	}
	ts := parseTSSchemas(t, string(src))

	var goDefault string
	for _, f := range ForBuiltin("pr_reviewer").Fields {
		if f.Key == "dry_run" {
			goDefault = f.Default
		}
	}
	var tsDefault string
	for _, f := range ts["pr_reviewer"] {
		if f.key == "dry_run" {
			tsDefault = f.defaultVal
		}
	}

	if goDefault == tsDefault {
		t.Fatalf("dry_run defaults now agree (%q) — the divergence is resolved, so delete this test "+
			"and let TestGoAndTypeScriptSchemasAgree assert defaults too", goDefault)
	}
	t.Logf("known divergence: dry_run default is %q in Go and %q in TypeScript", goDefault, tsDefault)
}

var (
	tsBuiltinStart = regexp.MustCompile(`(?m)^  ([a-z_]+): \{$`)
	tsKeyLine      = regexp.MustCompile(`key: "([a-z_]+)"`)
)

// parseTSSchemas extracts the ordered fields of each builtin from the
// TypeScript source.
//
// It reads the SCHEMAS object only, so the GENERIC fallback above it is
// ignored: the two generics deliberately differ — TypeScript asks for a board
// task's title and description, Go asks for a chat prompt — and they are not
// copies of one another.
func parseTSSchemas(t *testing.T, src string) map[string][]tsField {
	t.Helper()
	start := strings.Index(src, "const SCHEMAS")
	if start < 0 {
		t.Fatalf("could not find `const SCHEMAS` in %s", tsSchemaPath)
	}
	body := src[start:]

	out := map[string][]tsField{}
	locs := tsBuiltinStart.FindAllStringSubmatchIndex(body, -1)
	for i, loc := range locs {
		name := body[loc[2]:loc[3]]
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		block := body[loc[1]:end]

		fieldsAt := strings.Index(block, "fields: [")
		if fieldsAt < 0 {
			out[name] = nil
			continue
		}
		fields := block[fieldsAt:]
		if close := strings.Index(fields, "\n    ],"); close > 0 {
			fields = fields[:close]
		}
		out[name] = parseTSFields(fields)
	}
	return out
}

// parseTSFields splits a fields array on its key declarations, so each field's
// properties are read from the span between its key and the next one.
func parseTSFields(fields string) []tsField {
	keys := tsKeyLine.FindAllStringSubmatchIndex(fields, -1)
	out := make([]tsField, 0, len(keys))
	for i, k := range keys {
		end := len(fields)
		if i+1 < len(keys) {
			end = keys[i+1][0]
		}
		span := fields[k[1]:end]
		f := tsField{key: fields[k[2]:k[3]]}
		f.required = strings.Contains(span, "required: true")
		if m := regexp.MustCompile(`defaultValue: ([^,\n]+)`).FindStringSubmatch(span); m != nil {
			f.defaultVal = strings.TrimSpace(m[1])
		}
		out = append(out, f)
	}
	return out
}

func keysOf(fs []Field) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Key
	}
	return out
}

func tsKeysOf(fs []tsField) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.key
	}
	return out
}
