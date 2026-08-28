package research

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/llmtrace"
)

// Finding is one factual statement the agent is prepared to stand behind,
// bound to the source that supports it.
//
// A Finding cannot be constructed without a Quote and a SourceURL, and both are
// checked before it reaches a Brief. That is the whole point of the type: it
// makes "a claim with a citation attached" the only representable shape, so a
// consumer cannot accidentally use an unsourced assertion.
type Finding struct {
	// Claim is a single self-contained factual statement.
	Claim string `json:"claim"`
	// SourceURL is the document the claim was drawn from. It is always a URL
	// that was successfully retrieved — never one that merely appeared in
	// search results.
	SourceURL string `json:"source_url"`
	// Quote is the passage from that document which supports the claim,
	// verified to actually appear in the retrieved text.
	Quote string `json:"quote"`
}

// Brief is the Research Agent's output contract.
//
// This type, rather than the agent itself, is what makes the capability
// reusable: any caller that wants "go find out about X and come back with
// verified sources" consumes this. If the agent returned prose, the article
// pipeline could use it and nothing else could.
type Brief struct {
	Topic string `json:"topic"`
	// Summary is the agent's synthesis of what it learned, for a caller that
	// wants the shape of the subject before reading the findings.
	Summary string `json:"summary"`
	// Findings are the verified claims, each traceable to a Source below.
	Findings []Finding `json:"findings"`
	// Sources are the documents actually read and cited, de-duplicated. A
	// source that was read but supports no surviving finding is not included —
	// a reference list should be the sources the piece rests on, not every URL
	// the agent happened to open.
	Sources []Source `json:"sources"`
	// Queries records what was searched, so a run can be understood after the
	// fact and a poor result traced to a poor plan.
	Queries []string `json:"queries"`
	// Warnings records degradation that did not fail the run: sources that
	// could not be read, claims dropped for lack of support. Surfaced so a
	// thin brief is visibly thin rather than quietly so.
	Warnings []string `json:"warnings,omitempty"`
}

// IsUsable reports whether the brief has enough verified material to write
// from. Callers should refuse to generate an article from a brief that is not.
func (b *Brief) IsUsable() bool {
	return b != nil && len(b.Findings) > 0 && len(b.Sources) > 0
}

// Request is what the agent is asked to research.
type Request struct {
	// Topic is the subject. It is not a title — the agent researches the
	// subject and the caller decides what to call the result.
	Topic string
	// Context is the caller's extra guidance: angle, audience, points to hit,
	// things to avoid. Free text, passed to the planner verbatim.
	Context string
	// URLs, when non-empty, skip plan and search. Each URL is Fetch'd and
	// extracted; there is no web search, HN/arxiv, or search-pool recovery.
	URLs []string
	// Model optionally overrides the LLM used.
	Model string
}

// Agent researches a topic and returns verified findings.
type Agent struct {
	sources *Client
	llm     llm.Client
	logger  *zap.Logger
	cfg     AgentConfig
}

// AgentConfig bounds the work one Research call may do. The defaults are tuned
// for a single article: enough sources to say something substantive, few enough
// that a batch of articles finishes in a reasonable time on a local model.
type AgentConfig struct {
	// MaxQueries is how many distinct searches the planner may request.
	MaxQueries int
	// MaxSources is how many documents will actually be retrieved and read.
	// This is the dominant cost in both wall-clock and tokens.
	MaxSources int
	// FetchConcurrency bounds simultaneous retrievals.
	FetchConcurrency int
	// MinFindings is the floor below which the brief is reported unusable.
	MinFindings int
}

// DefaultAgentConfig returns the standard bounds.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		MaxQueries:       4,
		MaxSources:       6,
		FetchConcurrency: 3,
		MinFindings:      3,
	}
}

// NewAgent wires a Research Agent.
func NewAgent(sources *Client, llmClient llm.Client, cfg AgentConfig, logger *zap.Logger) *Agent {
	if cfg.MaxQueries <= 0 {
		cfg = DefaultAgentConfig()
	}
	return &Agent{sources: sources, llm: llmClient, cfg: cfg, logger: logger}
}

// ProgressFunc reports phase transitions so a caller can render a live trace.
// It must not block for long — it runs inline. A nil ProgressFunc is valid.
type ProgressFunc func(phase, detail string)

func (p ProgressFunc) report(phase, detail string) {
	if p != nil {
		p(phase, detail)
	}
}

// Research phases, reported through ProgressFunc.
const (
	// PhaseDiscovering is the topic-discovery sweep, which runs before any
	// research when the caller has not supplied a subject.
	PhaseDiscovering = "researching_discover"
	PhasePlanning    = "researching_plan"
	PhaseSearching   = "researching_search"
	PhaseReading     = "researching_read"
	PhaseVerifying   = "researching_verify"
	PhaseSynthesised = "researching_done"
)

