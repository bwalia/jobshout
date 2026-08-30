package agentschema

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const tsSchemaPath = "../../../web/nextjs/lib/agents/input-schemas.ts"

type tsField struct {
	key        string
	required   bool
	defaultVal string
}

func TestGoAndTypeScriptSchemasAgree(t *testing.T) {
	src, err := os.ReadFile(filepath.Clean(tsSchemaPath))
	if err != nil {
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
			for i := range goFields {
				if goFields[i].Key != tsFields[i].key {
					t.Fatalf("field %d differs: Go %q, TypeScript %q (order is the interview order)",
						i, goFields[i].Key, tsFields[i].key)
				}
				if goFields[i].Required != tsFields[i].required {
					t.Errorf("%s.required differs: Go %v, TypeScript %v",
						goFields[i].Key, goFields[i].Required, tsFields[i].required)
				}
				if goFields[i].Default != "" && tsFields[i].defaultVal != "" {
					tsNorm := strings.Trim(tsFields[i].defaultVal, `"'`)
					if tsNorm == "true" || tsNorm == "false" {
						// checkbox defaults are boolean in TS, string in Go
						if goFields[i].Default != tsNorm {
							t.Errorf("%s.default differs: Go %q, TypeScript %q",
								goFields[i].Key, goFields[i].Default, tsNorm)
						}
					} else if goFields[i].Default != tsNorm {
						t.Errorf("%s.default differs: Go %q, TypeScript %q",
							goFields[i].Key, goFields[i].Default, tsNorm)
					}
				}
			}
		})
	}
}

var (
	tsBuiltinStart = regexp.MustCompile(`(?m)^  ([a-z_]+): \{$`)
	tsKeyLine      = regexp.MustCompile(`key: "([a-z_]+)"`)
)

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
