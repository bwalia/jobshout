# Plan 2 — Article Agent

> ## Execution status — 2026-08-29
>
> **Done: upgrades 1 and 2, plus the distinctiveness suite.**
>
> Upgrade 1 was a prompt fix, as diagnosed: `illustrationRules` now advertises
> `maxInlineIllustrations` (interpolated from the constant, so the prompt and
> the pipeline cannot disagree again), asks for at least one in a long piece,
> and adds a spread rule. No pipeline change was needed.
>
> Upgrade 2 landed as specified: `coverVariant` + `variantFor` vary accent,
> title placement, focal arrangement and lighting, keyed on a per-axis divided
> FNV hash of the topic. Brand axes stay pinned. Measured result: **12 distinct
> treatments across 12 topics**, all four accents and all three compositions and
> lightings exercised.
>
> `TestCoverPrompt_NamesASubjectAndPinsTheStyle` asserted `LEFT`, which after
> this change passed only because that topic happened to hash to LEFT. It now
> asserts the invariant — the prompt commits to one of the curated placements.
>
> Suite B lives in `internal/blog/cover_diversity_eval_test.go` rather than
> `server/eval/article/`: `coverPrompt` is unexported, and widening the package
> API so an eval could reach it would be the tail wagging the dog. It writes its
> report through the shared harness.
>
> Suite A is already covered by the existing `TestIllustrateBody_*` tests.
>
> ## Phase 4 — 2026-08-30
>
> **Done.** Plan 4's single contract landed, so the two optional controls are
> now surfaced. `agentschema.ForBuiltin(article_writer)` and its TypeScript twin
> gained `cover_style` (a select over the four `coverAccents`, empty = the
> per-topic default) and `illustrations` (on/off, on by default); the parity
> test keeps the two copies in step. The values thread
> `GenerateBlogRequest → blog.GenerateRequest → coverPromptWithAccent` (accent
> override, unknown values fall back to the variant) and gate both
> `ensureIllustrations` and `illustrateBody`, with any writer-emitted fence
> stripped when off so the cover is untouched but the body draws nothing. The
> Task Manager form renders both from the schema automatically. New unit tests:
> `cover_style_test.go`.

Verified against `feat/landing-page` @ `063cce3`.

Two upgrades were requested. **One is already built and simply never fires**;
the other is real and the diagnosis is precise.

---

## What is actually in the repo

`server/internal/blog/` — the pipeline is mature and well-tested:

```
research → plan → draft → review → revise → expand
        → diagrams (mermaid) → citations → illustrate → render → publish
```

| File | Role |
|---|---|
| `runner.go` | stage orchestration, `blog_runs` row, progress |
| `write.go` | writing prompts, `diagramRules`, `illustrationRules` |
| `content.go` | per-article assembly; calls `illustrateBody` at `:146` |
| `illustrate.go` | cover generation + in-body illustration |
| `mermaid.go` | diagram rendering |
| `citations.go` | source attribution |
| `html.go` | CMS render |

Every one has a `_test.go` beside it. `blog_runs` is one of the three tables the
agent board reads, so article runs **do** appear on the board.

---

## Upgrade 1 — "generate images inside the articles, not just the cover"

### This is already implemented

`illustrate.go:79` defines the fence the model may emit:

```go
var illustrationFence = regexp.MustCompile("(?s)```illustration[ \t]*\r?\n(.*?)```")
```

`illustrate.go:297` `illustrateBody` finds those blocks, generates an image per
block, and substitutes a markdown image with the description as alt text. It is
wired into the pipeline at `content.go:146`. The cap is three:

```go
const maxInlineIllustrations = 3
```

### So why does the user never see in-body images?

Because the writing prompt tells the model not to. `write.go:325`
`illustrationRules`:

> *"You may request **AT MOST ONE** generated illustration, and only where a
> picture genuinely helps … a diagram carries information, an illustration
> [does not]; a diagram is almost always the better choice."*

The prompt caps at **one** while the pipeline allows **three**, and then spends
its remaining sentences arguing the model out of using even that one. A model
that is told a thing is rarely worth doing, and capped at one instance, reliably
does it zero times.

**The fix is the prompt, not the pipeline.** This is roughly a one-paragraph
change, and it is the highest payoff-per-line item in this entire plan set.

### The change

In `write.go`, rewrite `illustrationRules` to:

- Raise the stated cap from one to `maxInlineIllustrations` (three), and derive
  the number from the constant with `fmt.Sprintf` so the prompt and the pipeline
  can never disagree again.
- Reframe from discouragement to guidance: illustrations belong at **section
  boundaries in long expository passages** where a diagram would be false
  precision — a conceptual opener, a metaphor, a "what this feels like in
  practice" break. Diagrams stay preferred for anything with real structure.
- Keep the existing prohibition on text, labels, charts and UI inside an
  illustration. That constraint is correct: the image model renders text badly,
  which is exactly why `coverTitleMaxWords` is 5.
- Add a placement rule: never two illustrations in adjacent sections.

Leave `maxInlineIllustrations = 3` as the hard backstop. Prompts are advisory;
`illustrateBody` is the enforcement, and it already drops overflow blocks.

---

## Upgrade 2 — "covers all look almost identical"

### This complaint is accurate, and the cause is a single fixed template

`illustrate.go:142` `coverPrompt` is one `fmt.Sprintf` with three variables —
`subject`, `headline`, `subtitle`. Everything else is a string literal, and that
literal pins every visual decision:

| Pinned | Value in the template |
|---|---|
| Background | "deep charcoal navy … subtle dark gradient" |
| Palette | teal and cyan glow, coral accent dots |
| Composition | title text **LEFT**, focal objects opposite |
| Style | flat vector, grain texture, soft rim glow |
| Framing | wide 16:9 |
| Focal form | "tools, documents, agents, networks or machines as simple shapes" |

