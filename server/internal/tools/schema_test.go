package tools

import (
	"reflect"
	"testing"
)

func TestObjectSchema(t *testing.T) {
	got := ObjectSchema(map[string]any{
		"command": map[string]any{"type": "string"},
	}, "command")

	if got["type"] != "object" {
		t.Fatalf("type: got %v, want object", got["type"])
	}
	props, ok := got["properties"].(map[string]any)
	if !ok || props["command"] == nil {
		t.Fatalf("properties not carried through: %#v", got["properties"])
	}
	req, ok := got["required"].([]string)
	if !ok || !reflect.DeepEqual(req, []string{"command"}) {
		t.Fatalf("required: got %#v, want [command]", got["required"])
	}
}

func TestObjectSchemaNoRequiredIsEmptyArray(t *testing.T) {
	got := ObjectSchema(nil)
	if req, ok := got["required"].([]string); !ok || len(req) != 0 {
		t.Fatalf("required should be an empty (non-nil) slice, got %#v", got["required"])
	}
	if _, ok := got["properties"].(map[string]any); !ok {
		t.Fatalf("properties should default to an empty map, got %#v", got["properties"])
	}
}

// TestToolsAdvertiseSchemas asserts every native tool implements SchemaProvider
// and returns a well-formed object schema that includes its required fields.
func TestToolsAdvertiseSchemas(t *testing.T) {
	cases := []struct {
		name     string
		tool     Tool
		required []string
	}{
		{"http_request", NewHTTPTool(), []string{"method", "url"}},
		{"shell_command", NewShellTool(nil), []string{"command"}},
		{"jira_create_issue", &createIssueTool{provider: "jira"}, []string{"title"}},
		{"jira_get_issue", &getIssueTool{provider: "jira"}, []string{"external_id"}},
		{"slack_send_message", &sendMessageTool{name: "slack_send_message", channel: "slack"}, []string{"message"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sp, ok := c.tool.(SchemaProvider)
			if !ok {
				t.Fatalf("%s does not implement SchemaProvider", c.tool.Name())
			}
			schema := sp.Parameters()
			if schema["type"] != "object" {
				t.Fatalf("schema type: got %v", schema["type"])
			}
			props, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("properties missing: %#v", schema)
			}
			req, _ := schema["required"].([]string)
			for _, field := range c.required {
				if props[field] == nil {
					t.Fatalf("%s: missing property %q", c.name, field)
				}
				if !contains(req, field) {
					t.Fatalf("%s: %q not in required %v", c.name, field, req)
				}
			}
		})
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