// Research plans, searches, reads and verifies, returning a Brief.
//
// The loop is agentic in the sense that matters — the model decides what to
// search for and what each source establishes — but the sequence itself is
// fixed rather than left to the model to choose. Research always happens, and
// verification always happens, because a model that is having a bad day cannot
// skip a step that is not offered to it as a choice.
func (a *Agent) Research(ctx context.Context, req Request, progress ProgressFunc) (*Brief, error) {
	topic := strings.TrimSpace(req.Topic)
	if topic == "" {
		return nil, fmt.Errorf("research: topic is required")
	}
	// Re-label only the engine for Langfuse: when research runs inside a blog
	// pipeline this keeps its calls grouped under the blog run's session while
	// still counting as research in the by-engine widget.
	ctx = llmtrace.WithTraceName(ctx, "go-research-run")
	if a.llm == nil {
		return nil, fmt.Errorf("research: llm client is nil")
	}
	if a.sources == nil {
		return nil, fmt.Errorf("research: source client is nil")
	}

	brief := &Brief{Topic: topic}

	if len(req.URLs) > 0 {
		return a.researchPinned(ctx, req, brief, progress)
	}

	// 1. Plan — turn the topic and the caller's guidance into search queries.
	progress.report(PhasePlanning, fmt.Sprintf("Planning research for %q", topic))
	queries, err := a.plan(ctx, req)
	if err != nil {
		return nil, err
	}
	brief.Queries = queries

	// 2. Search — gather candidates across every backend.
	progress.report(PhaseSearching, fmt.Sprintf("Searching %d queries", len(queries)))
	searched := a.gather(ctx, queries, brief)
	if len(searched) == 0 {
		return nil, fmt.Errorf("research: no sources found for %q", topic)
	}

	// 3. Select — decide which candidates are actually about this topic.
	//
	// Search relevance is not topic relevance. Queries get relaxed when a
	// strict match finds nothing, and technical vocabulary is ambiguous — a
	// search for the Kubernetes Gateway API returns API-gateway vendors, which
	// share every keyword and none of the subject. Reading whichever URL ranked
	// highest is how an article ends up citing a real, well-written page about
	// something else entirely.
	candidates := a.selectSources(ctx, req, searched, brief)
	if len(candidates) == 0 {
		// An empty selection is often the model being too strict (or wrong) on a
		// niche topic, not proof that nothing exists — live runs of
		// "ai agents for tax return" failed here after a broader research call
		// on the same subject had already found sources. Broaden once, then
		// fall back to search order and let extraction/verification drop
		// anything that is not actually about the topic.
		candidates = a.recoverEmptySelection(ctx, req, searched, brief, progress)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("research: no sources found that are actually about %q", topic)
	}

	// 4. Read — retrieve the most promising candidates.
	//
	// Retrieval failure is routine (dead blogs, paywalls, HN item pages Jina
	// cannot render). tried tracks every URL already attempted so recovery can
	// walk the wider search pool without refetching the same dead links.
	tried := make(map[string]struct{})
	progress.report(PhaseReading, fmt.Sprintf("Reading %d of %d sources", min(len(candidates), a.cfg.MaxSources), len(candidates)))
	docs := a.read(ctx, candidates, brief, a.cfg.MaxSources, tried)
	if len(docs) == 0 {
		docs = a.recoverEmptyRead(ctx, req, searched, brief, progress, tried)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("research: none of the %d candidate sources could be retrieved", len(candidates))
	}

	// 5. Extract — pull claims out of each document, each with its supporting
	// passage.
	findings := a.extractAll(ctx, req, docs, brief)
	if len(findings) == 0 {
		return nil, fmt.Errorf("research: read %d sources but extracted no claims about %q", len(docs), topic)
	}

	// 6. Verify — drop anything the source does not actually support.
	progress.report(PhaseVerifying, fmt.Sprintf("Verifying %d claims against their sources", len(findings)))
	verified := a.verify(ctx, req, findings, docs, brief)

	// A thin first pass is common on niche topics: one readable page yields a
	// handful of claims, the mechanical quote check drops half of them, and
	// stopping there throws away a usable brief. Read unread candidates and
	// try again before deciding the research failed.
	if len(verified) < a.cfg.MinFindings {
		pool := dedupeSources(append(append([]Source{}, candidates...), searched...))
		verified, docs, findings = a.recoverFindings(ctx, req, pool, docs, findings, verified, brief, progress, tried)
	}

	if len(verified) == 0 {
		return nil, fmt.Errorf(
			"research: none of the %d claims about %q could be verified against their sources",
			len(findings), topic)
	}
	// MinFindings is a quality target, not a hard cliff. One or two verified
	// citations are enough to write from (see Brief.IsUsable); failing the
	// whole article over a missing third produces the recurring
	// "only 2 of 4 claims" error while discarding grounded material.
	if len(verified) < a.cfg.MinFindings {
		msg := fmt.Sprintf(
			"only %d of %d claims about %q could be verified (wanted %d); proceeding with what verified",
			len(verified), len(findings), topic, a.cfg.MinFindings)
		brief.Warnings = append(brief.Warnings, msg)
		a.logger.Warn("research: thin brief accepted",
			zap.Int("verified", len(verified)),
			zap.Int("wanted", a.cfg.MinFindings),
			zap.String("topic", topic))
	}
	brief.Findings = verified
	brief.Sources = citedSources(verified, docs)

	// 7. Synthesise — a short orientation to the subject.
	summary, err := a.synthesise(ctx, req, verified)
	if err != nil {
		// A missing summary is a degraded brief, not a failed one: the findings
		// are the substance and they are already verified.
		brief.Warnings = append(brief.Warnings, fmt.Sprintf("summary generation failed: %v", err))
		a.logger.Warn("research: synthesis failed", zap.Error(err))
	}
	brief.Summary = summary

	progress.report(PhaseSynthesised, fmt.Sprintf("%d verified findings from %d sources",
		len(brief.Findings), len(brief.Sources)))

	return brief, nil
}

