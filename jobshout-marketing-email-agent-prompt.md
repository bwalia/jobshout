# Build the Marketing Email Writer agent

## Goal

Add a JobShout AI agent, **Marketing Email Writer**, that writes a marketing email from a short brief,
renders it as a production-quality HTML email, and **publishes it live to OpsAPI immediately** — the
same way the Article Writer files posts — so the Workstation website serves it as the email's
**browser version** ("View this email in your browser").

Flow: JobShout (int.jobshout.co.uk) → OpsAPI CMS (int-opsapi.workstation.co.uk, namespace `workstation`)
→ `https://int.workstation.co.uk/email/<slug>` (website side is built separately in
`workstation-website`; the contract is below). Prod is the same with the prod hosts.

Read `CLAUDE.md` and `.claude/rules/agent-modules.md` first and follow them.

## Patterns to copy (don't reinvent)

- Agent module + registration: `server/internal/agentmodule/registry.go` (`Module`),
  `server/internal/agentmodules/register.go`. Model a new package on
  `server/internal/jobshoutcomwriter/module.go` (own tab, `StayOnTab`, `AutoPublish`) and
  `server/internal/blog/module.go` (schema, seed, system prompt).
- Launch fields live once in Go `agentschema`; the web reads `GET /api/v1/agent-schemas`. Register on all
  three surfaces (profile, chat `agent_execute`, tab via `AGENT_CLIENTS` in
  `web/nextjs/lib/agents/tab-clients.tsx`). Do **not** add switches to `tasklaunch/launch.go`,
  `chatagent/prompt.go` or `platformtools/execute.go`.
- OpsAPI client: `server/internal/integration/adapters/opsapi/cms.go` (Bearer `opsk_…` +
  `X-Namespace-Slug`). Per-agent destination: `service/blog_destination.go`,
  `model/writer_settings.go` (`writer_destination`).
- Model selection: `AGENT_PROVIDER__MARKETING_EMAIL_WRITER` / `AGENT_MODEL__MARKETING_EMAIL_WRITER`
  (`config/agentmodel.go`) plus a per-launch `model` field. Use the structured model for JSON.
- Cover/hero image: reuse the blog illustrator (`blog/illustrate.go`, `BLOG_COVER_IMAGES`); images
  must be absolute `https://` URLs.
- Scheduling: `scheduler/runner.go` — add a dispatch case so emails can be scheduled like articles.

## Agent definition

- Builtin `marketing_email_writer`, label "Marketing Email Writer", tab slug `emails`, `StayOnTab`.
- Launch schema: `brief` (required: what the email is about), `audience`, `goal` (announce | promote |
  nurture | newsletter), `primary_cta_label`, `primary_cta_url` (https only), `products` (optional
  list of shop URLs or slugs to feature), `tone`, `destination` (website field, as the article
  writer), `publish` (`live` default | `draft`), `model`.
  Never put a credential in the schema.

## Generation — the LLM never writes raw HTML

1. **Structured step** (structured model, tolerant decode with `flexibleString`): return JSON
   `{subject, preheader, title, slug_hint, hero: {headline, subline, image_prompt}, sections: [{type:
   "text"|"feature"|"product"|"quote"|"cta", heading, body_markdown, items[], cta_label, cta_url}],
   footer_note}`. Validate: subject ≤ 70 chars, preheader 40–110, every URL https, 2–6 sections.
   Retry once on invalid output with the validation errors.
2. **Render in Go** with `html/template` (auto-escaping) into a single email layout,
   `server/internal/marketingemail/templates/email.html.tmpl`:
   - `<!doctype html>`, `lang="en"`, `<meta charset>`, viewport,
     `<meta name="color-scheme" content="light dark">`, `<title>` = subject.
   - Hidden preheader span; 600px centred **table** layout; **inline CSS only** (a small `<style>`
     block only for media queries/dark mode); bulletproof table buttons; web-safe font stack;
     every `<img>` has `alt`, width/height and `display:block`.
   - Workstation brand: navy `#0d0b21` header, pink `#e5165f` buttons, logo
     `https://www.workstation.co.uk/img/email/workstation-logo.png` (PNG — Gmail/Outlook don't show
     SVG; configurable via env `MARKETING_EMAIL_LOGO_URL`).
   - Footer: company name and address, link to `https://www.workstation.co.uk`, and a "Manage
     preferences / unsubscribe" link. The ESP fills that link, so render it from a configurable
     placeholder (env `MARKETING_EMAIL_UNSUBSCRIBE_URL`, default
     `https://www.workstation.co.uk/en/contact-us`). Do not leave literal `{{…}}` merge tags in the
     published HTML — the browser version must look finished.
   - Markdown in `body_markdown` → a small allowlist (paragraphs, bold, italic, links, lists). No
     scripts, iframes, forms or external CSS.
