package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// fakeEmbedder returns a fixed vector per text (or an error). It records the
// texts it was asked to embed.
type fakeEmbedder struct {
	vec  []float32
	err  error
	seen []string
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.seen = append(f.seen, texts...)
	if f.err != nil {
		return nil, f.err
	}
	if f.vec == nil { // simulate an embedder that returns no vectors
		return [][]float32{}, nil
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = f.vec
	}
	return out, nil
}

// fakeSearcher returns canned chunks (or an error) and records the arguments it
// was called with.
type fakeSearcher struct {
	chunks   []model.KnowledgeChunk
	err      error
	gotAgent uuid.UUID
	gotVec   []float32
	gotK     int
	calls    int
}

func (f *fakeSearcher) SearchByAgent(_ context.Context, agentID uuid.UUID, vec []float32, k int) ([]model.KnowledgeChunk, error) {
	f.calls++
	f.gotAgent = agentID
	f.gotVec = vec
	f.gotK = k
	return f.chunks, f.err
}

func chunk(content string, dist float64) model.KnowledgeChunk {
	return model.KnowledgeChunk{Content: content, Distance: dist}
}

func TestKnowledgeTool_Metadata(t *testing.T) {
	tool := NewKnowledgeTool(&fakeEmbedder{}, &fakeSearcher{})

	if tool.Name() != "knowledge_search" {
		t.Fatalf("Name: got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Fatal("Description must not be empty")
	}

	// It must be usable on the native path (implements SchemaProvider).
	var _ Tool = tool
	var _ SchemaProvider = tool

	schema := tool.Parameters()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing properties: %#v", schema)
	}
	if _, ok := props["query"]; !ok {
		t.Fatal("schema missing query property")
	}
	if _, ok := props["k"]; !ok {
		t.Fatal("schema missing k property")
	}
	req, ok := schema["required"].([]string)
	if !ok || len(req) != 1 || req[0] != "query" {
		t.Fatalf("query must be the sole required field: %#v", schema["required"])
	}
}

