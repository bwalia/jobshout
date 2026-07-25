package llm

import "context"

// Embedder is the interface every embedding provider must satisfy. It converts
// a batch of texts into dense vectors suitable for cosine-similarity search in
// a vector store (see repository.KnowledgeChunkRepository).
type Embedder interface {
	// Embed returns one embedding vector per input text, in the same order.
	// The length of every returned vector equals Dimensions().
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Dimensions returns the fixed dimensionality of the produced vectors.
	Dimensions() int
	// EmbedderName returns a human-readable name for logging/metrics.
	EmbedderName() string
}