3. Hero image: illustrator from `hero.image_prompt` when cover images are enabled; otherwise no image
   (never a broken one).

## Publish to OpsAPI — CMS **pages**, not posts

Emails must not appear in the blog listing (`/en/docs/blogs` shows all published posts, and the blog
page restyles post HTML). Use the CMS **pages** API (opsapi `lapis/routes/cms-pages.lua`):

- Add `CreatePage`, `UpdatePage` and `FindPageBySlug` to the opsapi adapter:
  `POST /api/v2/cms/pages` and `PUT /api/v2/cms/pages/{uuid}`; for lookup, `GET /api/v2/cms/pages?search=<slug>`
  and exact-match on `slug`. Fields: `title`, `slug`, `excerpt` (= preheader), `content_html` (the
  full rendered document), `featured_image_url`, `status`, **`template: "email"`**,
  `show_in_nav: false`, `seo_title` (= subject), `seo_description` (= preheader).
- Slug: `kebab(slug_hint or title)`, max 80 chars, unique. On a re-run of the same email, **update** the
  existing page (upsert by slug) instead of creating a duplicate.
- `publish=live` → `status: "published"` straight away (this is the requested behaviour: no review
  gate). `publish=draft` → `status: "draft"`. Same key and scopes as posts (`cms` create/update/read).
  If update is forbidden (403), surface a clear error naming the missing `cms:update` scope — do not
  silently create a second page.
- Store on the run: page uuid, slug, status, subject, preheader and the **browser-version URL**.

## Browser-version URL (website contract)

`workstation-website` will serve `GET /email/{slug}`:
it fetches `GET {OPSAPI}/api/v2/public/cms/workstation/pages/{slug}` and returns `rendered_html` as
`text/html` **only when `template === "email"`**, with `X-Robots-Tag: noindex`.
So the agent reports:

| OpsAPI | Browser version |
|---|---|
| `int-opsapi.workstation.co.uk` | `https://int.workstation.co.uk/email/<slug>` |
| `opsapi.workstation.co.uk` | `https://www.workstation.co.uk/email/<slug>` |

Derive the site from the destination's OpsAPI base (config map, not string hacks), and allow an
override per destination (`config.site_url`).

## UI (`web/nextjs`, tab `emails`)

- Launch form from the schema; run list with subject, status, updated time.
- Run detail: a sandboxed `<iframe srcdoc>` preview at 600px and mobile width, subject and preheader,
  **Copy browser-version link**, **Copy HTML** (for pasting into the ESP), Open live page,
  Re-generate (upsert) and Unpublish (set `status: "archived"`).

## Tests and checks

- Go: renderer golden test (escaping of `<script>` and quotes in model text, 600px table, inline styles,
  no `{{` left, every img has alt); schema validation tests (http URL rejected, subject length);
  opsapi adapter tests with `httptest` (create, upsert by slug, 403 on update); module registration
  test. Build any credential-like fixtures at runtime (GitGuardian).
- Web: `cd web/nextjs && npm run typecheck && npm run lint`.
- Required checks must be green before merge: "Go build, vet & test", "Web typecheck & lint",
  "PR image check".

## Deploy and verify on int

1. Merge to master → int deploys. Ensure the int CMS key (`jobshout/int/cms` in WSL Vault) has
   `cms` create, update and read; restart `jobshout-api` if the key changes.
2. Run the agent on int.jobshout.co.uk with a real brief (for example: "Workstation AI shop is live —
   NVIDIA DGX Spark, Mac Studio M5 and Trade membership with an AI engineer on call").
3. Verify:
   `curl -s https://int-opsapi.workstation.co.uk/api/v2/public/cms/workstation/pages/<slug>` →
   `template: "email"`, `status` published; then open the browser-version URL once the website side is
   live. Check it in a 600px and a phone-width viewport, and paste the HTML into a test send (Gmail and
   Outlook).
4. Prod follows the normal Ring Promoter promotion (prod needs the password — do not bypass it).

## Done when

- The agent appears on its tab, in chat (`agent_execute`) and on agent profiles, and can be scheduled.
- A run publishes a live CMS page with `template: "email"` in under a few minutes and shows its
  browser-version URL.
- Re-running updates the same page; Unpublish archives it.
- Nothing is published to `/api/v2/cms/posts`, so the blog listing is unchanged.
