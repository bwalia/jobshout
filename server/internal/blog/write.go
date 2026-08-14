package blog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// writePlan is what the agent decides before drafting: what to call the piece
// and how to structure it.
type writePlan struct {
	// Title is chosen from what the research found, not from the topic string.
	// A topic is a subject; a title is a claim about what this particular
	// article says, and the agent is only in a position to make it after
	// reading the sources.
	Title string `json:"title"`
	// Angle is one sentence on what makes this piece worth reading, carried
	// into the draft prompt to keep a long article pointed the same way.
	Angle    string   `json:"angle"`
	Sections []string `json:"sections"`
}

// plan chooses a title and an outline from the research brief.
func (r *Runner) plan(ctx context.Context, modelName string, brief model.BlogBrief, rb *research.Brief) (*writePlan, error) {
	prompt := fmt.Sprintf(`You are planning a technical article for a developer audience.

TOPIC (a subject, not a title):
%s

GUIDANCE FROM THE REQUESTER:
%s

WHAT THE RESEARCH FOUND:
%s

VERIFIED FINDINGS YOU MAY BUILD ON:
%s

Decide:
1. A title. Base it on what the research actually found — the specific, current
   thing worth saying about this topic — not on restating the topic. Make it
   concrete and under 70 characters. No colons-and-subtitles, no "A Guide To".
2. One sentence on the angle: what a reader gets from this piece.
3. Four to six section headings that deliver that angle.

Respond with JSON only, in exactly this shape:
{"title": "...", "angle": "...", "sections": ["...", "..."]}`,
		brief.Topic,
		guidanceOrNone(brief.Context),
		orNone(rb.Summary),
		formatFindings(rb.Findings),
	)

	resp, err := r.generate(ctx, modelName, prompt)
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}

	var plan writePlan
	if err := json.Unmarshal([]byte(extractJSON(resp)), &plan); err != nil {
		return nil, fmt.Errorf("plan: parse response: %w", err)
	}
	plan.Title = strings.TrimSpace(plan.Title)
	if plan.Title == "" {
		return nil, fmt.Errorf("plan: model returned no title")
	}
	return &plan, nil
}

// draft writes the article against the plan and the verified findings.
//
// The findings are numbered and the model is told to cite by number, rather
// than being asked to produce URLs itself. A model writing a URL from memory
// invents one that looks right; a model writing "[3]" can only be wrong about
// which source it meant, and that is a mistake the review pass can catch.
func (r *Runner) draft(
	ctx context.Context, modelName string, brief model.BlogBrief, rb *research.Brief, plan *writePlan,
) (string, error) {
	prompt := fmt.Sprintf(`You are writing a technical article for a developer audience.

TITLE: %s
ANGLE: %s

SECTIONS TO COVER:
%s

GUIDANCE FROM THE REQUESTER:
%s

SOURCES — these are the only facts you may present as established. Each is
numbered; cite one by putting its number in square brackets, like [2], at the
end of the sentence that relies on it.
%s

Requirements:
- Pure markdown. No code fence around the whole response, no HTML.
- Start with a single H1 line: # %s
- Use H2/H3 headings for the sections above.
- 900-1400 words.
- Include at least one code block where it genuinely helps.
- Cite a source with [n] wherever you state a specific fact, version, number or
  quotation drawn from it. Do not cite a number that is not in the list above.
- You may explain, contextualise and give opinions in your own voice — just do
  not present an unsourced specific as established fact.
- Do NOT write a "Further Reading" or "References" section. The reference list
  is generated separately from the citations you use.

Return only the markdown article — no preamble, no meta commentary.`,
		plan.Title,
		plan.Angle,
		formatSections(plan.Sections),
		guidanceOrNone(brief.Context),
		formatSources(rb),
		plan.Title,
	)

	resp, err := r.generate(ctx, modelName, prompt)
	if err != nil {
		return "", fmt.Errorf("draft: %w", err)
	}
	md := stripOuterFence(strings.TrimSpace(resp))
	if md == "" {
		return "", fmt.Errorf("draft: model returned an empty article")
	}
	return md, nil
}

// critique is the agent reading its own draft before anyone else does.
type critique struct {
	// Issues are concrete problems to fix. An empty list means the draft is
	// good enough to ship as-is, which is a real outcome and not a failure.
	Issues []string `json:"issues"`
}