// researchPinned Fetch's the caller's URLs only: no planner, no search, no
// recovery from HN/arxiv. A failed URL is a warning; if none load, the brief
// is empty so the caller can draft without invented facts.
func (a *Agent) researchPinned(ctx context.Context, req Request, brief *Brief, progress ProgressFunc) (*Brief, error) {
	candidates := make([]Source, 0, len(req.URLs))
	seen := make(map[string]struct{}, len(req.URLs))
	for _, raw := range req.URLs {
		u, err := validateURL(raw)
		if err != nil {
			brief.Warnings = append(brief.Warnings, fmt.Sprintf("skipped pinned url %s: %v", strings.TrimSpace(raw), err))
			continue
		}
		key := canonicalURL(u)
		if key == "" {
			key = u
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, Source{URL: u, Site: siteOf(u)})
		if len(candidates) >= 20 {
			break
		}
	}

	if len(candidates) == 0 {
		brief.Warnings = append(brief.Warnings, "none of the pinned knowledge pages could be retrieved")
		return brief, nil
	}

	tried := make(map[string]struct{})
	progress.report(PhaseReading, fmt.Sprintf("Reading %d pinned knowledge page(s)", len(candidates)))
	docs := a.read(ctx, candidates, brief, len(candidates), tried)
	if len(docs) == 0 {
		brief.Warnings = append(brief.Warnings, "none of the pinned knowledge pages could be retrieved")
		return brief, nil
	}

	findings := a.extractAll(ctx, req, docs, brief)
	if len(findings) == 0 {
		progress.report(PhaseSynthesised, "no claims extracted from pinned pages")
		return brief, nil
	}

	progress.report(PhaseVerifying, fmt.Sprintf("Verifying %d claims against their sources", len(findings)))
	verified := a.verify(ctx, req, findings, docs, brief)
	brief.Findings = verified
	brief.Sources = citedSources(verified, docs)

	if len(verified) > 0 {
		summary, err := a.synthesise(ctx, req, verified)
		if err != nil {
			brief.Warnings = append(brief.Warnings, fmt.Sprintf("summary generation failed: %v", err))
			a.logger.Warn("research: synthesis failed", zap.Error(err))
		} else {
			brief.Summary = summary
		}
	}

	progress.report(PhaseSynthesised, fmt.Sprintf("%d verified findings from %d pinned page(s)",
		len(brief.Findings), len(brief.Sources)))
	return brief, nil
}

// plan asks the model what to search for.
func (a *Agent) plan(ctx context.Context, req Request) ([]string, error) {
	guidance := strings.TrimSpace(req.Context)
	if guidance == "" {
		guidance = "(none given)"
	}

	prompt := fmt.Sprintf(`You are planning research for a technical article.

TOPIC:
%s

ADDITIONAL CONTEXT FROM THE REQUESTER (angle, audience, points to cover, things to avoid):
%s

Produce up to %d web search queries that would find current, factual, citable
material on this topic.

Write them as SEARCH KEYWORDS, not as questions or sentences. Two to four words
each, using the terms practitioners actually use. A long query matches nothing.

Good:  "gateway api ga", "ingress2gateway migration", "envoy gateway benchmark"
Bad:   "Kubernetes Gateway API ingress replacement production implementation"

Respond with JSON only, in exactly this shape:
{"queries": ["first query", "second query"]}`,
		req.Topic, guidance, a.cfg.MaxQueries)

	resp, err := a.generate(ctx, req.Model, prompt)
	if err != nil {
		return nil, fmt.Errorf("research: plan: %w", err)
	}

	var parsed struct {
		Queries []string `json:"queries"`
	}
	// No retry here: the topic itself is a serviceable query, so a planner that
	// returns unparseable output degrades the research rather than ending it,
	// and that fallback is cheaper than a second generation.
	if err := llm.DecodeJSON(resp, &parsed); err != nil {
		a.logger.Warn("research: could not parse plan, falling back to the topic",
			zap.Error(err), zap.String("response", truncate(resp, 200)))
		return []string{req.Topic}, nil
	}

	out := make([]string, 0, len(parsed.Queries))
	for _, q := range parsed.Queries {
		if s := strings.TrimSpace(q); s != "" {
			out = append(out, s)
		}
		if len(out) >= a.cfg.MaxQueries {
			break
		}
	}
	if len(out) == 0 {
		return []string{req.Topic}, nil
	}
	return out, nil
}