So the *metaphor* varies with the topic and nothing else does. Same background,
same palette, same layout, same lighting, every time. The covers are a set in
the strongest possible sense — which was the original intent (`coverPromptStyle`
comments explain the fear: "left to invent its own style, the same model
produces a photograph for one article and a cartoon for the next") — but it
overshot.

### The goal, stated precisely

Not "make them random". Make them **recognisably the same publication, visibly
different articles**. Vary the axes a reader notices; pin the axes that carry
brand.

| Axis | Treatment |
|---|---|
| Background base (charcoal navy) | **pin** — this is the brand |
| Flat-vector style, grain, rim glow | **pin** |
| No logos / no text beyond title | **pin** |
| Accent hue | **vary** within a curated teal→cyan→violet→coral set |
| Composition | **vary** — title left / title right / title lower-third |
| Focal arrangement | **vary** — single hero object / cluster / layered depth |
| Lighting angle | **vary** — rim from left, top, behind |
| Metaphor | **vary** — already does, via topic |

### The mechanism: deterministic variation keyed by topic

Do **not** use `rand`. Two runs of the same topic should produce the same cover,
and the eval suite needs determinism. Hash the topic and index into curated
option slices:

```go
// coverVariant selects the varying axes deterministically from the topic, so a
// given topic always draws the same cover while different topics spread across
// the set. Curated slices, not free invention: the point is a family, not a
// lottery.
type coverVariant struct {
	accent      string
	composition string
	arrangement string
	lighting    string
}

func variantFor(topic string) coverVariant {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(topic))))
	n := h.Sum32()
	return coverVariant{
		accent:      coverAccents[n%uint32(len(coverAccents))],
		composition: coverCompositions[(n/7)%uint32(len(coverCompositions))],
		arrangement: coverArrangements[(n/13)%uint32(len(coverArrangements))],
		lighting:    coverLightings[(n/29)%uint32(len(coverLightings))],
	}
}
```

Divide by distinct primes before each modulus so the axes do not correlate — a
single hash reused directly would make accent and composition move in lockstep
and collapse the variety back down.

With 4 accents × 3 compositions × 3 arrangements × 3 lightings you get 108
combinations that all still read as one publication.

`coverPrompt` keeps its signature but interpolates the variant. `coverModel`
(`z-image-turbo`), `coverMaxAttempts` and `coverTitleMaxWords` are unchanged.

---

## Evaluations

`server/eval/article/`. Tier 1 uses a fake LLM, a fake image generator that
records prompts, and a fake CMS.

### Suite A — pipeline correctness

| # | Case | Assert |
|---|---|---|
| 1 | model emits two `illustration` fences | two image calls; two markdown images; fences gone from output |
| 2 | model emits five fences | exactly three drawn; overflow dropped, noted |
| 3 | fence with empty body | dropped with a note; no image call |
| 4 | image generator errors | block removed, article still renders — never a stray fence |
| 5 | no fences at all | article renders unchanged; cover still drawn |
| 6 | alt text | equals the fence description, escaped |

Cases 2 and 4 are regression nets for behaviour that already exists and is easy
to break.

### Suite B — image distinctiveness, the actual upgrade metric

This is the suite that proves Upgrade 2 landed. Generate covers for 12 varied
topics, capture the prompts, and assert on the **prompt strings** — hermetic and
deterministic, no GPU:

```go
Check{"covers_share_the_brand", Fatal: true,
    Fn: allPromptsContain("deep charcoal navy", "flat vector", "no logos")}

Check{"covers_differ_from_each_other", Fatal: true,
    Fn: distinctVariantTuples(prompts) >= 8}   // of 12 topics

Check{"same_topic_is_stable", Fatal: true,
    Fn: equalPrompts(coverPrompt("K8s costs", "k8s"), coverPrompt("K8s costs", "k8s"))}

Check{"axes_are_uncorrelated",
    Fn: noAxisPairPerfectlyPredicts(prompts)}  // catches the shared-hash mistake
```

Tier 2 renders the 12 covers for real and writes a contact sheet to
`eval/out/article-covers.html`. Diversity is ultimately a judgement a person
makes by looking; the deterministic checks stop the obvious regressions and the
contact sheet answers the real question.

---

## Implementation

### Phase 1 — the prompt fix (1 hour)

`write.go` `illustrationRules` only. Derive the count from
`maxInlineIllustrations`. Ship this first and independently: it is small,
reversible, and immediately visible in the next article.

### Phase 2 — cover variation (½ day)

Add `coverVariant`, the four curated slices, and `variantFor` to
`illustrate.go`. Interpolate into `coverPrompt`. Update
`TestCoverPrompt_NamesASubjectAndPinsTheStyle` — it currently asserts the pinned
dark template, and those assertions must survive: the pinned axes are still
pinned.

### Phase 3 — evals (½ day)

Suites A and B, plus the Tier 2 contact sheet.

### Phase 4 — surface the controls (depends on Plan 4)

Once Plan 4 lands one input contract, add optional `cover_style` and
`illustrations` fields for the Article Writer so an operator can override the
variant or turn in-body images off. Deliberately last: adding a field to two
hand-synced schemas today doubles the drift problem Plan 4 exists to remove.

---

## Acceptance criteria

- [ ] Prompt permits and encourages up to `maxInlineIllustrations`, count
      derived from the constant
- [ ] A real article run produces ≥1 in-body image on a long expository topic
- [ ] `coverPrompt` varies accent, composition, arrangement and lighting by topic
- [ ] Brand axes still pinned; existing cover test still passes
- [ ] Same topic → identical prompt; ≥8 distinct variants across 12 topics
- [ ] Suites A and B green in CI; contact sheet renders in Tier 2