// review asks the model what is wrong with the draft it just wrote.
//
// Separating review from revision matters more than it looks. Asking a model to
// "improve this" in one pass mostly produces a reworded version of the same
// article, because it has nothing to aim at. Making it first name specific
// defects, then fix those named defects, gives the second pass something
// concrete to act on — and gives us a record of what it thought was wrong.
func (r *Runner) review(
	ctx context.Context, modelName string, rb *research.Brief, plan *writePlan, markdown string,
) (*critique, error) {
	prompt := fmt.Sprintf(`You are reviewing a draft technical article before publication. Be a harsh critic.

INTENDED TITLE: %s
INTENDED ANGLE: %s

AVAILABLE SOURCES (cited by number in the draft):
%s

DRAFT:
%s

Find concrete problems. Look specifically for:
- Specific claims (versions, numbers, dates, capabilities) presented as fact with
  no [n] citation, or citing a number that is not in the source list.
- Claims that go further than the cited source actually supports.
- Filler: paragraphs that restate the heading, or that would survive being
  deleted without the reader losing anything.
- Sections that do not deliver the intended angle.
- Missing or broken code blocks, or code that would not run.

List each problem as one specific, actionable sentence naming where it occurs.
If the draft has no real problems, return an empty list — do not invent work.

Respond with JSON only, in exactly this shape:
{"issues": ["...", "..."]}`,
		plan.Title, plan.Angle, formatSources(rb), markdown)

	resp, err := r.generate(ctx, modelName, prompt)
	if err != nil {
		return nil, fmt.Errorf("review: %w", err)
	}

	var c critique
	if err := json.Unmarshal([]byte(extractJSON(resp)), &c); err != nil {
		return nil, fmt.Errorf("review: parse response: %w", err)
	}

	out := make([]string, 0, len(c.Issues))
	for _, issue := range c.Issues {
		if s := strings.TrimSpace(issue); s != "" {
			out = append(out, s)
		}
	}
	c.Issues = out
	return &c, nil
}

// revise rewrites the draft to address the critique.
func (r *Runner) revise(
	ctx context.Context, modelName string, rb *research.Brief, plan *writePlan, markdown string, c *critique,
) (string, error) {
	prompt := fmt.Sprintf(`You are revising a technical article to fix specific problems a reviewer found.

TITLE: %s

AVAILABLE SOURCES (cite by number):
%s

PROBLEMS TO FIX:
%s

CURRENT DRAFT:
%s

Rewrite the article addressing every problem listed. Fix an uncited claim either
by citing a source that supports it or by removing the claim — do not invent a
citation number, and do not soften a claim into vagueness to avoid citing it.

Keep everything that was already working. Preserve the H1 title, the markdown
structure and the length. Do not add a "Further Reading" or "References" section.

Return only the revised markdown article — no preamble, no commentary on what
you changed.`,
		plan.Title, formatSources(rb), formatIssues(c.Issues), markdown)

	resp, err := r.generate(ctx, modelName, prompt)
	if err != nil {
		return "", fmt.Errorf("revise: %w", err)
	}
	md := stripOuterFence(strings.TrimSpace(resp))
	if md == "" {
		return "", fmt.Errorf("revise: model returned an empty article")
	}
	return md, nil
}

// generate is the single point where the writer talks to the LLM.
func (r *Runner) generate(ctx context.Context, modelName, prompt string) (string, error) {
	resp, err := r.llm.Generate(ctx, llm.GenerateRequest{
		Model:    modelName,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return "", fmt.Errorf("empty response from %s", r.llm.ProviderName())
	}
	return resp.Content, nil
}

// formatFindings numbers the verified claims for a prompt.
func formatFindings(findings []research.Finding) string {
	if len(findings) == 0 {
		return "(none)"
	}
	var b strings.Builder
	for i, f := range findings {
		fmt.Fprintf(&b, "[%d] %s\n    source: %s\n", i+1, f.Claim, f.SourceURL)
	}
	return b.String()
}

// formatSources numbers the sources exactly as the draft is told to cite them.
//
// The numbering is 1-based and shared by every prompt in this file, so the [n]
// a draft writes, the [n] the reviewer checks and the [n] the reference list is
// built from all refer to the same source.
func formatSources(rb *research.Brief) string {
	if rb == nil || len(rb.Findings) == 0 {
		return "(none)"
	}
	var b strings.Builder
	for i, f := range rb.Findings {
		fmt.Fprintf(&b, "[%d] %s\n    from: %s\n    supporting quote: %s\n",
			i+1, f.Claim, f.SourceURL, f.Quote)
	}
	return b.String()
}

func formatSections(sections []string) string {
	if len(sections) == 0 {
		return "(decide the structure yourself)"
	}
	var b strings.Builder
	for _, s := range sections {
		fmt.Fprintf(&b, "- %s\n", s)
	}
	return b.String()
}

func formatIssues(issues []string) string {
	var b strings.Builder
	for i, s := range issues {
		fmt.Fprintf(&b, "%d. %s\n", i+1, s)
	}
	return b.String()
}

func guidanceOrNone(s string) string {
	return orNone(strings.TrimSpace(s))
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none given)"
	}
	return s
}

// extractJSON pulls a JSON object out of a model response that may be wrapped
// in prose or a code fence.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end < start {
		return s
	}
	return s[start : end+1]
}
