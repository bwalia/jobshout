package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/research"
)

// fakeResearch stands in for *research.Client so these tests exercise the tool
// contract — parameter handling, JSON shape, error propagation — with no network.
type fakeResearch struct {
	sources  []research.Source
	doc      *research.Document
	trending []research.TrendingItem
	err      error

	// gotQuery and gotLimit record what the tool passed through.
	gotQuery string
	gotLimit int
	gotURL   string
}

func (f *fakeResearch) Search(_ context.Context, query string, limit int) ([]research.Source, error) {
	f.gotQuery, f.gotLimit = query, limit
	return f.sources, f.err
}

func (f *fakeResearch) Fetch(_ context.Context, rawURL string) (*research.Document, error) {
	f.gotURL = rawURL
	return f.doc, f.err
}

func (f *fakeResearch) Trending(_ context.Context, limit int) ([]research.TrendingItem, error) {
	f.gotLimit = limit
	return f.trending, f.err
}

func TestNewResearchTools_RegistersTheExpectedSet(t *testing.T) {
	got := NewResearchTools(&fakeResearch{})

	want := map[string]bool{"web_search": false, "web_fetch": false, "trending_topics": false}
	for _, tool := range got {
		if _, known := want[tool.Name()]; !known {
			t.Errorf("unexpected tool %q", tool.Name())
			continue
		}
		want[tool.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("tool %q was not returned", name)
		}
	}
}

// Every research tool must advertise a schema: the executor only takes the
// native tool-calling path when all of an agent's tools implement
// SchemaProvider, so one missing schema quietly downgrades the whole agent to
// the ReAct prompt loop.
func TestResearchTools_AllProvideSchemas(t *testing.T) {
	for _, tool := range NewResearchTools(&fakeResearch{}) {
		sp, ok := tool.(SchemaProvider)
		if !ok {
			t.Errorf("tool %q does not implement SchemaProvider", tool.Name())
			continue
		}
		schema := sp.Parameters()
		if schema["type"] != "object" {
			t.Errorf("tool %q schema type = %v, want object", tool.Name(), schema["type"])
		}
		if _, ok := schema["properties"]; !ok {
			t.Errorf("tool %q schema has no properties", tool.Name())
		}
		if _, ok := schema["required"]; !ok {
			t.Errorf("tool %q schema omits required", tool.Name())
		}
	}
}

