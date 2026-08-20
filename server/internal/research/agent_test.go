package research

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
)

// scriptedLLM answers each prompt with the first canned response whose trigger
// appears in the prompt. Keying on content rather than call order keeps the
// tests readable and stops them breaking when a phase is reordered.
type scriptedLLM struct {
	responses []scriptedResponse
	// prompts records everything the agent asked, for assertions about what
	// the model was actually shown.
	prompts []string
	// failOn makes any prompt containing this substring return an error.
	failOn string
}

type scriptedResponse struct {
	trigger string
	content string
}

func (s *scriptedLLM) ProviderName() string { return "scripted" }

func (s *scriptedLLM) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	prompt := req.Messages[len(req.Messages)-1].Content
	s.prompts = append(s.prompts, prompt)

	if s.failOn != "" && strings.Contains(prompt, s.failOn) {
		return nil, fmt.Errorf("scripted failure")
	}
	for _, r := range s.responses {
		if strings.Contains(prompt, r.trigger) {
			return &llm.GenerateResponse{Content: r.content}, nil
		}
	}
	return nil, fmt.Errorf("scriptedLLM: no canned response matched prompt: %s", truncate(prompt, 120))
}

// The document the fake fetcher serves. Quotes in the tests are taken from — or
// deliberately not taken from — this text.
const testDocText = `Kubernetes 1.31 promoted the Gateway API to general availability after an
extended beta period. The API separates operator and developer concerns into distinct
resources, which Ingress never did. Teams migrating large Ingress estates can use the
ingress2gateway conversion tool to generate equivalent Gateway resources automatically.`

// fixedBackend serves canned search results and documents.
type fixedBackend struct {
	sources   []Source
	docs      map[string]*Document
	fetchErrs map[string]error
}

func (f *fixedBackend) Name() string { return "fixed" }

func (f *fixedBackend) Search(context.Context, string, int) ([]Source, error) {
	return f.sources, nil
}

func (f *fixedBackend) Fetch(_ context.Context, rawURL string) (*Document, error) {
	if err, ok := f.fetchErrs[rawURL]; ok {
		return nil, err
	}
	if doc, ok := f.docs[rawURL]; ok {
		return doc, nil
	}
	return nil, fmt.Errorf("no such document: %s", rawURL)
}

// newTestAgent wires an Agent over the fake backend and scripted model.
func newTestAgent(t *testing.T, backend *fixedBackend, model *scriptedLLM) *Agent {
	t.Helper()
	client := NewWith(backend, []Searcher{backend}, nil, zap.NewNop())
	cfg := DefaultAgentConfig()
	cfg.MinFindings = 1
	return NewAgent(client, model, cfg, zap.NewNop())
}

// defaultBackend serves one good document.
func defaultBackend() *fixedBackend {
	return &fixedBackend{
		sources: []Source{{URL: "https://kubernetes.io/blog/gateway-ga", Title: "Gateway API is GA"}},
		docs: map[string]*Document{
			"https://kubernetes.io/blog/gateway-ga": {
				Source: Source{URL: "https://kubernetes.io/blog/gateway-ga", Title: "Gateway API is GA", Site: "kubernetes.io"},
				Text:   testDocText,
			},
		},
	}
}

func TestResearch_HappyPath(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["kubernetes gateway api ga", "ingress2gateway migration"]}`},
		{trigger: "extracting citable facts", content: `{"findings": [
			{"claim": "Gateway API reached GA in Kubernetes 1.31.",
			 "quote": "Kubernetes 1.31 promoted the Gateway API to general availability after an extended beta period."},
			{"claim": "A conversion tool exists for migrating from Ingress.",
			 "quote": "Teams migrating large Ingress estates can use the ingress2gateway conversion tool"}
		]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}, {"index": 1, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "Gateway API is now GA and tooling exists to migrate."},
	}}

	agent := newTestAgent(t, defaultBackend(), model)

	brief, err := agent.Research(context.Background(), Request{
		Topic:   "Kubernetes Gateway API",
		Context: "For platform engineers running large clusters.",
	}, nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}

	if !brief.IsUsable() {
		t.Fatal("brief reports itself unusable")
	}
	if len(brief.Findings) != 2 {
		t.Errorf("got %d findings, want 2", len(brief.Findings))
	}
	if len(brief.Sources) != 1 {
		t.Errorf("got %d sources, want 1 de-duplicated source", len(brief.Sources))
	}
	if len(brief.Queries) != 2 {
		t.Errorf("got %d queries recorded, want 2", len(brief.Queries))
	}
	if brief.Summary == "" {
		t.Error("summary is empty")
	}
	for _, f := range brief.Findings {
		if f.SourceURL == "" || f.Quote == "" {
			t.Errorf("finding is missing its citation: %+v", f)
		}
	}
}

