package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
)

func TestChunkText(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		size    int
		overlap int
		check   func(t *testing.T, chunks []string)
	}{
		{
			name: "empty",
			text: "   \n  ",
			size: 100, overlap: 10,
			check: func(t *testing.T, chunks []string) {
				if chunks != nil {
					t.Errorf("expected nil for blank input, got %v", chunks)
				}
			},
		},
		{
			name: "shorter than size returns single chunk",
			text: "hello world",
			size: 100, overlap: 10,
			check: func(t *testing.T, chunks []string) {
				if len(chunks) != 1 || chunks[0] != "hello world" {
					t.Errorf("got %v; want single chunk", chunks)
				}
			},
		},
		{
			name: "splits on paragraph boundary",
			text: strings.Repeat("a", 40) + "\n\n" + strings.Repeat("b", 40),
			size: 50, overlap: 5,
			check: func(t *testing.T, chunks []string) {
				if len(chunks) < 2 {
					t.Fatalf("expected multiple chunks, got %d", len(chunks))
				}
				if !strings.HasPrefix(chunks[0], "a") {
					t.Errorf("first chunk should start with a's: %q", chunks[0])
				}
			},
		},
		{
			name: "covers all content and makes progress",
			text: strings.Repeat("word. ", 500),
			size: 800, overlap: 100,
			check: func(t *testing.T, chunks []string) {
				if len(chunks) < 2 {
					t.Fatalf("expected several chunks, got %d", len(chunks))
				}
				joined := strings.Join(chunks, " ")
				if !strings.Contains(joined, "word.") {
					t.Errorf("content missing from chunks")
				}
			},
		},
		{
			name: "zero overlap still terminates",
			text: strings.Repeat("x", 2000),
			size: 300, overlap: 0,
			check: func(t *testing.T, chunks []string) {
				if len(chunks) == 0 {
					t.Fatal("expected chunks")
				}
			},
		},
		{
			name: "overlap >= size is treated as zero",
			text: strings.Repeat("y", 1000),
			size: 200, overlap: 500,
			check: func(t *testing.T, chunks []string) {
				if len(chunks) == 0 {
					t.Fatal("expected chunks and termination")
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chunks := ChunkText(c.text, c.size, c.overlap)
			c.check(t, chunks)
		})
	}
}

func TestChunkTextDeterministic(t *testing.T) {
	text := strings.Repeat("The quick brown fox. ", 200)
	a := ChunkText(text, 400, 50)
	b := ChunkText(text, 400, 50)
	if len(a) != len(b) {
		t.Fatalf("non-deterministic length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic chunk at %d", i)
		}
	}
}

// --- fakes -----------------------------------------------------------------

type fakeChunkRepo struct {
	created    []model.KnowledgeChunk
	deleted    []uuid.UUID
	failCreate bool
}

func (f *fakeChunkRepo) Create(ctx context.Context, chunks []model.KnowledgeChunk) error {
	f.created = append(f.created, chunks...)
	return nil
}
func (f *fakeChunkRepo) DeleteByFile(ctx context.Context, fileID uuid.UUID) error {
	f.deleted = append(f.deleted, fileID)
	return nil
}
func (f *fakeChunkRepo) SearchByAgent(ctx context.Context, agentID uuid.UUID, q []float32, k int) ([]model.KnowledgeChunk, error) {
	return nil, nil
}

type fakeEmbedder struct {
	dims  int
	calls int
}

func (e *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.calls++
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, e.dims)
	}
	return out, nil
}
func (e *fakeEmbedder) Dimensions() int      { return e.dims }
func (e *fakeEmbedder) EmbedderName() string { return "fake" }

func TestIngestFileWithEmbedder(t *testing.T) {
	repo := &fakeChunkRepo{}
	emb := &fakeEmbedder{dims: 4}
	svc := NewKnowledgeIngestService(repo, emb, zap.NewNop())

	orgID, agentID, fileID := uuid.New(), uuid.New(), uuid.New()
	content := strings.Repeat("Sentence one. Sentence two. ", 200)

	if err := svc.IngestFile(context.Background(), orgID, agentID, fileID, content); err != nil {
		t.Fatalf("IngestFile error: %v", err)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != fileID {
		t.Errorf("expected old chunks deleted for file, got %v", repo.deleted)
	}
	if emb.calls != 1 {
		t.Errorf("expected embedder called once, got %d", emb.calls)
	}
	if len(repo.created) == 0 {
		t.Fatal("expected chunks created")
	}
	for i, c := range repo.created {
		if c.ChunkIndex != i {
			t.Errorf("chunk %d has index %d", i, c.ChunkIndex)
		}
		if c.OrgID != orgID || c.AgentID != agentID || c.KnowledgeFileID != fileID {
			t.Errorf("chunk scoping incorrect: %+v", c)
		}
		if len(c.Embedding) != 4 {
			t.Errorf("chunk embedding dims = %d; want 4", len(c.Embedding))
		}
	}
}

func TestIngestFileWithoutEmbedder(t *testing.T) {
	repo := &fakeChunkRepo{}
	svc := NewKnowledgeIngestService(repo, nil, zap.NewNop())

	fileID := uuid.New()
	err := svc.IngestFile(context.Background(), uuid.New(), uuid.New(), fileID, "some content")
	if err != nil {
		t.Fatalf("IngestFile should be best-effort no-op: %v", err)
	}
	// Old chunks are still cleared to keep the store in sync.
	if len(repo.deleted) != 1 {
		t.Errorf("expected delete-by-file even without embedder, got %v", repo.deleted)
	}
	if len(repo.created) != 0 {
		t.Errorf("expected no chunks created without embedder, got %d", len(repo.created))
	}
}