// gather runs every planned query and merges the candidates.
//
// If the whole plan comes back empty the topic itself is searched as a last
// resort. A planner can write queries that are individually reasonable and
// collectively too narrow, and abandoning the research at that point wastes a
// perfectly answerable request over a bad guess at phrasing.
func (a *Agent) gather(ctx context.Context, queries []string, brief *Brief) []Source {
	all := a.runQueries(ctx, queries, brief)
	if len(all) > 0 {
		return dedupeSources(all)
	}

	if len(queries) == 1 && queries[0] == brief.Topic {
		return nil // already tried the topic; nothing more to fall back to
	}
	a.logger.Info("research: planned queries found nothing, retrying with the topic",
		zap.String("topic", brief.Topic), zap.Strings("queries", queries))
	brief.Warnings = append(brief.Warnings,
		"the planned search queries returned nothing; fell back to searching the topic directly")

	return dedupeSources(a.runQueries(ctx, []string{brief.Topic}, brief))
}

// runQueries searches each query, tolerating individual failures.
func (a *Agent) runQueries(ctx context.Context, queries []string, brief *Brief) []Source {
	var all []Source
	for _, q := range queries {
		found, err := a.sources.Search(ctx, q, DefaultLimit)
		if err != nil {
			brief.Warnings = append(brief.Warnings, fmt.Sprintf("search %q failed: %v", q, err))
			a.logger.Warn("research: search failed", zap.String("query", q), zap.Error(err))
			continue
		}
		all = append(all, found...)
	}
	return all
}

// read retrieves candidates until limit documents have been read successfully.
//
// Retrieval failure is the normal case, not the exception: search results
// routinely include long-dead blogs and sites that time out. So this walks the
// candidate list in batches and keeps going until it has enough documents or
// runs out of candidates, rather than fetching exactly limit URLs and
// accepting whatever fraction succeeds.
//
// tried records every URL already attempted (success or failure) so a later
// recovery pass can skip them. It must not be nil.
func (a *Agent) read(ctx context.Context, candidates []Source, brief *Brief, limit int, tried map[string]struct{}) []Document {
	if limit <= 0 {
		limit = a.cfg.MaxSources
	}
	if tried == nil {
		tried = make(map[string]struct{})
	}
	candidates = skipTried(candidates, tried)

	var (
		mu   sync.Mutex
		docs []Document
	)

	for i := 0; i < len(candidates) && len(docs) < limit; {
		// Take the next batch, sized to what is still missing.
		need := limit - len(docs)
		batch := candidates[i:min(i+max(need, a.cfg.FetchConcurrency), len(candidates))]
		i += len(batch)

		sem := make(chan struct{}, a.cfg.FetchConcurrency)
		var wg sync.WaitGroup

		for _, src := range batch {
			tried[src.URL] = struct{}{}
			wg.Add(1)
			go func(src Source) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				doc, err := a.sources.Fetch(ctx, src.URL)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					brief.Warnings = append(brief.Warnings,
						fmt.Sprintf("could not read %s: %v", src.URL, err))
					return
				}
				// Carry the search-time publication date across: Reader does not
				// report one, and recency is part of judging a source.
				if doc.PublishedAt == nil && src.PublishedAt != nil {
					doc.PublishedAt = src.PublishedAt
				}
				docs = append(docs, *doc)
			}(src)
		}
		wg.Wait()
	}

	if len(docs) > limit {
		docs = docs[:limit]
	}
	return docs
}

// recoverEmptyRead runs when every selected candidate failed to fetch.
//
// Selection often narrows to a single URL that then 404s or times out — the
// live "none of the 1 candidate sources could be retrieved" failure — while
// the wider search pool still has readable pages. Try those next, then a
// broadened search, before giving up.
func (a *Agent) recoverEmptyRead(
	ctx context.Context, req Request, searched []Source, brief *Brief, progress ProgressFunc, tried map[string]struct{},
) []Document {
	progress.report(PhaseReading, "Selected sources were unreadable; trying other search results")

	pool := skipTried(searched, tried)
	if len(pool) == 0 {
		broader := dedupeSources(a.runQueries(ctx, broadenQueries(req.Topic, brief.Queries), brief))
		pool = skipTried(broader, tried)
	}
	if len(pool) == 0 {
		return nil
	}

	brief.Warnings = append(brief.Warnings,
		fmt.Sprintf("could not retrieve selected sources; reading %d other search result(s)", min(len(pool), a.cfg.MaxSources)))
	a.logger.Warn("research: selected sources unreadable, trying search pool",
		zap.String("topic", req.Topic), zap.Int("remaining", len(pool)))
	return a.read(ctx, pool, brief, a.cfg.MaxSources, tried)
}

// recoverExtraSources is how many additional pages a thin first pass may open
// before accepting fewer than MinFindings. Enough to usually clear the floor
// without turning a niche topic into an unbounded crawl.
const recoverExtraSources = 4

