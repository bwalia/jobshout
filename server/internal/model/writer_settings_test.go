package model

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func writerAgent(cfg map[string]any) *Agent {
	return &Agent{ID: uuid.New(), EngineConfig: cfg}
}

func TestWriterSettingsOfReadsListOrCommaString(t *testing.T) {
	for _, focus := range []any{
		[]any{"Agentic AI", " DevOps ", ""},
		"Agentic AI, DevOps,",
	} {
		ws := WriterSettingsOf(writerAgent(map[string]any{
			EngineConfigWriterStyle:       " Business and tech, with tips. ",
			EngineConfigWriterFocus:       focus,
			EngineConfigWriterDestination: "Workstation Website",
		}))
		if ws.Style != "Business and tech, with tips." {
			t.Errorf("style = %q", ws.Style)
		}
		if want := []string{"Agentic AI", "DevOps"}; !reflect.DeepEqual(ws.Focus, want) {
			t.Errorf("focus from %#v = %#v, want %#v", focus, ws.Focus, want)
		}
		if ws.Destination != "Workstation Website" {
			t.Errorf("destination = %q", ws.Destination)
		}
	}
	if ws := WriterSettingsOf(nil); !reflect.DeepEqual(ws, WriterSettings{}) {
		t.Errorf("nil agent settings = %#v, want none", ws)
	}
}

// One agent serves several schedules: what a schedule names wins, and only
// what it leaves unset comes from the agent.
func TestApplyWriterSettingsRequestWins(t *testing.T) {
	ws := WriterSettings{
		Style:    "agent style",
		Audience: "business",
		Industry: "Banking",
		Focus:    []string{"Agentic AI"},
	}

	req := GenerateBlogRequest{Trending: true, Industry: "Healthtech", Focus: []string{"SRE"}}
	req.ApplyWriterSettings(ws)
	if req.Industry != "Healthtech" || !reflect.DeepEqual(req.Focus, []string{"SRE"}) {
		t.Errorf("schedule's own values were overridden: %+v", req)
	}
	if req.Style != "agent style" || req.Audience != "business" {
		t.Errorf("unset values not filled from the agent: %+v", req)
	}

	bare := GenerateBlogRequest{Trending: true}
	bare.ApplyWriterSettings(ws)
	if !reflect.DeepEqual(bare.Focus, []string{"Agentic AI"}) {
		t.Errorf("trending run without focus got %#v, want the agent's keywords", bare.Focus)
	}

	fixed := GenerateBlogRequest{Briefs: []BlogBrief{{Topic: "x"}}}
	fixed.ApplyWriterSettings(ws)
	if len(fixed.Focus) != 0 {
		t.Errorf("a run given its topic must not gain focus keywords: %#v", fixed.Focus)
	}
}

// The style rides on every brief, so a retry writes the same way.
func TestNormalizeCarriesStyleToBriefs(t *testing.T) {
	req := GenerateBlogRequest{
		Style:  " house style ",
		Briefs: []BlogBrief{{Topic: "a"}, {Topic: "b", Style: "own style"}},
		Topics: []string{"c"},
	}
	req.Normalize()
	got := map[string]string{}
	for _, b := range req.Briefs {
		got[b.Topic] = b.Style
	}
	want := map[string]string{"a": "house style", "b": "own style", "c": "house style"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("brief styles = %#v, want %#v", got, want)
	}
	if req.RunOptions().Style != "house style" {
		t.Errorf("run options style = %q", req.RunOptions().Style)
	}
}
