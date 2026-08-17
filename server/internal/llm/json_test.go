package llm

import (
	"reflect"
	"testing"
)

type planShape struct {
	Title    string   `json:"title"`
	Angle    string   `json:"angle"`
	Sections []string `json:"sections"`
}

func TestDecodeJSONAcceptsACleanReply(t *testing.T) {
	var got planShape
	if err := DecodeJSON(`{"title":"T","angle":"A","sections":["one","two"]}`, &got); err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	want := planShape{Title: "T", Angle: "A", Sections: []string{"one", "two"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDecodeJSONStripsFenceAndProse(t *testing.T) {
	reply := "Sure, here is the plan:\n\n```json\n{\"title\":\"T\",\"sections\":[\"a\"]}\n```\n\nLet me know if you want changes."
	var got planShape
	if err := DecodeJSON(reply, &got); err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if got.Title != "T" || len(got.Sections) != 1 {
		t.Errorf("got %+v", got)
	}
}

// Prose after the JSON containing a brace must not extend the captured span.
func TestDecodeJSONIgnoresBracesInTrailingProse(t *testing.T) {
	reply := `{"title":"T","sections":["a"]} Note: use {braces} carefully.`
	var got planShape
	if err := DecodeJSON(reply, &got); err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if got.Title != "T" {
		t.Errorf("got %+v", got)
	}
}

// Braces inside string values must not confuse the scanner.
func TestDecodeJSONHandlesBracesInsideStrings(t *testing.T) {
	var got planShape
	if err := DecodeJSON(`{"title":"use {this} and [that]","sections":["a"]}`, &got); err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if got.Title != "use {this} and [that]" {
		t.Errorf("got %q", got.Title)
	}
}

func TestDecodeJSONRepairsTrailingCommas(t *testing.T) {
	var got planShape
	if err := DecodeJSON(`{"title":"T","sections":["a","b",],}`, &got); err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if len(got.Sections) != 2 {
		t.Errorf("got %+v", got)
	}
}

// The observed failure: a reply cut off by the token limit. What arrived should
// still decode.
func TestDecodeJSONRecoversTruncatedReply(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  planShape
	}{
		{"cut mid-array",
			`{"title":"T","angle":"A","sections":["one","two"`,
			planShape{Title: "T", Angle: "A", Sections: []string{"one", "two"}}},
		{"cut mid-string",
			`{"title":"T","angle":"A","sections":["one","tw`,
			planShape{Title: "T", Angle: "A", Sections: []string{"one", "tw"}}},
		{"cut after comma",
			`{"title":"T","angle":"A","sections":["one",`,
			planShape{Title: "T", Angle: "A", Sections: []string{"one"}}},
		{"cut after a key",
			`{"title":"T","angle":"A","sections":`,
			planShape{Title: "T", Angle: "A"}},
		{"cut after opening brace",
			`{"title":"T",`,
			planShape{Title: "T"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got planShape
			if err := DecodeJSON(tc.reply, &got); err != nil {
				t.Fatalf("DecodeJSON: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// Repair must never invent content — a reply with no JSON in it stays an error.
func TestDecodeJSONRejectsAReplyWithNoJSON(t *testing.T) {
	var got planShape
	for _, reply := range []string{"", "I cannot help with that.", "```\n\n```"} {
		if err := DecodeJSON(reply, &got); err == nil {
			t.Errorf("DecodeJSON(%q) = nil error, want failure", reply)
		}
	}
}

// A nested object must not be closed early by an inner bracket.
func TestExtractJSONHandlesNesting(t *testing.T) {
	src := `{"a":{"b":[1,2,{"c":3}]},"d":"e"}`
	if got := ExtractJSON("noise " + src + " more noise"); got != src {
		t.Errorf("ExtractJSON = %q, want %q", got, src)
	}
}

// An escaped quote inside a string must not be read as the string's end.
func TestExtractJSONHandlesEscapedQuotes(t *testing.T) {
	src := `{"title":"a \"quoted\" word"}`
	if got := ExtractJSON(src + " trailing"); got != src {
		t.Errorf("ExtractJSON = %q, want %q", got, src)
	}
}

func TestDecodeJSONArrayValued(t *testing.T) {
	var got []string
	if err := DecodeJSON("```json\n[\"a\",\"b\"]\n```", &got); err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %v", got)
	}
}