// recoverFindings reads unread candidates and merges any newly verified claims
// when the first pass came up short of MinFindings.
func (a *Agent) recoverFindings(
	ctx context.Context,
	req Request,
	candidates []Source,
	docs []Document,
	findings, verified []Finding,
	brief *Brief,
	progress ProgressFunc,
	tried map[string]struct{},
) ([]Finding, []Document, []Finding) {
	remaining := skipTried(candidates, tried)
	if len(remaining) == 0 {
		return verified, docs, findings
	}

	progress.report(PhaseReading, fmt.Sprintf(
		"Only %d verified finding(s); reading %d more source(s)",
		len(verified), min(len(remaining), recoverExtraSources)))
	extra := a.read(ctx, remaining, brief, recoverExtraSources, tried)
	if len(extra) == 0 {
		return verified, docs, findings
	}

	docs = append(docs, extra...)
	moreFindings := a.extractAll(ctx, req, extra, brief)
	if len(moreFindings) == 0 {
		return verified, docs, findings
	}
	findings = append(findings, moreFindings...)

	progress.report(PhaseVerifying, fmt.Sprintf(
		"Verifying %d additional claim(s) against their sources", len(moreFindings)))
	moreVerified := a.verify(ctx, req, moreFindings, extra, brief)
	verified = append(verified, moreVerified...)
	return verified, docs, findings
}

// skipTried returns candidates whose URLs are not in tried.
func skipTried(candidates []Source, tried map[string]struct{}) []Source {
	if len(tried) == 0 {
		return candidates
	}
	out := make([]Source, 0, len(candidates))
	for _, s := range candidates {
		if _, ok := tried[s.URL]; ok {
			continue
		}
		out = append(out, s)
	}
	return out
}

// unreadSources returns candidates whose URLs are not already in docs.
func unreadSources(candidates []Source, docs []Document) []Source {
	seen := make(map[string]struct{}, len(docs))
	for _, d := range docs {
		seen[d.URL] = struct{}{}
	}
	return skipTried(candidates, seen)
}

// selectCandidates is how many search hits are put in front of the model for
// the relevance pass. Titles and excerpts are cheap, so this can be well above
// MaxSources — the point is to give the selector enough to choose from.
const selectCandidates = 20

// selectSources filters candidates down to those genuinely about the topic.
//
// This is a judgement about relevance, so it is the model's to make, but it is
// made from titles and excerpts rather than full documents — which is what
// makes it affordable enough to run before committing to any retrieval. It
// costs one call and saves reading pages that were never going to be citable.
//
// On any failure the original ordering is returned unchanged: a broken selector
// should degrade the research to what it was before this step existed, not
// block it.
func (a *Agent) selectSources(ctx context.Context, req Request, candidates []Source, brief *Brief) []Source {
	if len(candidates) <= 1 {
		return candidates
	}
	shortlist := candidates[:min(len(candidates), selectCandidates)]

	var b strings.Builder
	for i, s := range shortlist {
		published := "unknown date"
		if s.PublishedAt != nil {
			published = s.PublishedAt.Format("2006-01-02")
		}
		fmt.Fprintf(&b, "\n[%d] %s\n    site: %s | published: %s\n    %s\n",
			i, s.Title, s.Site, published, truncate(s.Excerpt, 200))
	}

	prompt := fmt.Sprintf(`You are choosing which search results to read for an article.

ARTICLE TOPIC:
%s

CANDIDATE SOURCES:
%s

Select the sources that are genuinely about the article topic, best first.

Be careful about the subject. Technical terms are ambiguous and search engines
match on keywords, so results often share vocabulary with the topic while being
about something else — reject those. Prefer primary sources (project
documentation, release notes, engineering blogs, papers) and recent material.

Select at most %d. Prefer a plausible, related source over an empty list: return
[] only when every result is clearly about a different subject.

Respond with JSON only, in exactly this shape:
{"selected": [0, 3, 7]}`,
		req.Topic, b.String(), a.cfg.MaxSources*2)

	// Worth a retry: falling back to search order means reading sources that
	// are not about the topic, which is the failure this step exists to stop.
	var parsed struct {
		Selected []int `json:"selected"`
	}
	if err := a.generateJSON(ctx, req.Model, "source selection", prompt, maxResearchTokens, &parsed); err != nil {
		a.logger.Warn("research: could not select sources, reading in search order", zap.Error(err))
		return candidates
	}

	out := make([]Source, 0, len(parsed.Selected))
	seen := make(map[int]struct{}, len(parsed.Selected))
	for _, idx := range parsed.Selected {
		if idx < 0 || idx >= len(shortlist) {
			continue
		}
		if _, dup := seen[idx]; dup {
			continue
		}
		seen[idx] = struct{}{}
		out = append(out, shortlist[idx])
	}

	if dropped := len(shortlist) - len(out); dropped > 0 && len(out) > 0 {
		brief.Warnings = append(brief.Warnings,
			fmt.Sprintf("set aside %d search result(s) as not being about the topic", dropped))
	}
	return out
}

