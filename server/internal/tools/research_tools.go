package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jobshout/server/internal/research"
)

// ResearchClient is the slice of *research.Client these tools consume. Declared
// here, where it is used, so a test can substitute a fake without any network —
// the same pattern KnowledgeSearcher and IntegrationConfigStore follow.
type ResearchClient interface {
	Search(ctx context.Context, query string, limit int) ([]research.Source, error)
	Fetch(ctx context.Context, rawURL string) (*research.Document, error)
	Trending(ctx context.Context, limit int) ([]research.TrendingItem, error)
}

// NewResearchTools builds the three internet-access tools over one client.
//
// They are returned together because they are only useful together: search
// finds candidate URLs, fetch reads one, and trending answers the case where
// there is no query yet. Registering them as a set is also what lets an
// operator grant "can research the web" as a single decision in the agent's
// tool permissions rather than three unrelated checkboxes.
func NewResearchTools(client ResearchClient) []Tool {
	return []Tool{
		&WebSearchTool{client: client},
		&WebFetchTool{client: client},
		&TrendingTopicsTool{client: client},
	}
}

// maxToolResults bounds what any of these tools will return in one call. The
// result is going straight into an LLM context window, and a large result set
// crowds out the work the model is actually doing.
const maxToolResults = 25

// WebSearchTool finds published sources on a topic.
type WebSearchTool struct{ client ResearchClient }

// Name is the identifier the LLM uses to select this tool.
func (t *WebSearchTool) Name() string { return "web_search" }

// Description explains the tool to the model; included verbatim in the prompt.
func (t *WebSearchTool) Description() string {
	return `Search the internet for published articles, discussions and papers about a topic.
Input parameters:
  query (string, required) - What to search for, e.g. "kubernetes gateway api ga"
  limit (integer, optional) - Maximum sources to return (default 10, max 25)

Returns a JSON array of sources, each with url, title, site, published_at and a
short excerpt. Sources are candidates only — the excerpt is not quotable and the
page has NOT been read. Use web_fetch to read a source before citing it.`
}

// Parameters advertises the JSON-Schema for this tool's inputs so providers
// that support native tool-calling receive a real function definition.
func (t *WebSearchTool) Parameters() ParameterSchema {
	return ObjectSchema(map[string]any{
		"query": map[string]any{
			"type":        "string",
			"description": `What to search for, e.g. "kubernetes gateway api ga"`,
		},
		"limit": map[string]any{
			"type":        "integer",
			"description": "Maximum sources to return (default 10, max 25)",
		},
	}, "query")
}

// Execute runs the search and returns the sources as JSON.
func (t *WebSearchTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	query, err := stringParam(input, "query", true)
	if err != nil {
		return "", fmt.Errorf("web_search: %w", err)
	}

	sources, err := t.client.Search(ctx, query, toolLimit(input))
	if err != nil {
		return "", fmt.Errorf("web_search: %w", err)
	}

	// An empty result is a legitimate answer, not a failure: the model needs to
	// learn that this query found nothing so it can try a different one, and
	// returning an error would instead read as "the tool is broken".
	if len(sources) == 0 {
		return `{"sources":[],"note":"No sources found for this query. Try different or broader terms."}`, nil
	}

	out, err := json.Marshal(map[string]any{"sources": sources})
	if err != nil {
		return "", fmt.Errorf("web_search: marshal results: %w", err)
	}
	return string(out), nil
}

// WebFetchTool retrieves one page and returns its readable text.
type WebFetchTool struct{ client ResearchClient }

// Name is the identifier the LLM uses to select this tool.
func (t *WebFetchTool) Name() string { return "web_fetch" }

// Description explains the tool to the model; included verbatim in the prompt.
func (t *WebFetchTool) Description() string {
	return `Retrieve a web page and return its readable text, with navigation and ads stripped.
Input parameters:
  url (string, required) - Full http(s) URL of the page to read

Returns the page's title, resolved url and extracted text as markdown.
Fails if the page does not exist or cannot be retrieved — a URL that errors here
is not safe to cite. Only cite a source after fetching it successfully.`
}

// Parameters advertises the JSON-Schema for this tool's inputs.
func (t *WebFetchTool) Parameters() ParameterSchema {
	return ObjectSchema(map[string]any{
		"url": map[string]any{
			"type":        "string",
			"description": "Full http(s) URL of the page to read",
		},
	}, "url")
}

// Execute fetches the page.
//
// A failure is returned as an error rather than an empty document on purpose.
// The model must be able to tell "I read this and it does not support the
// claim" from "I could not read this at all" — collapsing the two is how a
// dead link ends up in a reference list.
func (t *WebFetchTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	rawURL, err := stringParam(input, "url", true)
	if err != nil {
		return "", fmt.Errorf("web_fetch: %w", err)
	}

	doc, err := t.client.Fetch(ctx, rawURL)
	if err != nil {
		return "", fmt.Errorf("web_fetch: %w", err)
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("web_fetch: marshal document: %w", err)
	}
	return string(out), nil
}

// TrendingTopicsTool lists what is currently being published and discussed.
type TrendingTopicsTool struct{ client ResearchClient }

// Name is the identifier the LLM uses to select this tool.
func (t *TrendingTopicsTool) Name() string { return "trending_topics" }

// Description explains the tool to the model; included verbatim in the prompt.
func (t *TrendingTopicsTool) Description() string {
	return `List what is currently trending across technology, AI and infrastructure.
Takes no query — it reports what is being published and discussed right now,
drawn from Hacker News, engineering blogs and arXiv.
Input parameters:
  limit (integer, optional) - Maximum items to return (default 10, max 25)

Returns a JSON array of items with url, title, site, published_at, score and the
channel each came from. Use this when you need to find a subject to write about
rather than research one you already have.`
}

// Parameters advertises the JSON-Schema for this tool's inputs.
func (t *TrendingTopicsTool) Parameters() ParameterSchema {
	return ObjectSchema(map[string]any{
		"limit": map[string]any{
			"type":        "integer",
			"description": "Maximum items to return (default 10, max 25)",
		},
	})
}

// Execute sweeps the trending backends.
func (t *TrendingTopicsTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	items, err := t.client.Trending(ctx, toolLimit(input))
	if err != nil {
		return "", fmt.Errorf("trending_topics: %w", err)
	}

	out, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		return "", fmt.Errorf("trending_topics: marshal results: %w", err)
	}
	return string(out), nil
}

// toolLimit reads the shared optional "limit" parameter, bounded to something a
// context window can absorb.
func toolLimit(input map[string]any) int {
	n := intParam(input, "limit", research.DefaultLimit)
	if n <= 0 {
		return research.DefaultLimit
	}
	if n > maxToolResults {
		return maxToolResults
	}
	return n
}
