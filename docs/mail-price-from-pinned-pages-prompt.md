# Mail Agent — Get real prices from pinned knowledge pages

> Paste everything below the line into the implementing agent. Self-contained.
> Work on a new branch. Do not mix unrelated refactors.
>
> Companion: `#105` (invented prices), `#106` (honesty guard — **keep it**),
> `docs/plans/01-gmail-agent.md`, `server/internal/mail/draft.go`,
> `server/internal/research/agent.go` (`researchPinned`).

---

## ROLE

You are fixing a **remaining Mail Agent gap** after PR **#106**.

#106 stopped the drafter inventing prices. That is good and must stay.

This prompt is the next slice: when a customer asks for a **price** and the
operator has pinned a **product URL**, the reply should quote the **actual
listed price** from pages the org already pointed at — not invent figures, and
not stop at “not listed on those pages” when the price is one click away on
the same product (buy / newsroom / JSON-LD).

Read this whole document before writing code. Evals first. Ship one reviewable
PR. Do not revert `#106`. Do not turn pinned research into open-web search.

---

## PRODUCT INTENT

**One sentence:** *If Knowledge links include a product page and the inbound
mail asks for a price, the draft must quote prices that actually appear on
that product’s pages — or honestly say they are not there after those pages
were read — never from model memory.*

Human Approve still sends. Chat still cannot send.

---

## WHAT ALREADY SHIPPED (do not undo)

### Issue #105 (live on int, v1.0.17)

Operator pinned Knowledge: `https://www.apple.com/mac-studio/`
Subject prefix: `[js-test]`. Watch senders empty. Pinned-only research
(UI: “matching mail is researched from those pages only — not the open web”).

**Email A** — URL also in the body

- Subject: `[js-test] What is the price of this machine?`
- Body included `https://www.apple.com/mac-studio/`
- **Draft:** no dollar amounts. Told the customer to visit the same URL.

**Email B** — price question only (no URL in body)

- Subject: `[js-test] What is the price of the Mac Studio?`
- Body: `Hi, what is the price of the Mac Studio?`
- **Draft:** `$1,999 (base model)` and `$2,499 (fully configured)`, then
  “visit our website or consult with an authorized retailer”.

**Ground truth (Apple, Aug 2026):**

- Mac Studio with **M5 Max** starts at **$2,499** (education $2,299)
- Mac Studio with **M5 Ultra** starts at **$5,499** (education $5,099)
- `apple.com/mac-studio` marketing HTML **does not list dollar amounts**
  (prices live on the buy flow / newsroom)

So the model **invented** a stale $1,999, mis-labelled $2,499 as “fully
configured”, omitted Ultra, and deflected to a website.

### PR #106 (live on int, v1.0.18) — honesty

Two layers in `server/internal/mail/draft.go`:

1. **Prompt:** quote a figure only if it appears in the findings; if not,
   say it is not listed on those pages; never fill from memory; never tell
   the sender to visit a website/retailer instead of answering.
2. **Deterministic check:** every `$` / `£` / `€` amount in the draft must
   appear in the findings, the inbound mail, or operator instructions.
   Unsourced amounts → one redraft; if they persist → `noFigureDraft`
   (“not stated in our reference pages … we will follow up”).

Regression tests in `server/internal/mail/draft_test.go`
(`TestUnsupportedAmounts`, `TestDraftRedraftsWhenAmountNotInFindings`,
`TestDraftFallsBackWhenModelKeepsInventingAmounts`,
`TestDraftKeepsSourcedAmountsWithoutRedraft`).

### Email C — same playbook, after #106 (live on int, v1.0.18)

- Subject: `[js-test] Mac Studio price please`
- Body: `Hi, what is the price of the Mac Studio?` (no URL)
- Research UI: *dropped a claim whose quote does not appear in
  https://www.apple.com/mac-studio/*
- **Draft:**

  > According to our pinned knowledge pages, the current prices for Mac
  > Studio are not listed. We will follow up with exact details. Please let
  > us know if you would like a detailed answer.

Honesty: **pass**. Customer still does not get **$2,499 / $5,499**.

That is the bug this prompt is for.

---

## WHY IT STILL FAILS

Pinned path is `Agent.researchPinned` in `server/internal/research/agent.go`:
fetch **only** `Request.URLs` → extract claims → **verify quotes against
fetched text** → synthesise.

For Mac Studio:

1. Operator pins the **marketing** URL (`/mac-studio`), which is the natural
   Knowledge link.
2. Fetcher returns HTML **with no `$`**.
3. Extractor may still *claim* a price from model memory.
4. Verifier **drops** that claim because the quote is not on the page
   (the live “dropped a claim…” line).
5. Brief has no amounts → #106 drafter correctly says “not listed”.

The prices **are** on Apple, one hop from the pinned page:

- Newsroom:
  `https://www.apple.com/newsroom/2026/08/apple-introduces-new-mac-studio-with-m5-max-and-m5-ultra/`
  (“Mac Studio with M5 Max starts at $2,499 … M5 Ultra starts at $5,499”)
