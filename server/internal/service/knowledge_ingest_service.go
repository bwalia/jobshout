package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// Default chunking parameters. Chunks are ~800 characters with ~100 characters
// of overlap so that context spanning a boundary is preserved in both chunks.
const (
	defaultChunkSize    = 800
	defaultChunkOverlap = 100
)

// KnowledgeIngestService turns a knowledge file's raw content into embedded
// chunks stored in the vector store. Ingestion is best-effort: when no embedder
// is configured, or embedding fails, it logs and returns without error so that
// knowledge-file CRUD is never blocked.
type KnowledgeIngestService interface {
	// IngestFile re-chunks and re-embeds a knowledge file, replacing any
	// previously stored chunks for that file.
	IngestFile(ctx context.Context, orgID, agentID, fileID uuid.UUID, content string) error
	// DeleteFile removes all stored chunks for a knowledge file.
	DeleteFile(ctx context.Context, fileID uuid.UUID) error
}

type knowledgeIngestService struct {
	repo     repository.KnowledgeChunkRepository
	embedder llm.Embedder // may be nil when embeddings are not configured
	logger   *zap.Logger

	chunkSize    int
	chunkOverlap int
}

// NewKnowledgeIngestService constructs a KnowledgeIngestService. embedder may be
// nil, in which case ingestion becomes a no-op (chunks are cleared but not
// re-embedded).
func NewKnowledgeIngestService(repo repository.KnowledgeChunkRepository, embedder llm.Embedder, logger *zap.Logger) KnowledgeIngestService {
	return &knowledgeIngestService{
		repo:         repo,
		embedder:     embedder,
		logger:       logger,
		chunkSize:    defaultChunkSize,
		chunkOverlap: defaultChunkOverlap,
	}
}

func (s *knowledgeIngestService) IngestFile(ctx context.Context, orgID, agentID, fileID uuid.UUID, content string) error {
	// Always clear stale chunks first so the vector store stays in sync with
	// the file even when embeddings are unavailable.
	if err := s.repo.DeleteByFile(ctx, fileID); err != nil {
		s.logger.Warn("knowledge_ingest: failed to delete old chunks",
			zap.String("file_id", fileID.String()), zap.Error(err))
		// Continue — a failed delete should not block re-ingestion attempts.
	}

	if s.embedder == nil {
		s.logger.Info("knowledge_ingest: embedder not configured, skipping embedding",
			zap.String("file_id", fileID.String()))
		return nil
	}

	chunks := ChunkText(content, s.chunkSize, s.chunkOverlap)
	if len(chunks) == 0 {
		return nil
	}

	vectors, err := s.embedder.Embed(ctx, chunks)
	if err != nil {
		s.logger.Warn("knowledge_ingest: embedding failed",
			zap.String("file_id", fileID.String()), zap.Error(err))
		return nil // best-effort: do not break CRUD
	}
	if len(vectors) != len(chunks) {
		s.logger.Warn("knowledge_ingest: embedding count mismatch",
			zap.Int("chunks", len(chunks)), zap.Int("vectors", len(vectors)))
		return nil
	}

	records := make([]model.KnowledgeChunk, 0, len(chunks))
	for i, chunk := range chunks {
		records = append(records, model.KnowledgeChunk{
			OrgID:           orgID,
			AgentID:         agentID,
			KnowledgeFileID: fileID,
			ChunkIndex:      i,
			Content:         chunk,
			Embedding:       vectors[i],
			Metadata:        map[string]any{},
		})
	}

	if err := s.repo.Create(ctx, records); err != nil {
		s.logger.Warn("knowledge_ingest: failed to store chunks",
			zap.String("file_id", fileID.String()), zap.Error(err))
		return nil
	}

	s.logger.Info("knowledge_ingest: file ingested",
		zap.String("file_id", fileID.String()), zap.Int("chunks", len(records)))
	return nil
}

func (s *knowledgeIngestService) DeleteFile(ctx context.Context, fileID uuid.UUID) error {
	return s.repo.DeleteByFile(ctx, fileID)
}

// ChunkText splits text into overlapping chunks of roughly size characters with
// overlap characters of trailing context carried into the next chunk. It prefers
// to break on paragraph, then sentence, then line boundaries within the second
// half of each window so chunks end at natural points. The output is
// deterministic for a given input.
func ChunkText(text string, size, overlap int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if size <= 0 {
		size = defaultChunkSize
	}
	if overlap < 0 || overlap >= size {
		overlap = 0
	}

	runes := []rune(text)
	if len(runes) <= size {
		return []string{string(runes)}
	}

	var chunks []string
	start := 0
	for start < len(runes) {
		end := start + size
		if end >= len(runes) {
			if chunk := strings.TrimSpace(string(runes[start:])); chunk != "" {
				chunks = append(chunks, chunk)
			}
			break
		}

		breakAt := findBreak(runes, start, end)
		if chunk := strings.TrimSpace(string(runes[start:breakAt])); chunk != "" {
			chunks = append(chunks, chunk)
		}

		next := breakAt - overlap
		if next <= start {
			// Guarantee forward progress even with pathological overlaps.
			next = breakAt
		}
		start = next
	}
	return chunks
}

// findBreak returns the index just past a natural boundary within
// runes[start:end], searching only the second half of the window so chunks do
// not become too small. It falls back to end when no boundary is found.
func findBreak(runes []rune, start, end int) int {
	minBreak := start + (end-start)/2

	// Paragraph boundary: a blank line ("\n\n").
	for i := end - 1; i > minBreak; i-- {
		if runes[i] == '\n' && i-1 >= start && runes[i-1] == '\n' {
			return i + 1
		}
	}
	// Sentence boundary: '.', '!' or '?' followed by whitespace.
	for i := end - 1; i > minBreak; i-- {
		if runes[i] == '.' || runes[i] == '!' || runes[i] == '?' {
			if i+1 < len(runes) && (runes[i+1] == ' ' || runes[i+1] == '\n' || runes[i+1] == '\t') {
				return i + 1
			}
		}
	}
	// Line boundary: a single newline.
	for i := end - 1; i > minBreak; i-- {
		if runes[i] == '\n' {
			return i + 1
		}
	}
	return end
}