// recoverEmptySelection runs when the relevance pass rejected every hit.
//
// First it searches broader phrasings of the topic and selects again. If that
// is still empty, it returns the top search hits so extraction can filter —
// the same degradation used when the selector itself fails to parse. Hard-
// failing here was the recurring "no sources found that are actually about"
// error on niche topics the same agent had already researched successfully.
func (a *Agent) recoverEmptySelection(
	ctx context.Context, req Request, searched []Source, brief *Brief, progress ProgressFunc,
) []Source {
	progress.report(PhaseSearching, fmt.Sprintf(
		"Selection found nothing on-topic for %q; broadening search", req.Topic))

	broaderQueries := broadenQueries(req.Topic, brief.Queries)
	broader := dedupeSources(a.runQueries(ctx, broaderQueries, brief))
	pool := dedupeSources(append(append([]Source{}, searched...), broader...))
	if len(pool) == 0 {
		return nil
	}

	selected := a.selectSources(ctx, req, pool, brief)
	if len(selected) > 0 {
		brief.Warnings = append(brief.Warnings,
			"initial source selection was empty; recovered after a broader search")
		return selected
	}

	limit := min(len(pool), a.cfg.MaxSources*2)
	brief.Warnings = append(brief.Warnings,
		fmt.Sprintf("source selection found nothing on-topic; reading top %d search result(s) and filtering at extraction", limit))
	a.logger.Warn("research: empty selection, falling back to search order",
		zap.String("topic", req.Topic), zap.Int("candidates", limit))
	return pool[:limit]
}