func TestKnowledgeTool_Execute(t *testing.T) {
	agentID := uuid.New()
	withAgent := func(ctx context.Context) context.Context { return WithAgent(ctx, agentID) }
	vec := []float32{0.1, 0.2, 0.3}

	tests := []struct {
		name         string
		ctx          func(context.Context) context.Context
		input        map[string]any
		embedder     *fakeEmbedder
		searcher     *fakeSearcher
		wantErr      bool
		wantK        int // expected k forwarded to the searcher (only if no error)
		wantContents []string
	}{
		{
			name:         "returns top chunks as JSON with default k",
			ctx:          withAgent,
			input:        map[string]any{"query": "how do refunds work?"},
			embedder:     &fakeEmbedder{vec: vec},
			searcher:     &fakeSearcher{chunks: []model.KnowledgeChunk{chunk("refunds take 5 days", 0.1), chunk("no refunds on sale items", 0.2)}},
			wantK:        defaultKnowledgeK,
			wantContents: []string{"refunds take 5 days", "no refunds on sale items"},
		},
		{
			name:         "honors explicit k (arriving as JSON float64)",
			ctx:          withAgent,
			input:        map[string]any{"query": "q", "k": float64(3)},
			embedder:     &fakeEmbedder{vec: vec},
			searcher:     &fakeSearcher{chunks: []model.KnowledgeChunk{chunk("a", 0.1)}},
			wantK:        3,
			wantContents: []string{"a"},
		},
		{
			name:         "honors explicit k as int",
			ctx:          withAgent,
			input:        map[string]any{"query": "q", "k": 7},
			embedder:     &fakeEmbedder{vec: vec},
			searcher:     &fakeSearcher{chunks: nil},
			wantK:        7,
			wantContents: []string{},
		},
		{
			name:         "non-positive k falls back to default",
			ctx:          withAgent,
			input:        map[string]any{"query": "q", "k": 0},
			embedder:     &fakeEmbedder{vec: vec},
			searcher:     &fakeSearcher{chunks: nil},
			wantK:        defaultKnowledgeK,
			wantContents: []string{},
		},
		{
			name:     "missing agent in context is an error",
			ctx:      func(ctx context.Context) context.Context { return ctx },
			input:    map[string]any{"query": "q"},
			embedder: &fakeEmbedder{vec: vec},
			searcher: &fakeSearcher{},
			wantErr:  true,
		},
		{
			name:     "missing query is an error",
			ctx:      withAgent,
			input:    map[string]any{},
			embedder: &fakeEmbedder{vec: vec},
			searcher: &fakeSearcher{},
			wantErr:  true,
		},
		{
			name:     "embedder error is surfaced",
			ctx:      withAgent,
			input:    map[string]any{"query": "q"},
			embedder: &fakeEmbedder{err: fmt.Errorf("boom")},
			searcher: &fakeSearcher{},
			wantErr:  true,
		},
		{
			name:     "embedder returning no vectors is an error",
			ctx:      withAgent,
			input:    map[string]any{"query": "q"},
			embedder: &fakeEmbedder{vec: nil},
			searcher: &fakeSearcher{},
			wantErr:  true,
		},
		{
			name:     "searcher error is surfaced",
			ctx:      withAgent,
			input:    map[string]any{"query": "q"},
			embedder: &fakeEmbedder{vec: vec},
			searcher: &fakeSearcher{err: fmt.Errorf("db down")},
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool := NewKnowledgeTool(tc.embedder, tc.searcher)
			out, err := tool.Execute(tc.ctx(context.Background()), tc.input)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got output %q", out)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.searcher.gotAgent != agentID {
				t.Fatalf("searcher got agent %v, want %v", tc.searcher.gotAgent, agentID)
			}
			if tc.searcher.gotK != tc.wantK {
				t.Fatalf("searcher got k=%d, want %d", tc.searcher.gotK, tc.wantK)
			}
			// The query embedding must have been forwarded to the searcher.
			if len(tc.searcher.gotVec) != len(vec) {
				t.Fatalf("searcher got vec len %d, want %d", len(tc.searcher.gotVec), len(vec))
			}

			var hits []knowledgeHit
			if err := json.Unmarshal([]byte(out), &hits); err != nil {
				t.Fatalf("output is not valid JSON: %v (%q)", err, out)
			}
			if len(hits) != len(tc.wantContents) {
				t.Fatalf("got %d hits, want %d: %#v", len(hits), len(tc.wantContents), hits)
			}
			for i, want := range tc.wantContents {
				if hits[i].Content != want {
					t.Fatalf("hit %d content: got %q, want %q", i, hits[i].Content, want)
				}
			}
		})
	}
}

func TestKnowledgeTool_EmbedsTheQuery(t *testing.T) {
	emb := &fakeEmbedder{vec: []float32{1}}
	tool := NewKnowledgeTool(emb, &fakeSearcher{})
	ctx := WithAgent(context.Background(), uuid.New())

	if _, err := tool.Execute(ctx, map[string]any{"query": "hello world"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(emb.seen) != 1 || emb.seen[0] != "hello world" {
		t.Fatalf("embedder should have embedded the query, saw %#v", emb.seen)
	}
}

func TestIntParam(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		def   int
		want  int
	}{
		{"absent", map[string]any{}, 5, 5},
		{"nil value", map[string]any{"k": nil}, 5, 5},
		{"float64", map[string]any{"k": float64(3)}, 5, 3},
		{"int", map[string]any{"k": 9}, 5, 9},
		{"int64", map[string]any{"k": int64(4)}, 5, 4},
		{"numeric string", map[string]any{"k": "8"}, 5, 8},
		{"json.Number", map[string]any{"k": json.Number("6")}, 5, 6},
		{"unparseable string", map[string]any{"k": "abc"}, 5, 5},
		{"wrong type", map[string]any{"k": []int{1}}, 5, 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := intParam(tc.input, "k", tc.def); got != tc.want {
				t.Fatalf("intParam(%#v) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}
