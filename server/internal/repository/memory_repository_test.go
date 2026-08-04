package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
)

// fakeEmbedder is a DB-less stand-in for llm.Embedder used to exercise the
// memory repository's embed-on-write / cosine-recall decision logic.
type fakeEmbedder struct {
	vec  []float32
	err  error
	last []string
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.last = texts
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = f.vec
	}
	return out, nil
}

func TestMemoryEmbedText(t *testing.T) {
	ctx := context.Background()

	t.Run("no embedder returns nil (ILIKE fallback)", func(t *testing.T) {
		r := &memoryRepository{}
		if got := r.embedText(ctx, "hello"); got != nil {
			t.Errorf("embedText without embedder = %v; want nil", got)
		}
	})

	t.Run("empty text returns nil", func(t *testing.T) {
		r := &memoryRepository{embedder: &fakeEmbedder{vec: []float32{0.1}}}
		if got := r.embedText(ctx, ""); got != nil {
			t.Errorf("embedText(\"\") = %v; want nil", got)
		}
	})

	t.Run("embedder error falls back to nil", func(t *testing.T) {
		r := &memoryRepository{embedder: &fakeEmbedder{err: errors.New("boom")}}
		if got := r.embedText(ctx, "hello"); got != nil {
			t.Errorf("embedText on error = %v; want nil", got)
		}
	})

	t.Run("empty vector falls back to nil", func(t *testing.T) {
		r := &memoryRepository{embedder: &fakeEmbedder{vec: []float32{}}}
		if got := r.embedText(ctx, "hello"); got != nil {
			t.Errorf("embedText with empty vector = %v; want nil", got)
		}
	})

	t.Run("configured embedder returns the query vector", func(t *testing.T) {
		fe := &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3}}
		r := &memoryRepository{embedder: fe}
		got := r.embedText(ctx, "recall this")
		if len(got) != 3 || got[0] != 0.1 || got[2] != 0.3 {
			t.Fatalf("embedText = %v; want [0.1 0.2 0.3]", got)
		}
		if len(fe.last) != 1 || fe.last[0] != "recall this" {
			t.Errorf("embedder called with %v; want single query text", fe.last)
		}
	})
}

func TestMemoryEmbedInput(t *testing.T) {
	if got := memoryEmbedInput(&model.AgentMemoryLongTerm{Content: "c", Summary: "s"}); got != "c" {
		t.Errorf("prefer content: got %q", got)
	}
	if got := memoryEmbedInput(&model.AgentMemoryLongTerm{Summary: "s"}); got != "s" {
		t.Errorf("fall back to summary: got %q", got)
	}
	if got := memoryEmbedInput(&model.AgentMemoryLongTerm{}); got != "" {
		t.Errorf("empty memory: got %q", got)
	}
}

// TestLongTermSearchSQL locks in the cosine query building and the ILIKE
// fallback so the two paths cannot silently converge or drop their operators.
func TestLongTermSearchSQL(t *testing.T) {
	if !strings.Contains(longTermVectorSearchSQL, "embedding <=> $2::vector") {
		t.Errorf("vector search SQL must order by cosine distance:\n%s", longTermVectorSearchSQL)
	}
	if !strings.Contains(longTermVectorSearchSQL, "embedding IS NOT NULL") {
		t.Errorf("vector search must skip pre-backfill NULL rows:\n%s", longTermVectorSearchSQL)
	}
	if !strings.Contains(longTermTextSearchSQL, "ILIKE") {
		t.Errorf("fallback search SQL must use ILIKE:\n%s", longTermTextSearchSQL)
	}
	if strings.Contains(longTermTextSearchSQL, "<=>") {
		t.Errorf("fallback search SQL must not use vector operators:\n%s", longTermTextSearchSQL)
	}
	if !strings.Contains(appendLongTermSQL, "$7::vector") {
		t.Errorf("append SQL must store the embedding as a vector cast:\n%s", appendLongTermSQL)
	}
}

// TestVectorStringForEmbedding verifies the query vector rendered for the
// cosine search matches pgvector's expected literal form.
func TestVectorStringForEmbedding(t *testing.T) {
	if got := VectorString([]float32{0.1, 0.2, 0.3}); got != "[0.1,0.2,0.3]" {
		t.Errorf("VectorString = %q; want [0.1,0.2,0.3]", got)
	}
}