func TestWebSearch_PassesQueryAndDefaultsLimit(t *testing.T) {
	fake := &fakeResearch{sources: []research.Source{{URL: "https://a.com/1", Title: "One"}}}
	tool := &WebSearchTool{client: fake}

	out, err := tool.Execute(context.Background(), map[string]any{"query": "gateway api"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if fake.gotQuery != "gateway api" {
		t.Errorf("passed query %q, want %q", fake.gotQuery, "gateway api")
	}
	if fake.gotLimit != research.DefaultLimit {
		t.Errorf("passed limit %d, want the default %d", fake.gotLimit, research.DefaultLimit)
	}

	var parsed struct {
		Sources []research.Source `json:"sources"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(parsed.Sources) != 1 || parsed.Sources[0].Title != "One" {
		t.Errorf("got sources %+v", parsed.Sources)
	}
}

func TestWebSearch_MissingQueryIsAnError(t *testing.T) {
	tool := &WebSearchTool{client: &fakeResearch{}}
	if _, err := tool.Execute(context.Background(), map[string]any{}); err == nil {
		t.Fatal("Execute accepted a call with no query")
	}
}

// No results is an answer, not a malfunction. Returning an error would tell the
// model its tool is broken instead of prompting it to try other terms.
func TestWebSearch_NoResultsIsNotAnError(t *testing.T) {
	tool := &WebSearchTool{client: &fakeResearch{sources: nil}}

	out, err := tool.Execute(context.Background(), map[string]any{"query": "zzzz"})
	if err != nil {
		t.Fatalf("Execute returned error for an empty result set: %v", err)
	}
	if !strings.Contains(out, "No sources found") {
		t.Errorf("output %q does not tell the model the search was empty", out)
	}
}

func TestWebSearch_BackendErrorPropagates(t *testing.T) {
	tool := &WebSearchTool{client: &fakeResearch{err: fmt.Errorf("backend down")}}
	if _, err := tool.Execute(context.Background(), map[string]any{"query": "x"}); err == nil {
		t.Fatal("Execute swallowed a backend failure")
	}
}

func TestToolLimit(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		want  int
	}{
		{"absent uses default", map[string]any{}, research.DefaultLimit},
		{"json float", map[string]any{"limit": float64(7)}, 7},
		{"numeric string", map[string]any{"limit": "7"}, 7},
		{"zero uses default", map[string]any{"limit": float64(0)}, research.DefaultLimit},
		{"negative uses default", map[string]any{"limit": float64(-3)}, research.DefaultLimit},
		{"above max is capped", map[string]any{"limit": float64(500)}, maxToolResults},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toolLimit(tt.input); got != tt.want {
				t.Errorf("toolLimit(%v) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestWebFetch_ReturnsDocument(t *testing.T) {
	fake := &fakeResearch{doc: &research.Document{
		Source: research.Source{URL: "https://a.com/1", Title: "One", Site: "a.com"},
		Text:   "the body",
	}}
	tool := &WebFetchTool{client: fake}

	out, err := tool.Execute(context.Background(), map[string]any{"url": "https://a.com/1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if fake.gotURL != "https://a.com/1" {
		t.Errorf("passed url %q", fake.gotURL)
	}

	var doc research.Document
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if doc.Text != "the body" {
		t.Errorf("got text %q", doc.Text)
	}
}

// A page that cannot be read must surface as an error. If it came back as an
// empty document the model could not distinguish "read it, does not support the
// claim" from "never read it" — which is exactly how a dead link gets cited.
func TestWebFetch_UnreadablePageIsAnError(t *testing.T) {
	tool := &WebFetchTool{client: &fakeResearch{err: fmt.Errorf("target returned HTTP 404")}}

	_, err := tool.Execute(context.Background(), map[string]any{"url": "https://a.com/gone"})
	if err == nil {
		t.Fatal("Execute reported success for an unreadable page")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error %q lost the underlying reason", err)
	}
}

func TestWebFetch_MissingURLIsAnError(t *testing.T) {
	tool := &WebFetchTool{client: &fakeResearch{}}
	if _, err := tool.Execute(context.Background(), map[string]any{}); err == nil {
		t.Fatal("Execute accepted a call with no url")
	}
}

func TestTrendingTopics_NeedsNoQuery(t *testing.T) {
	fake := &fakeResearch{trending: []research.TrendingItem{
		{Source: research.Source{URL: "https://a.com/1", Title: "Something"}, Score: 300, Channel: "hackernews"},
	}}
	tool := &TrendingTopicsTool{client: fake}

	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	var parsed struct {
		Items []research.TrendingItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(parsed.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(parsed.Items))
	}
	if parsed.Items[0].Score != 300 || parsed.Items[0].Channel != "hackernews" {
		t.Errorf("ranking signal lost: %+v", parsed.Items[0])
	}
}

func TestTrendingTopics_RequiredIsEmptyNotAbsent(t *testing.T) {
	// A schema whose "required" key is missing is interpreted differently by
	// different providers. ObjectSchema always emits the array; this guards the
	// no-required-parameters case specifically.
	schema := (&TrendingTopicsTool{}).Parameters()
	req, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required is %T, want []string", schema["required"])
	}
	if len(req) != 0 {
		t.Errorf("got required %v, want empty", req)
	}
}

// The registry panics on duplicate names, so a collision with an existing tool
// would be a startup crash rather than a test failure. Catch it here.
func TestResearchTools_NamesDoNotCollideWithBuiltins(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewHTTPTool())
	reg.Register(NewShellTool(nil))

	for _, tool := range NewResearchTools(&fakeResearch{}) {
		if _, exists := reg.Get(tool.Name()); exists {
			t.Fatalf("tool %q collides with an already-registered tool", tool.Name())
		}
		reg.Register(tool)
	}
}