- Shop: `https://www.apple.com/shop/buy-mac/mac-studio` (often JS; HTML
  fetch may still lack `$`)

So: **pinned landing page is a shell; the figure is on a same-host child
page (or in JSON-LD / embedded data the fetcher ignores).**

This is the general product case, not Apple-only: vendor “product” URLs
are often marketing; list price is on `/buy`, `/pricing`, `/shop`, or a
press release linked from that page.

---

## NON-GOALS

- Do **not** remove or weaken `#106` amount checks.
- Do **not** turn pinned research into open-web search (HN, Google, etc.).
- Do **not** invent prices when no fetched text contains them.
- Do **not** tell the customer to visit a website instead of answering.
- Do **not** auto-send. Approve stays in the Mail UI.
- Do **not** rewrite Task Manager / chat / article writer.

---

## ALLOWED APPROACH (pick the smallest that passes evals)

Stay inside **pinned / same-product** fetch. Prefer in this order:

1. **Same-host follow-on (capped).** After reading a pinned URL, if
   `research_focus` / inbound mail is asking for price / cost / how much,
   **and** the fetched text has no money amount, collect a small set of
   http(s) links from that page that look like buy / shop / pricing /
   newsroom / press on the **same registrable host**. Fetch those too
   (hard cap, e.g. 2–3 extra URLs, still counting toward the existing
   URL cap). Merge into the same brief. Still no open web.
2. **Structured data on the pinned page.** If the HTML has JSON-LD
   `Offer` / `price` / `priceCurrency`, or obvious `__NEXT_DATA__` /
   price blobs, treat those strings as fetched text for extract + verify.
3. If after (1)+(2) there is still no amount: keep today’s honest
   “not listed … follow up” draft. That remains the correct fallback.

Do not follow tracking, unsubscribe, CDN image, or social share URLs
(reuse mail `knowledge.go` host/path filters where they fit).

---

## EVALS FIRST (must exist before the follow-on fetch)

Add tests that encode **this Mac Studio example**. Fakes, no live Gmail,
no live Apple (fixture HTML). Suggested file:
`server/internal/research/eval_test.go` and/or
`server/internal/service/mail_eval_test.go`.

| ID | Fixture | Assert |
|----|---------|--------|
| P1 | Pinned marketing HTML **with no `$`**, plus an on-page same-host newsroom/buy link whose fixture HTML **does** contain `$2,499` and `$5,499`. Inbound: “What is the price of the Mac Studio?” | Brief is usable; findings/quotes include `$2,499` and `$5,499`; search is **not** called |
| P2 | Same marketing HTML, **no** child links and **no** `$` | Brief has no money amounts (or warning); does not panic |
| P3 | Mail draft: findings contain `$2,499` / `$5,499` | Draft may quote those; must **not** quote `$1,999` |
| P4 | Mail draft: findings have **no** amounts (P2 brief) | Draft has **no** `$` / `£` / `€`; must not say “visit our website”; should follow up / not listed |
| P5 | `#106` cases still pass (`TestUnsupportedAmounts`, redraft, fallback, sourced amounts) | Unchanged contract |

P1/P3 fail until follow-on (or structured-data) lands. P2/P4/P5 must stay green.

If you add a fetcher fixture, keep it small: a fake `/mac-studio` page with
a link to `/newsroom/...`, and a fake newsroom page with the two starting
prices in visible text.

---

## CODE TOUCHPOINTS (read before editing)

| Area | Path |
|------|------|
| Pinned fetch | `server/internal/research/agent.go` `researchPinned` / `read` / `verify` |
| URL extract / tracking filter | `server/internal/mail/knowledge.go` |
| Mail research request | `server/internal/service/mail_service.go` `processThread` |
| Drafter + amount guard | `server/internal/mail/draft.go` |
| Existing mail evals | `server/internal/service/mail_eval_test.go` |
| Existing research evals | `server/internal/research/eval_test.go` |

Prefer extending `researchPinned` (or the fetcher) so **Mail, Article, and
Task Manager** all benefit when they pass pinned URLs. Do not special-case
Apple hosts in production code; the eval fixture may use apple.com-shaped
paths.

---

## ACCEPTANCE

- Eval table above is green.
- `#106` tests still green. `go test` for touched packages.
- With Knowledge = product marketing URL, inbound “what is the price of X?”,
  and a same-host child page that **does** list prices: draft quotes those
  figures (and only those).
- If no fetched text has a price: honest not-listed / follow-up; **no**
  invented `$1,999`; **no** “visit our website”.
- Nothing is sent without Approve.
- Manual re-check on int (optional): same `[js-test]` Mac Studio mail against
  Knowledge `https://www.apple.com/mac-studio/` — draft should quote
  **$2,499** / **$5,499** if follow-on reached newsroom, otherwise still
  honest not-listed (never `$1,999`).

---

## OUT OF SCOPE FOR THIS PR

- Scraping behind login / bot walls.
- A full headless browser for every pin (only if a tiny, existing fetch
  path already supports it and evals need it).
- Changing Gmail OAuth, Approve/send, or Task Manager field copy except a
  one-line hint if Knowledge help text must say “product or pricing URL”.