// broadenQueries builds follow-up searches when the first pass selected nothing.
// The topic itself is always included; planned queries that were overly narrow
// are replaced with shorter keyword forms of the topic.
func broadenQueries(topic string, planned []string) []string {
	out := []string{topic}
	words := strings.Fields(strings.ToLower(topic))
	if len(words) > 2 {
		// Drop filler words that inflate queries into zero-hit phrases.
		var keep []string
		for _, w := range words {
			switch w {
			case "a", "an", "the", "for", "of", "and", "or", "to", "in", "on":
				continue
			}
			keep = append(keep, w)
		}
		if len(keep) >= 2 {
			out = append(out, strings.Join(keep, " "))
		}
		if len(keep) >= 3 {
			out = append(out, strings.Join(keep[:3], " "))
		}
	}
	for _, q := range planned {
		if s := strings.TrimSpace(q); s != "" && !strings.EqualFold(s, topic) {
			out = append(out, s)
		}
	}
	// Cap so a bad planner cannot explode the recovery into dozens of searches.
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// docExcerptChars bounds how much of a document is shown to the model during
// extraction. Enough to cover an article's substance without spending the
// context window on one long page. Which slice of the document that is, is
// decided by proseExcerpt rather than by taking the head — see excerpt.go.
const docExcerptChars = 6000

// extractAll pulls claims from every document.
func (a *Agent) extractAll(ctx context.Context, req Request, docs []Document, brief *Brief) []Finding {
	var out []Finding
	for i := range docs {
		found, err := a.extract(ctx, req, docs[i])
		if err != nil {
			brief.Warnings = append(brief.Warnings,
				fmt.Sprintf("could not extract claims from %s: %v", docs[i].URL, err))
			a.logger.Warn("research: extraction failed",
				zap.String("url", docs[i].URL), zap.Error(err))
			continue
		}
		out = append(out, found...)
	}
	return out
}

// extract asks the model what one document establishes.
//
// The model is required to return a verbatim quote alongside each claim. That
// is not for the reader's benefit — it is what makes the next step possible.
// A claim on its own can only be checked by asking another model whether it
// looks right; a claim plus a quote can be checked against the actual bytes of
// the page.
func (a *Agent) extract(ctx context.Context, req Request, doc Document) ([]Finding, error) {
	var prompt string
	if len(req.URLs) > 0 {
		prompt = extractPinnedPrompt(req, doc)
	} else {
		prompt = fmt.Sprintf(`You are extracting citable facts from a source document for an article.

ARTICLE TOPIC:
%s

SOURCE URL: %s
SOURCE TITLE: %s

SOURCE TEXT:
%s

Extract up to 4 specific, factual claims this document supports and that are
relevant to the article topic. For each claim, include the VERBATIM passage from
the source text above that supports it — copied exactly, not paraphrased, and
between 10 and 60 words.

Rules:
- Only extract claims the source text actually states. Do not add knowledge of
  your own.
- If the document is not relevant to the topic, return an empty list.
- The quote must appear word-for-word in the SOURCE TEXT above.

Respond with JSON only, in exactly this shape:
{"findings": [{"claim": "...", "quote": "..."}]}`,
			req.Topic, doc.URL, doc.Title, proseExcerpt(doc.Text, docExcerptChars))
	}

	var parsed struct {
		Findings []struct {
			Claim string `json:"claim"`
			Quote string `json:"quote"`
		} `json:"findings"`
	}
	if err := a.generateJSON(ctx, req.Model, "extraction", prompt, maxResearchTokens, &parsed); err != nil {
		return nil, err
	}

	out := make([]Finding, 0, len(parsed.Findings))
	for _, f := range parsed.Findings {
		claim := strings.TrimSpace(f.Claim)
		quote := strings.TrimSpace(f.Quote)
		if claim == "" || quote == "" {
			continue
		}
		out = append(out, Finding{Claim: claim, SourceURL: doc.URL, Quote: quote})
	}
	return out, nil
}

func extractPinnedPrompt(req Request, doc Document) string {
	lookFor := pinnedLookFor(req)
	subject, body := pinnedInbound(req)
	if len(body) > 4000 {
		body = body[:4000] + "\n…"
	}
	return fmt.Sprintf(`You extract facts from one knowledge page to answer an inbound email.
Use only DOCUMENT TEXT. If the page does not contain what we are looking for, return {"findings":[]}. Do not guess.

LOOK FOR:
%s

INBOUND SUBJECT:
%s

INBOUND EMAIL (trimmed):
%s

DOCUMENT URL:
%s

DOCUMENT TEXT:
%s

Return JSON only:
{"findings":[{"claim":"...","quote":"..."}]}
Each claim is one factual sentence. Each quote is verbatim from DOCUMENT TEXT.`,
		lookFor, subject, body, doc.URL, proseExcerpt(doc.Text, docExcerptChars))
}

func pinnedLookFor(req Request) string {
	const prefix = "Look for: "
	ctx := req.Context
	if i := strings.Index(ctx, prefix); i >= 0 {
		rest := ctx[i+len(prefix):]
		if j := strings.Index(rest, "\n"); j >= 0 {
			rest = rest[:j]
		}
		if s := strings.TrimSpace(rest); s != "" {
			return s
		}
	}
	return "(whatever the email is asking)"
}

func pinnedInbound(req Request) (subject, body string) {
	const prefix = "Inbound email:\n"
	ctx := req.Context
	i := strings.Index(ctx, prefix)
	if i < 0 {
		return "", strings.TrimSpace(ctx)
	}
	inbound := ctx[i+len(prefix):]
	if j := strings.Index(inbound, "\n"); j >= 0 {
		return inbound[:j], strings.TrimSpace(inbound[j+1:])
	}
	return inbound, ""
}

// verify drops every finding the source does not actually support.
//
// Three checks, in increasing cost and decreasing certainty:
//
//  1. The source was retrieved. Guaranteed by construction — a Finding's
//     SourceURL comes from a Document, and a Document only exists after a
//     successful fetch — which is what rules out invented URLs.
//  2. The quote appears in the retrieved text. A deterministic comparison
//     against bytes we hold, which rules out invented quotes.
//  3. The quote supports the claim. This one is a judgement, so it is the only
//     one delegated to the model, and it is asked adversarially.
//
// Putting the two mechanical checks first means the model is never asked to
// adjudicate a citation that is already provably fabricated.
func (a *Agent) verify(ctx context.Context, req Request, findings []Finding, docs []Document, brief *Brief) []Finding {
	byURL := make(map[string]*Document, len(docs))
	for i := range docs {
		byURL[docs[i].URL] = &docs[i]
	}

	grounded := make([]Finding, 0, len(findings))
	for _, f := range findings {
		doc, ok := byURL[f.SourceURL]
		if !ok {
			brief.Warnings = append(brief.Warnings,
				fmt.Sprintf("dropped a claim citing %s, which was never retrieved", f.SourceURL))
			continue
		}
		if !quoteSupported(doc.Text, f.Quote) {
			brief.Warnings = append(brief.Warnings,
				fmt.Sprintf("dropped a claim whose quote does not appear in %s", f.SourceURL))
			a.logger.Info("research: rejected a fabricated quote",
				zap.String("url", f.SourceURL), zap.String("quote", truncate(f.Quote, 120)))
			continue
		}
		grounded = append(grounded, f)
	}

	if len(grounded) == 0 {
		return nil
	}

	relevant, err := a.judgeRelevance(ctx, req, grounded)
	if err != nil {
		// The mechanical checks have already passed at this point: the source
		// was read and the quote is really in it. Keeping those findings when
		// the judge is unavailable is the honest degradation — the alternative
		// throws away provably grounded material because one LLM call failed.
		brief.Warnings = append(brief.Warnings,
			fmt.Sprintf("relevance check unavailable, kept %d quote-verified claims unjudged: %v", len(grounded), err))
		a.logger.Warn("research: relevance judgement failed", zap.Error(err))
		return grounded
	}
	if dropped := len(grounded) - len(relevant); dropped > 0 {
		brief.Warnings = append(brief.Warnings,
			fmt.Sprintf("dropped %d claim(s) whose quote did not support them", dropped))
	}
	return relevant
}

// judgeRelevance asks whether each quote actually supports its claim.
//
// Framed adversarially and in one batch: the model is told to reject by default
// on doubt, because the failure that matters here is keeping a bad citation,
// not losing a good one.
func (a *Agent) judgeRelevance(ctx context.Context, req Request, findings []Finding) ([]Finding, error) {
	var b strings.Builder
	for i, f := range findings {
		fmt.Fprintf(&b, "\n[%d]\nCLAIM: %s\nQUOTE FROM SOURCE: %s\n", i, f.Claim, f.Quote)
	}

	prompt := fmt.Sprintf(`You are fact-checking citations for an article. For each numbered pair below,
decide whether the QUOTE genuinely supports the CLAIM.

Reject the pair if:
- The quote is about a different subject than the claim.
- The claim asserts more than the quote establishes (e.g. the quote describes one
  case and the claim generalises it).
- The claim reverses, exaggerates or sharpens what the quote says.
- You are unsure. Default to rejecting.

Accept only when the quote plainly establishes the claim.
%s

Respond with JSON only, in exactly this shape, including every index above:
{"verdicts": [{"index": 0, "supported": true}, {"index": 1, "supported": false}]}`, b.String())

	var parsed struct {
		Verdicts []struct {
			Index     int  `json:"index"`
			Supported bool `json:"supported"`
		} `json:"verdicts"`
	}
	if err := a.generateJSON(ctx, req.Model, "relevance judgement", prompt, maxResearchTokens, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Verdicts) == 0 {
		return nil, fmt.Errorf("judge returned no verdicts")
	}

	// A finding with no verdict is kept. It has already passed both mechanical
	// checks, and a judge that silently omits an index should not be able to
	// delete evidence by saying nothing about it.
	rejected := make(map[int]bool, len(parsed.Verdicts))
	for _, v := range parsed.Verdicts {
		if !v.Supported {
			rejected[v.Index] = true
		}
	}

	out := make([]Finding, 0, len(findings))
	for i, f := range findings {
		if !rejected[i] {
			out = append(out, f)
		}
	}
	return out, nil
}

// synthesise writes a short orientation to the subject from the verified
// findings alone.
func (a *Agent) synthesise(ctx context.Context, req Request, findings []Finding) (string, error) {
	var b strings.Builder
	for _, f := range findings {
		fmt.Fprintf(&b, "- %s (source: %s)\n", f.Claim, f.SourceURL)
	}

	prompt := fmt.Sprintf(`Summarise the current state of this topic in 3-5 sentences, for a writer who is
about to draft an article on it.

TOPIC: %s

VERIFIED FINDINGS:
%s

Use only the findings above. Do not introduce facts that are not present in them.
Respond with the summary text only — no preamble, no JSON, no bullet points.`,
		req.Topic, b.String())

	resp, err := a.generateBounded(ctx, req.Model, prompt, maxSummaryTokens)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp), nil
}

