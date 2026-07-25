package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// KnowledgeSearcher returns an agent's stored knowledge chunks ranked by
// similarity to a query embedding (nearest first). It is satisfied by
// repository.KnowledgeChunkRepository — declared here (not imported) so the
// tools package stays decoupled from the repository layer, mirroring the
// IntegrationConfigStore / executor.SkillProvider pattern.
type KnowledgeSearcher interface {
	SearchByAgent(ctx context.Context, agentID uuid.UUID, queryEmbedding []float32, k int) ([]model.KnowledgeChunk, error)
}

// Embedder converts a batch of texts into dense vectors suitable for
// similarity search. It is satisfied by llm.Embedder (a superset interface) —
// declared narrowly here so the tools package does not import the llm package.
type Embedder interface {
	// Embed returns one embedding vector per input text, in the same order.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// defaultKnowledgeK is the number of passages knowledge_search returns when the
// caller does not specify k.
const defaultKnowledgeK = 5

// errNoAgent is returned when knowledge_search is invoked outside an
// agent-scoped execution (the executor injects the agent id via WithAgent).
var errNoAgent = fmt.Errorf("no agent in context: knowledge_search requires an agent-scoped execution")

// KnowledgeTool implements semantic retrieval over an agent's knowledge base.
// It embeds the query and returns the most similar stored chunks. It satisfies
// both Tool and SchemaProvider, so it works on the ReAct loop and the native
// tool-calling path.
type KnowledgeTool struct {
	embedder Embedder
	searcher KnowledgeSearcher
}

// NewKnowledgeTool builds a knowledge_search tool backed by the given embedder
// and chunk searcher.
func NewKnowledgeTool(embedder Embedder, searcher KnowledgeSearcher) *KnowledgeTool {
	return &KnowledgeTool{embedder: embedder, searcher: searcher}
}

// Name is the identifier the LLM uses to select this tool.
func (t *KnowledgeTool) Name() string { return "knowledge_search" }

// Description explains the tool to the model; included verbatim in the prompt.
func (t *KnowledgeTool) Description() string {
	return `Search this agent's knowledge base for passages relevant to a query using semantic (vector) similarity.
Input parameters:
  query (string, required) - The natural-language question or topic to search for.
  k     (integer, optional) - Maximum number of passages to return (default 5).
Returns a JSON array of the most relevant passages, most-similar first.`
}

// Parameters advertises the JSON-Schema for this tool's inputs so providers that
// support native tool-calling receive a real function definition.
func (t *KnowledgeTool) Parameters() ParameterSchema {
	return ObjectSchema(map[string]any{
		"query": map[string]any{
			"type":        "string",
			"description": "The natural-language question or topic to search for",
		},
		"k": map[string]any{
			"type":        "integer",
			"description": "Maximum number of passages to return (default 5)",
		},
	}, "query")
}

// knowledgeHit is a single search result serialized back to the model.
type knowledgeHit struct {
	Content  string  `json:"content"`
	Distance float64 `json:"distance"`
}

// Execute embeds the query and returns the agent's top-k most similar knowledge
// chunks as a JSON array.
func (t *KnowledgeTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	agentID, ok := AgentFromContext(ctx)
	if !ok {
		return "", errNoAgent
	}

	query, err := stringParam(input, "query", true)
	if err != nil {
		return "", err
	}

	k := intParam(input, "k", defaultKnowledgeK)
	if k <= 0 {
		k = defaultKnowledgeK
	}

	vectors, err := t.embedder.Embed(ctx, []string{query})
	if err != nil {
		return "", fmt.Errorf("knowledge_search: embed query: %w", err)
	}
	if len(vectors) == 0 {
		return "", fmt.Errorf("knowledge_search: embedder returned no vectors")
	}

	chunks, err := t.searcher.SearchByAgent(ctx, agentID, vectors[0], k)
	if err != nil {
		return "", fmt.Errorf("knowledge_search: %w", err)
	}

	results := make([]knowledgeHit, 0, len(chunks))
	for _, c := range chunks {
		results = append(results, knowledgeHit{Content: c.Content, Distance: c.Distance})
	}

	out, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("knowledge_search: marshal results: %w", err)
	}
	return string(out), nil
}

// intParam extracts an integer parameter, tolerating JSON's float64 numbers,
// json.Number, and numeric strings (tool inputs arrive from decoded JSON, so an
// integer typically presents as float64). Returns def when the key is absent,
// null, or not parseable as an integer.
func intParam(input map[string]any, key string, def int) int {
	v, ok := input[key]
	if !ok || v == nil {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
		}
	}
	return def
}
