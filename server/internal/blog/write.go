package blog

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

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

	var plan writePlan
	if err := r.generateJSON(ctx, modelName, "plan", prompt, maxPlanTokens, &plan); err != nil {
		return nil, err
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
- Include a DIAGRAM where one genuinely helps — see the diagram rules below.
%s- Cite a source with [n] wherever you state a specific fact, version, number or
  quotation drawn from it. Do not cite a number that is not in the list above.
- Anything describing HOW A TECHNOLOGY WORKS is a factual claim and needs a
  citation — what runs where, what a component does, what replaces what. This
  holds even when it feels like common background knowledge. If no source above
  supports it, leave it out; a shorter accurate article beats a longer one with
  a confident mistake in it.
- You may explain, contextualise, draw connections and give opinions in your own
  voice. What you may not do is assert an unsupported mechanism as fact.
- Do NOT write a "Further Reading" or "References" section. The reference list
  is generated separately from the citations you use.

%s

Return only the markdown article — no preamble, no meta commentary.`,
		plan.Title,
		plan.Angle,
		formatSections(plan.Sections),
		guidanceOrNone(brief.Context),
		formatSources(rb),
		plan.Title,
		r.illustrationRequirement(),
		r.visualRules(),
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

- UNCITED TECHNICAL ASSERTIONS. This is the most important check. Any sentence
  explaining how something works — what runs where, what a component does, what
  happens to data, what a technology replaces — is a factual claim, even when it
  sounds like general background. If it has no [n] citation, flag it. Judge it
  on its own merits too: if such a sentence is technically WRONG, say so plainly
  and say what is actually true. Do not assume the writer got the basics right.
- Specific claims (versions, numbers, dates, capabilities) presented as fact with
  no [n] citation, or citing a number that is not in the source list.
- Claims that go further than the cited source actually supports.
- Filler: paragraphs that restate the heading, or that would survive being
  deleted without the reader losing anything.
- Sections that do not deliver the intended angle.
- Missing or broken code blocks, or code that would not run.
- Sections that are too thin to be useful — a heading with one short paragraph
  under it.
- Diagrams drawn as ASCII art instead of a mermaid fence — boxes made of dashes,
  arrows made of hyphens. Flag every one.
- Decision nodes in a diagram whose branches all lead to the same place, which
  makes the decision meaningless.
- A diagram that contradicts the prose around it, or that no sentence refers to.

List each problem as one specific, actionable sentence naming where it occurs.
If the draft has no real problems, return an empty list — do not invent work.

Respond with JSON only, in exactly this shape:
{"issues": ["...", "..."]}`,
		plan.Title, plan.Angle, formatSources(rb), markdown)

	var c critique
	if err := r.generateJSON(ctx, modelName, "review", prompt, maxPlanTokens, &c); err != nil {
		return nil, err
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

Rewrite the article addressing every problem listed.

For an uncited or incorrect technical claim you have exactly three options:
cite a source above that genuinely supports it, correct it to what is actually
true, or delete the sentence. Do not invent a citation number, and do not blur a
claim into vagueness to avoid having to support it — a sentence that survives by
saying nothing is worse than one that is gone.

Keep everything that was already working. Preserve the H1 title, the markdown
structure, the length, and any mermaid diagrams that were fine. If a diagram was
flagged, fix it in place — redraw it as a mermaid fence if it was ASCII art, or
delete it if it was not earning its space. Never convert a mermaid fence back
into text. Do not add a "Further Reading" or "References" section.

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

// diagramRules is appended to the drafting and revision prompts.
//
// Every rule here comes from watching the models actually try. Both were asked
// for six diagrams and both produced six that render, but what they are good at
// differs sharply, and the failures are specific rather than general:
//
//   - Sequence, state and entity-relationship diagrams come out well. Those
//     three have distinctive syntax that is hard to fake, and the models used
//     it correctly — real participants and aliases, real [*] terminals, real
//     cardinality markers.
//   - Class diagrams do not. Asked for one, both models produced a flowchart
//     instead, and one of them wired it into a meaningless ring: Agent to
//     Searcher to Fetcher to Document to Finding and back to Agent. So class
//     diagrams are steered away from entirely.
//   - Decision nodes are the reliable failure. Both models drew "TLS
//     terminate?" with the Yes branch and the No branch going to the same
//     node — a question whose answer changes nothing. Hence the explicit rule.
//
// The ban on ASCII art is the point of the whole feature: a diagram made of
// dashes and arrows in a code fence is worse than no diagram, because it
// survives review looking deliberate.
const diagramRules = `
DIAGRAMS:

Include one or two Mermaid diagrams where a diagram genuinely explains
something faster than a paragraph — a request path, a protocol exchange, a
lifecycle, a data model. Do not add one to every section, and do not add one
that merely restates a list.

Write them as a mermaid code fence:

  ` + "```" + `mermaid
  sequenceDiagram
      participant C as Client
      participant S as Server
      C->>S: request
      S-->>C: response
  ` + "```" + `

Use whichever of these fits:
  sequenceDiagram    an exchange between parties over time
  stateDiagram-v2    a lifecycle, with [*] as the start and end
  erDiagram          a data model, with cardinality and typed attributes
  flowchart LR/TD    a path through a system

Rules:
- NEVER draw a diagram with ASCII art — no boxes made of dashes and pipes, no
  arrows made of hyphens and angle brackets. A mermaid fence or nothing.
- Do not use classDiagram. Model structure as a flowchart instead.
- Every decision node must have branches that lead somewhere DIFFERENT. If both
  answers go to the same place, it is not a decision — delete the node.
- Label nodes with short readable text: A[Load Balancer], not just LB.
- Keep it under about twelve nodes. A diagram nobody can read helps nobody.
- The diagram must agree with the surrounding prose and with the sources. It is
  a claim like any other, not decoration.
`

// illustrationRules is appended to the diagram rules only when the run can
// actually draw.
//
// Offering it unconditionally would be worse than not offering it: a model told
// it may request pictures will request them, illustrateBody would strip every
// block on a server with no image provider, and the article would arrive with
// paragraphs referring to figures that are not there.
//
// The budget is interpolated from maxInlineIllustrations rather than written
// out, because the two disagreed for a while: the pipeline allowed three and
// this prompt allowed one, so a model told a picture was rarely worth having
// and capped at a single instance reliably asked for none. Deriving the number
// makes that particular drift unrepresentable. illustrateBody remains the
// enforcement — a prompt is advice, and blocks past the cap are dropped there.
var illustrationRules = fmt.Sprintf(`
ILLUSTRATIONS:

You may request up to %d generated illustrations, and should use at least one in
a long piece. They earn their place at a section boundary in an expository
stretch — an opening image that sets up the subject, or a concrete scene the
prose then unpacks — where a diagram would be false precision because there is
no real structure to draw.

Prefer a diagram whenever the content genuinely has structure: a diagram carries
information, an illustration carries atmosphere, and a reader can tell.

Request one by writing an illustration fence whose body describes the picture:

  `+"```"+`illustration
  A lighthouse on a rocky shore at dawn, seen from the water
  `+"```"+`

Rules:
- Describe a CONCRETE SCENE — a thing that could be photographed. "A lighthouse
  at dawn" works. "The concept of reliability" does not, and produces a muddle.
- Do not ask for text, labels, diagrams, charts or UI in an illustration. Image
  models render lettering badly, and a diagram belongs in a mermaid fence.
- Do not use one to replace a diagram, and never to illustrate a claim that
  needs a source.
- Spread them out: never two in adjacent sections, and never two that describe
  the same scene.
- The description becomes the image's alt text, so write it as a sentence a
  screen-reader user would find useful.
`, maxInlineIllustrations)

// visualRules is the visual half of the drafting prompt: diagrams always, plus
// illustrations when this run has a generator behind it.
func (r *Runner) visualRules() string {
	if !r.canIllustrate() {
		return diagramRules
	}
	return diagramRules + illustrationRules
}

// Target article length. The draft prompt asks for this range, but asking is
// not getting: a live run against a local model produced 382 words against the
// same instruction. So the floor is checked rather than trusted, which is the
// same stance this package takes on citations.
const (
	MinArticleWords = 900
	MaxArticleWords = 1400
)

// expand fills out an article that came in under the target length.
//
// It is deliberately not "write more". A model told to lengthen a piece pads it
// — restating the heading, adding a paragraph of throat-clearing before each
// section — which makes the article longer and worse. So it is given the
// specific sections that are thin, told what material it still has available,
// and told which kinds of expansion actually carry weight.
func (r *Runner) expand(
	ctx context.Context, modelName string, brief model.BlogBrief, rb *research.Brief,
	plan *writePlan, markdown string, currentWords int,
) (string, error) {
	prompt := fmt.Sprintf(`This article is too short. It is %d words and needs to be %d-%d.

TITLE: %s
ANGLE: %s

GUIDANCE FROM THE REQUESTER:
%s

SOURCES you may draw on — cite by number, e.g. [2]:
%s

CURRENT ARTICLE:
%s

Expand it to at least %d words by adding substance, not padding. Specifically:
- Develop the sections that are thinnest. A section of one short paragraph is
  the first place to look.
- Add the practical detail a working engineer needs: what the trade-offs are,
  what breaks, what to watch for, what the migration actually involves.
- Add or extend a code block where it earns its place.
- Draw on sources you have not used yet, if any are relevant.

Do NOT:
- Restate the heading as the first sentence of a section.
- Add a preamble to each section explaining what the section will cover.
- Add filler like "in today's fast-moving landscape".
- Introduce facts no source supports. If you cannot support it, do not add it.
- Add a "Further Reading" or "References" section.

Keep the existing title, structure and every citation that is already there.
Return only the expanded markdown article — no preamble, no commentary.`,
		currentWords, MinArticleWords, MaxArticleWords,
		plan.Title, plan.Angle,
		guidanceOrNone(brief.Context),
		formatSources(rb),
		markdown,
		MinArticleWords,
	)

	resp, err := r.generate(ctx, modelName, prompt)
	if err != nil {
		return "", fmt.Errorf("expand: %w", err)
	}
	md := stripOuterFence(strings.TrimSpace(resp))
	if md == "" {
		return "", fmt.Errorf("expand: model returned an empty article")
	}
	return md, nil
}

// wordCount counts words the way the UI reports them.
func wordCount(markdown string) int { return len(strings.Fields(markdown)) }

// generate is the single point where the writer talks to the LLM.
// Generation ceilings, in tokens. See the note in research/agent.go — these
// bound a runaway rather than shape the output.
//
// The article ceiling is deliberately roomy: MaxArticleWords is 1400, which is
// roughly 1900 tokens of prose, and markdown structure and code blocks push it
// higher. Truncating a finished article mid-sentence would be a worse failure
// than the one this guards against.
const (
	// maxArticleTokens covers drafting, revising and expanding.
	maxArticleTokens = 4000
	// maxPlanTokens covers the outline and the review, both short JSON.
	maxPlanTokens = 1500
)

func (r *Runner) generate(ctx context.Context, modelName, prompt string) (string, error) {
	return r.generateBounded(ctx, modelName, prompt, maxArticleTokens)
}

// generateJSON asks for a JSON reply and decodes it into v.
//
// The stages that ask for JSON used to fail the article outright on a reply
// they could not parse, which made the choice of model hostage to one fragile
// parse. Now a malformed reply is repaired where it can be and asked for again
// where it cannot.
func (r *Runner) generateJSON(
	ctx context.Context, modelName, stage, prompt string, maxTokens int, v any,
) error {
	return llm.GenerateJSON(ctx, stage, prompt, v,
		func(ctx context.Context, p string) (string, error) {
			return r.generateBounded(ctx, modelName, p, maxTokens)
		},
		func(reply string, err error) {
			r.logger.Warn("blog: could not parse the model's JSON, asking again",
				zap.String("stage", stage), zap.String("model", modelName), zap.Error(err))
		},
	)
}

func (r *Runner) generateBounded(ctx context.Context, modelName, prompt string, maxTokens int) (string, error) {
	resp, err := r.llm.Generate(ctx, llm.GenerateRequest{
		Model:     modelName,
		MaxTokens: maxTokens,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: prompt}},
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