// generate is the single point where this agent talks to the LLM.
// Generation ceilings, in tokens.
//
// These exist to bound a runaway, not to shape the output — every value has
// generous headroom over what the prompt actually asks for, so a normal
// response is never truncated.
//
// A model that fails to emit a stop token generates until something stops it,
// and with nothing set there is nothing to. That is not hypothetical: a runner
// on the shared Ollama host was found mid-generation four days after a
// 2,376-token prompt, having produced a quarter of a million tokens and still
// climbing, holding that model's only slot the entire time. Every request for
// it queued behind a task that was never going to finish.
const (
	// maxResearchTokens covers planning, source selection, extraction and
	// judging — all short, structured JSON replies.
	maxResearchTokens = 2000
	// maxSummaryTokens covers the synthesis, which is a few sentences.
	maxSummaryTokens = 800
)

func (a *Agent) generate(ctx context.Context, model, prompt string) (string, error) {
	return a.generateBounded(ctx, model, prompt, maxResearchTokens)
}

// generateJSON asks for a JSON reply and decodes it into v, repairing what it
// can and asking again once when it cannot. See llm.GenerateJSON.
func (a *Agent) generateJSON(
	ctx context.Context, model, stage, prompt string, maxTokens int, v any,
) error {
	return llm.GenerateJSON(ctx, stage, prompt, v,
		func(ctx context.Context, p string) (string, error) {
			return a.generateBounded(ctx, model, p, maxTokens)
		},
		func(reply string, err error) {
			a.logger.Warn("research: could not parse the model's JSON, asking again",
				zap.String("stage", stage), zap.Error(err),
				zap.String("response", truncate(reply, 200)))
		},
	)
}

func (a *Agent) generateBounded(ctx context.Context, model, prompt string, maxTokens int) (string, error) {
	resp, err := a.llm.Generate(ctx, llm.GenerateRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return "", fmt.Errorf("empty response from %s", a.llm.ProviderName())
	}
	return resp.Content, nil
}

// citedSources returns the documents that survived verification, in the order
// they are first cited, as Sources.
func citedSources(findings []Finding, docs []Document) []Source {
	byURL := make(map[string]*Document, len(docs))
	for i := range docs {
		byURL[docs[i].URL] = &docs[i]
	}

	seen := make(map[string]struct{}, len(findings))
	out := make([]Source, 0, len(findings))
	for _, f := range findings {
		if _, dup := seen[f.SourceURL]; dup {
			continue
		}
		doc, ok := byURL[f.SourceURL]
		if !ok {
			continue
		}
		seen[f.SourceURL] = struct{}{}
		// PublishedAt stays whatever the search backend reported, including
		// nil. Substituting the fetch time would date the source to when we
		// happened to read it, which is not the same claim at all.
		out = append(out, doc.Source)
	}
	return out
}