// The caller's context is what distinguishes "write about X" from "write about
// X for this audience, hitting these points". It is useless if it never
// reaches the model.
func TestResearch_PassesCallerContextToThePlanner(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["q"]}`},
		{trigger: "extracting citable facts", content: `{"findings": [{"claim": "c", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability"}]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}

	agent := newTestAgent(t, defaultBackend(), model)

	_, err := agent.Research(context.Background(), Request{
		Topic:   "Gateway API",
		Context: "Avoid vendor comparisons; assume the reader knows Ingress.",
	}, nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}

	if !strings.Contains(model.prompts[0], "Avoid vendor comparisons") {
		t.Errorf("planner prompt did not carry the caller's context:\n%s", model.prompts[0])
	}
}

// The central guarantee: a quote the model invented is dropped even when the
// model then insists the citation is sound. The mechanical check runs first and
// the judge never gets to overrule it.
func TestResearch_DropsFabricatedQuotesEvenWhenTheJudgeApproves(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["q"]}`},
		{trigger: "extracting citable facts", content: `{"findings": [
			{"claim": "Real claim.",
			 "quote": "Kubernetes 1.31 promoted the Gateway API to general availability after an extended beta period."},
			{"claim": "Invented claim.",
			 "quote": "Gateway API will become mandatory for all clusters in Kubernetes 1.35 according to the maintainers."}
		]}`},
		// The judge approves both, including the fabricated one.
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}

	agent := newTestAgent(t, defaultBackend(), model)

	brief, err := agent.Research(context.Background(), Request{Topic: "Gateway API"}, nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}

	if len(brief.Findings) != 1 {
		t.Fatalf("got %d findings, want 1 — the fabricated quote should have been dropped", len(brief.Findings))
	}
	if brief.Findings[0].Claim != "Real claim." {
		t.Errorf("kept the wrong finding: %+v", brief.Findings[0])
	}
	if len(brief.Warnings) == 0 {
		t.Error("dropping a fabricated quote was not recorded as a warning")
	}
}

// A claim whose quote is real but does not support it is the judge's job.
func TestResearch_DropsClaimsTheJudgeRejects(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["q"]}`},
		{trigger: "extracting citable facts", content: `{"findings": [
			{"claim": "Gateway API reached GA.",
			 "quote": "Kubernetes 1.31 promoted the Gateway API to general availability after an extended beta period."},
			{"claim": "Ingress is deprecated and will be removed.",
			 "quote": "The API separates operator and developer concerns into distinct resources, which Ingress never did."}
		]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}, {"index": 1, "supported": false}]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}

	agent := newTestAgent(t, defaultBackend(), model)

	brief, err := agent.Research(context.Background(), Request{Topic: "Gateway API"}, nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}
	if len(brief.Findings) != 1 {
		t.Fatalf("got %d findings, want 1 after the judge rejected an overreaching claim", len(brief.Findings))
	}
	if !strings.Contains(brief.Findings[0].Claim, "GA") {
		t.Errorf("kept the wrong finding: %+v", brief.Findings[0])
	}
}

// Retrieval failure is routine — search results are full of dead blogs. One
// unreadable source must not end the research.
func TestResearch_ToleratesUnreadableSources(t *testing.T) {
	backend := &fixedBackend{
		sources: []Source{
			{URL: "https://dead.example/gone", Title: "Dead"},
			{URL: "https://kubernetes.io/blog/gateway-ga", Title: "Gateway API is GA"},
		},
		docs: map[string]*Document{
			"https://kubernetes.io/blog/gateway-ga": {
				Source: Source{URL: "https://kubernetes.io/blog/gateway-ga", Site: "kubernetes.io"},
				Text:   testDocText,
			},
		},
		fetchErrs: map[string]error{
			"https://dead.example/gone": fmt.Errorf("target returned HTTP 404"),
		},
	}

	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["q"]}`},
		{trigger: "choosing which search results", content: `{"selected": [0, 1]}`},
		{trigger: "extracting citable facts", content: `{"findings": [{"claim": "c", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability"}]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}

	agent := newTestAgent(t, backend, model)

	brief, err := agent.Research(context.Background(), Request{Topic: "Gateway API"}, nil)
	if err != nil {
		t.Fatalf("Research failed despite one readable source: %v", err)
	}
	if len(brief.Findings) == 0 {
		t.Error("no findings from the source that did load")
	}

	var noted bool
	for _, w := range brief.Warnings {
		if strings.Contains(w, "dead.example") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("the unreadable source was not recorded in warnings: %v", brief.Warnings)
	}
}

// A planner can write queries that are individually reasonable and jointly too
// narrow — long natural-language phrases are the common case, and they match
// nothing. The topic itself is the last resort rather than giving up.
func TestResearch_FallsBackToTheTopicWhenPlannedQueriesFindNothing(t *testing.T) {
	backend := &queryAwareBackend{
		// Only the bare topic returns anything; the planner's phrasings miss.
		hits: map[string][]Source{
			"Gateway API": {{URL: "https://kubernetes.io/blog/gateway-ga", Title: "Gateway API is GA"}},
		},
		docs: map[string]*Document{
			"https://kubernetes.io/blog/gateway-ga": {
				Source: Source{URL: "https://kubernetes.io/blog/gateway-ga", Site: "kubernetes.io"},
				Text:   testDocText,
			},
		},
	}

	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["kubernetes gateway api ingress replacement production implementation"]}`},
		{trigger: "extracting citable facts", content: `{"findings": [{"claim": "c", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability"}]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}

	client := NewWith(backend, []Searcher{backend}, nil, zap.NewNop())
	cfg := DefaultAgentConfig()
	cfg.MinFindings = 1
	agent := NewAgent(client, model, cfg, zap.NewNop())

	brief, err := agent.Research(context.Background(), Request{Topic: "Gateway API"}, nil)
	if err != nil {
		t.Fatalf("Research gave up instead of falling back to the topic: %v", err)
	}
	if len(brief.Findings) != 1 {
		t.Errorf("got %d findings, want 1 via the fallback", len(brief.Findings))
	}

	var noted bool
	for _, w := range brief.Warnings {
		if strings.Contains(w, "fell back to searching the topic") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("the fallback was not disclosed in warnings: %v", brief.Warnings)
	}
}

// queryAwareBackend returns results only for exact queries it knows, so a test
// can prove which query actually produced the sources.
type queryAwareBackend struct {
	hits map[string][]Source
	docs map[string]*Document
}

func (q *queryAwareBackend) Name() string { return "query-aware" }

func (q *queryAwareBackend) Search(_ context.Context, query string, _ int) ([]Source, error) {
	return q.hits[query], nil
}

func (q *queryAwareBackend) Fetch(_ context.Context, rawURL string) (*Document, error) {
	if doc, ok := q.docs[rawURL]; ok {
		return doc, nil
	}
	return nil, fmt.Errorf("no such document: %s", rawURL)
}

// A brief with nothing verified must be an error. Returning an empty brief
// would let the article pipeline write an uncited piece and call it a success.
func TestResearch_FailsWhenNothingVerifies(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["q"]}`},
		{trigger: "extracting citable facts", content: `{"findings": [
			{"claim": "Invented.", "quote": "This sentence does not appear anywhere in the source document at all."}
		]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}

	agent := newTestAgent(t, defaultBackend(), model)

	if _, err := agent.Research(context.Background(), Request{Topic: "Gateway API"}, nil); err == nil {
		t.Fatal("Research succeeded with no verifiable findings")
	}
}

// MinFindings is a quality target. Two verified claims used to abort the whole
// article ("only 2 of 4 … need 3") even though Brief.IsUsable only requires one.
func TestResearch_AcceptsThinBriefBelowMinFindings(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["q"]}`},
		{trigger: "extracting citable facts", content: `{"findings": [
			{"claim": "Real one.", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability after an extended beta period."},
			{"claim": "Real two.", "quote": "Teams migrating large Ingress estates can use the ingress2gateway conversion tool"},
			{"claim": "Invented.", "quote": "This sentence does not appear anywhere in the source document at all."},
			{"claim": "Also invented.", "quote": "Gateway API will become mandatory for all clusters in Kubernetes 1.35."}
		]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [
			{"index": 0, "supported": true},
			{"index": 1, "supported": true}
		]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}

	client := NewWith(defaultBackend(), []Searcher{defaultBackend()}, nil, zap.NewNop())
	cfg := DefaultAgentConfig()
	cfg.MinFindings = 3
	agent := NewAgent(client, model, cfg, zap.NewNop())

	brief, err := agent.Research(context.Background(), Request{Topic: "Gateway API"}, nil)
	if err != nil {
		t.Fatalf("Research failed a usable thin brief: %v", err)
	}
	if len(brief.Findings) != 2 {
		t.Fatalf("got %d findings, want the 2 that verified", len(brief.Findings))
	}
	var noted bool
	for _, w := range brief.Warnings {
		if strings.Contains(w, "proceeding with what verified") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("thin brief was not disclosed in warnings: %v", brief.Warnings)
	}
}

// A planner that returns unparseable output should degrade to searching the
// topic itself rather than failing the run.
func TestResearch_FallsBackWhenThePlannerIsUnparseable(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: "I'm sorry, I can't help with that."},
		{trigger: "extracting citable facts", content: `{"findings": [{"claim": "c", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability"}]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}

	agent := newTestAgent(t, defaultBackend(), model)

	brief, err := agent.Research(context.Background(), Request{Topic: "Gateway API"}, nil)
	if err != nil {
		t.Fatalf("Research failed on an unparseable plan instead of falling back: %v", err)
	}
	if len(brief.Queries) != 1 || brief.Queries[0] != "Gateway API" {
		t.Errorf("got queries %v, want a fallback to the topic itself", brief.Queries)
	}
}

// When the judge is unreachable the quote-verified findings should survive.
// They have already passed both mechanical checks; discarding them because one
// LLM call failed would throw away provably grounded material.
func TestResearch_KeepsGroundedFindingsWhenTheJudgeFails(t *testing.T) {
	model := &scriptedLLM{
		failOn: "fact-checking citations",
		responses: []scriptedResponse{
			{trigger: "planning research", content: `{"queries": ["q"]}`},
			{trigger: "extracting citable facts", content: `{"findings": [{"claim": "c", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability"}]}`},
			{trigger: "Summarise the current state", content: "summary"},
		},
	}

	agent := newTestAgent(t, defaultBackend(), model)

	brief, err := agent.Research(context.Background(), Request{Topic: "Gateway API"}, nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}
	if len(brief.Findings) != 1 {
		t.Errorf("got %d findings, want the quote-verified finding kept", len(brief.Findings))
	}

	var noted bool
	for _, w := range brief.Warnings {
		if strings.Contains(w, "relevance check unavailable") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("the unjudged state was not disclosed in warnings: %v", brief.Warnings)
	}
}

// A synthesis failure is cosmetic — the findings are the substance.
func TestResearch_SurvivesSynthesisFailure(t *testing.T) {
	model := &scriptedLLM{
		failOn: "Summarise the current state",
		responses: []scriptedResponse{
			{trigger: "planning research", content: `{"queries": ["q"]}`},
			{trigger: "extracting citable facts", content: `{"findings": [{"claim": "c", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability"}]}`},
			{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		},
	}

	agent := newTestAgent(t, defaultBackend(), model)

	brief, err := agent.Research(context.Background(), Request{Topic: "Gateway API"}, nil)
	if err != nil {
		t.Fatalf("Research failed on a cosmetic synthesis error: %v", err)
	}
	if len(brief.Findings) != 1 {
		t.Errorf("got %d findings, want 1", len(brief.Findings))
	}
}

// offTopicBackend reproduces the case a live run actually hit: searching for
// the Kubernetes Gateway API returns well-written pages about API gateway
// vendors, which share every keyword and none of the subject.
func offTopicBackend() *fixedBackend {
	return &fixedBackend{
		sources: []Source{
			{URL: "https://zuplo.com/blog/openapi-native", Title: "Zuplo now natively supports OpenAPI", Site: "zuplo.com"},
			{URL: "https://kubernetes.io/blog/gateway-ga", Title: "Gateway API is GA", Site: "kubernetes.io"},
		},
		docs: map[string]*Document{
			"https://zuplo.com/blog/openapi-native": {
				Source: Source{URL: "https://zuplo.com/blog/openapi-native", Site: "zuplo.com"},
				Text:   "Zuplo now uses the OpenAPI specification standard at its core. Any valid OpenAPI document is a valid API Gateway configuration for Zuplo.",
			},
			"https://kubernetes.io/blog/gateway-ga": {
				Source: Source{URL: "https://kubernetes.io/blog/gateway-ga", Site: "kubernetes.io"},
				Text:   testDocText,
			},
		},
	}
}

// An article about the Kubernetes Gateway API must not end up citing a vendor
// post about OpenAPI, however well-written that post is. Relevance is judged
// before anything is read.
func TestResearch_SetsAsideSourcesThatAreNotAboutTheTopic(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["gateway api ga"]}`},
		// The selector keeps only the Kubernetes source.
		{trigger: "choosing which search results", content: `{"selected": [1]}`},
		{trigger: "extracting citable facts", content: `{"findings": [{"claim": "Gateway API reached GA.", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability"}]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}

	agent := newTestAgent(t, offTopicBackend(), model)

	brief, err := agent.Research(context.Background(), Request{Topic: "Kubernetes Gateway API"}, nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}
	if len(brief.Sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(brief.Sources))
	}
	if brief.Sources[0].Site != "kubernetes.io" {
		t.Errorf("cited %q, want the on-topic source", brief.Sources[0].Site)
	}
	// The off-topic page must never have been fetched — the whole point of
	// judging relevance before reading is not paying to read it.
	for _, f := range brief.Findings {
		if strings.Contains(f.SourceURL, "zuplo") {
			t.Errorf("finding cites the off-topic source: %+v", f)
		}
	}
}

// If the selector rejects every hit, research must recover rather than fail the
// article. Niche topics (and flaky local models) routinely produce an empty
// selection even when an on-topic page is in the candidate list — falling back
// to search order lets extraction and quote verification drop the rest.
func TestResearch_RecoversWhenSelectionFindsNothingOnTopic(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["q"]}`},
		// Both selection passes (initial + recovery) return empty.
		{trigger: "choosing which search results", content: `{"selected": []}`},
		{trigger: "extracting citable facts", content: `{"findings": [{"claim": "Gateway API reached GA.", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability"}]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}

	agent := newTestAgent(t, offTopicBackend(), model)

	brief, err := agent.Research(context.Background(), Request{Topic: "Kubernetes Gateway API"}, nil)
	if err != nil {
		t.Fatalf("Research failed instead of recovering from an empty selection: %v", err)
	}
	if !brief.IsUsable() {
		t.Fatal("brief reports itself unusable after selection recovery")
	}
	var noted bool
	for _, w := range brief.Warnings {
		if strings.Contains(w, "reading top") || strings.Contains(w, "broader search") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("recovery was not disclosed in warnings: %v", brief.Warnings)
	}
}

// When every candidate is off-topic and nothing verifies, research must still
// fail — recovery is not a licence to invent a brief from irrelevant pages.
func TestResearch_FailsWhenFallbackSourcesDoNotVerify(t *testing.T) {
	backend := &fixedBackend{
		sources: []Source{
			{URL: "https://zuplo.com/blog/openapi-native", Title: "Zuplo OpenAPI", Site: "zuplo.com"},
		},
		docs: map[string]*Document{
			"https://zuplo.com/blog/openapi-native": {
				Source: Source{URL: "https://zuplo.com/blog/openapi-native", Site: "zuplo.com"},
				Text:   "Zuplo now uses the OpenAPI specification standard at its core.",
			},
		},
	}
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["q"]}`},
		{trigger: "choosing which search results", content: `{"selected": []}`},
		// Quote is not in the off-topic page → mechanical verify drops it.
		{trigger: "extracting citable facts", content: `{"findings": [{"claim": "Invented.", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability"}]}`},
	}}

	agent := newTestAgent(t, backend, model)

	_, err := agent.Research(context.Background(), Request{Topic: "Kubernetes Gateway API"}, nil)
	if err == nil {
		t.Fatal("Research succeeded with nothing verifiable after selection fallback")
	}
	if !strings.Contains(err.Error(), "none of the") && !strings.Contains(err.Error(), "no claims") {
		t.Errorf("error %q should say nothing verified", err)
	}
}

// A selector that breaks should cost relevance filtering, not the whole run.
func TestResearch_FallsBackToSearchOrderWhenSelectionFails(t *testing.T) {
	model := &scriptedLLM{
		failOn: "choosing which search results",
		responses: []scriptedResponse{
			{trigger: "planning research", content: `{"queries": ["q"]}`},
			{trigger: "extracting citable facts", content: `{"findings": [{"claim": "c", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability"}]}`},
			{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
			{trigger: "Summarise the current state", content: "summary"},
		},
	}

	agent := newTestAgent(t, offTopicBackend(), model)

	brief, err := agent.Research(context.Background(), Request{Topic: "Kubernetes Gateway API"}, nil)
	if err != nil {
		t.Fatalf("Research failed when only the selector was broken: %v", err)
	}
	if len(brief.Findings) == 0 {
		t.Error("no findings after falling back to search order")
	}
}

func TestResearch_RejectsEmptyTopic(t *testing.T) {
	agent := newTestAgent(t, defaultBackend(), &scriptedLLM{})
	if _, err := agent.Research(context.Background(), Request{Topic: "   "}, nil); err == nil {
		t.Fatal("Research accepted an empty topic")
	}
}

func TestResearch_ReportsProgressPhases(t *testing.T) {
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "planning research", content: `{"queries": ["q"]}`},
		{trigger: "extracting citable facts", content: `{"findings": [{"claim": "c", "quote": "Kubernetes 1.31 promoted the Gateway API to general availability"}]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "summary"},
	}}

	agent := newTestAgent(t, defaultBackend(), model)

	var phases []string
	_, err := agent.Research(context.Background(), Request{Topic: "Gateway API"},
		func(phase, _ string) { phases = append(phases, phase) })
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}

	want := []string{PhasePlanning, PhaseSearching, PhaseReading, PhaseVerifying, PhaseSynthesised}
	if len(phases) != len(want) {
		t.Fatalf("got phases %v, want %v", phases, want)
	}
	for i, w := range want {
		if phases[i] != w {
			t.Errorf("phase %d = %q, want %q", i, phases[i], w)
		}
	}
}

func TestBriefIsUsable(t *testing.T) {
	tests := []struct {
		name  string
		brief *Brief
		want  bool
	}{
		{"nil", nil, false},
		{"empty", &Brief{}, false},
		{"findings but no sources", &Brief{Findings: []Finding{{Claim: "c"}}}, false},
		{"sources but no findings", &Brief{Sources: []Source{{URL: "u"}}}, false},
		{"both", &Brief{Findings: []Finding{{Claim: "c"}}, Sources: []Source{{URL: "u"}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.brief.IsUsable(); got != tt.want {
				t.Errorf("IsUsable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCitedSources_OnlyIncludesSourcesThatSurvived(t *testing.T) {
	// A source that was read but supports no surviving finding must not appear
	// in the reference list — a reference list is what the piece rests on, not
	// everything the agent opened.
	docs := []Document{
		{Source: Source{URL: "https://a.com/1", Title: "Cited"}},
		{Source: Source{URL: "https://b.com/2", Title: "Read but unused"}},
	}
	findings := []Finding{
		{Claim: "x", SourceURL: "https://a.com/1", Quote: "q"},
		{Claim: "y", SourceURL: "https://a.com/1", Quote: "q2"},
	}

	got := citedSources(findings, docs)

	if len(got) != 1 {
		t.Fatalf("got %d sources, want 1", len(got))
	}
	if got[0].Title != "Cited" {
		t.Errorf("got %q, want the cited source", got[0].Title)
	}
}
