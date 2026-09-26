# Article Agent — Plan

## Purpose

Research a topic and write a cited article. Generate a unique-but-on-theme cover, and generate images **inside** the article body (not cover-only).

## Current behaviour

`blog.Runner` (`server/internal/blog/content.go`): research → plan → draft → review → revise → expand → `illustrateBody` (only if the writer emits ` ```illustration `) → `generateCover` → render.

`illustrationRules` in `write.go` tells the writer **“AT MOST ONE”** and **“Most articles need none.”** `coverPrompt` in `illustrate.go` is one fixed charcoal/teal template; only title/topic text vary. `BLOG_COVER_IMAGES` gates **both** cover and inline.

Chat `article_generate` passes `Topics` only — **drops `context`**. Launch does not pass `task_id`. `writeSteps()` does not pre-seed `BlogStepIllustrating`.

## Evals first (baseline)

File: `server/internal/blog/article_eval_test.go` (fakes, no GPU).

- Writer prompt contains illustration rules only when illustrator is enabled.
- A draft **without** fences produces cover-only (today’s behaviour) until the insert-pass lands; after it, a draft without fences still gets 1–2 body images when the illustrator is on.
- A draft **with** fences is replaced by `![alt](url)` (keep `illustrate_test.go`).
- `coverPrompt` for two topics shares house-style tokens (navy, left title, teal/coral) and differs in metaphor/subject.

## Code changes after evals

1. **Body images by policy.** After `expand()`, if `canIllustrate()` and the draft has zero fences, insert 1–2 ` ```illustration ` blocks at H2 boundaries (concrete scene from heading + surrounding prose). Align prompt with `maxInlineIllustrations = 3`: ask for 1–2, cap 3. Drop “Most articles need none.”
2. **Unique covers, shared theme.** After `plan()`, a structured LLM step outputs `{ metaphor, focal_objects, accent_note }`. Feed those into `coverPrompt` while keeping house style (charcoal navy, left title, teal/coral, flat vector).
3. **Wire `context` and `task_id`:** specialists + `GenerateBlogRequest`; Task Manager / `tasklaunch` pass `task_id`; board task updates when the run completes.
4. **Pre-seed `BlogStepIllustrating`** in `writeSteps()`.

## Acceptance

- New articles with images enabled have a cover **and** at least one inline image when the illustrator is on.
- Covers stay on-brand but do not share the same generic “tools/documents/machines” clause.
- Eval file asserts the new contract.
