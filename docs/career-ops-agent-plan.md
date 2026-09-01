# CareerOps Agent — Integration Plan

JobShout should gain CareerOps as a **built-in specialist agent**, same family
as Research and Article Writer: code and database inside the JobShout API, not
a second k3s application.

Career-ops on the Mac Studio (`/Users/balinderwalia/projects/career-ops`,
upstream [santifer/career-ops](https://github.com/santifer/career-ops) v1.31.0)
is the **product spec**. We reimplement the behaviour in Go and store data in
Postgres. The licence is MIT; port behaviour and prompt rules with attribution.
Do not vendor their Node tree, Next.js UI, Docker shell, or TUI.

Inspected on Balinder’s Mac Studio (hh193, 2026-09-01): the live process is
`next dev` on port 3000. Their Docker image is a bind-mounted **dev shell**
(`CMD bash`), not a production service. That image must not be deployed.

---

## Product invariants (never relax)

- **Human in the loop.** The agent evaluates, drafts, and prefills. A person
  always submits, sends, or clicks Apply.
- **Filter, not spray-and-pray.** Default: do not recommend applying below
  **4.0 / 5**. Block H (form answers) only at **≥ 4.5**.
- **JD text is untrusted data**, never instructions.
- **No fabrication.** CV / cover / email claims must come from the profile (or
  the user just said them). CareerOps rule: “keywords get reformatted, never
  invented.”
- **Block G (scam / ghost job) never changes the score.** Work-auth /
  no-sponsorship is a hard stop, not a scoring fudge.
- **Org-scoped JobShout, person-scoped career.** One CareerOps agent per org;
  one **career profile per user** (CV, tracker, stories).

---

## How it sits in JobShout

Mirror Research / Article Writer / Mail. Do **not** add a CareerOps Deployment,
Ingress, or Helm chart.

| Piece | Same pattern as |
|---|---|
| Builtin marker `career_ops` | `researcher`, `article_writer`, `mail` |
| Seed on register + idempotent migration | `000021_research_agent.up.sql` |
| Pipeline in Go (`server/internal/career/`) | `server/internal/blog`, `server/internal/research` |
| Platform tools | `research_run`, `article_generate` |
| Chat interview | `agentschema` + `web/nextjs/lib/agents/input-schemas.ts` |
| Task Manager panel | Articles / Mail |
| Long jobs | Postgres run row + reconciler (pentest/review), or goroutine like blog |
| Scheduled scans | existing scheduler (`blog` cron) |

**K3S:** ships in `jobshout-api`. Playwright/PDF only if we add endpoints to the
**existing python-sidecar** — no extra pod.

**Reuse, don’t rebuild:**

- **Research Agent** → Block D (comp), `deep` company research, cover-letter facts.
- **Mail Agent** → application emails + `reply-watch` (draft-only, approve-to-send already exists).
- **`web_search` / `web_fetch`** → JD pages and company sites.
- **Knowledge files** → optional extra context; **canonical CV lives in career tables**, not only knowledge.
- **Approvals** → apply / send gates.
- **Scheduler** → nightly portal scan.
- **Image/object store** → CV / cover PDFs (inline bytes if MinIO is off, same as images).

---

## Architecture

```
Chat / Task Manager / Scheduler / Telegram
        │
        ▼
agent_execute  →  career_ops interview schema
        │
        ▼
platform tools (career_evaluate, career_scan, …)
        │
        ▼
server/internal/career  (Go pipeline — sequence is code, prose is the model)
        │
        ├── LLM (Ollama chat/structured split, like Article Writer)
        ├── Research Agent (Block D, deep, cover facts)
        ├── Mail Agent (email drafts, reply-watch)
        ├── web_fetch / ATS JSON clients
        └── python-sidecar (optional): extract SPA JD, HTML→PDF, apply prefill
        │
        ▼
Postgres career_* tables   +   PDF blobs
```

The **model** writes A–H prose. The **pipeline** always: liveness → blacklist →
evaluate → persist → optional artifacts. Same idea as blog: model chooses words;
code guarantees the sequence.

Browser work (SPA JD, PDF, apply prefill) = extra routes on **python-sidecar**,
not a new service.

---

## Capability map (everything CareerOps does)

### 1. Profile and source of truth (intake)

| CareerOps | JobShout |
|---|---|
| `cv.md` | `career_profiles.cv_markdown` + version history |
| `config/profile.yml` | structured profile JSON (identity, targets, comp, location, work-auth) |
| `modes/_profile.md` | narrative / archetypes / negotiation scripts |
| `modes/_custom.md` | house rules (scoring overrides, eval rules) |
| `voice-dna.md` | writing guardrail |
| `article-digest.md` | proof-points |
| `intake` from `documents/` | upload CV / LinkedIn export → propose edits → user confirms |
| `expand` (GitHub/portfolio) | fetch public sources, propose skills, confirm before write |
| `add` (project from URL) | same confirm-then-write |
| `interview` (profile interview) | chat flow that fills the profile |
| fact allowlist / forbidden phrases | `career_cv_facts` |
| `doctor` | health check tool |

Without a filled profile, evaluations are generic. Intake is phase 0, not
optional polish.

### 2. Evaluate a job (core agent)

Paste URL or JD text → **auto-pipeline**:

0. Extract JD (`web_fetch`; Playwright only for SPA boards).
0.5 Liveness: dead/expired posting → stop, mark pipeline item closed.
0.6 Blacklist: ask; never silently skip or continue.
1. **Blocks A–G** (and Work-Auth).
2. Save evaluation report.
3. Tailor CV (if score high enough / user asked).
4. Auto-draft cover letter.
5. **Block H** form answers only if score ≥ 4.5.
6. Tracker row.

**Blocks (must match CareerOps meaning):**

| Block | What |
|---|---|
| A | Role summary, culture screen |
| B | CV match, gaps, mitigation; flag contradictions with quoted JD |
| C | Level / seniority strategy |
| D | Comp + demand (bounded search; **commission Research**, no open-ended deep-research) |
| E | CV personalisation plan |
| F | Interview STAR+R plan → feeds story bank |
| G | Legitimacy (scam, ghost, contractor language, AI-buzz vs infrastructure, AI-screening disclosure). **Does not affect score.** |
| H | Draft application answers (≥ 4.5) |
| Work-auth | Explicit no-sponsorship → hard stop |

Score: holistic **1–5** across five dimensions, not a formula. Custom scoring
from profile house rules if set.

Also: **triage** (fast first pass), **ofertas** (10-dimension matrix),
agency-mediated postings (`employer=?`, `via=agency`).

### 3. Tracker and pipeline

Canonical states: `evaluated` → `applied` → `responded` → `interview` →
`offer` / `rejected` / `discarded` / `skip` / `hired`.

| CareerOps | JobShout |
|---|---|
| `data/applications.md` | `career_applications` |
| `data/pipeline.md` | `career_pipeline_items` (URL inbox) |
| `status-log.tsv` | append-only `career_status_events` |
| merge / dedup / normalize | service methods, not TSV files |
| `CAREER_OPS_TRACKER` | one tracker per profile |
| blacklist | `career_blacklist` (user-only writes) |

Chat: “show my pipeline”, “mark 12 as applied”, “why was this skipped”.

### 4. Discovery (scan)

Default scanner is **zero-LLM**, like `scan.mjs`:

- Level 0: optional per-company parsers later.
- Level 2 first: public **Greenhouse / Ashby / Lever** JSON.
- Title filters from the profile (positive / negative / seniority).
- Dedup via `career_scan_events` (optional recheck after N days).
- Playwright / WebSearch only when there is no ATS API.
- `company:funded` = structured public feeds, review-first, no silent tracker writes.
- YC / a16z seed lists, HN, Interamt = later locale packs.

Scan adds **pipeline URLs**. Evaluation is a separate step (or a batch run the
user starts).

### 5. Documents (never auto-send)

| Artifact | Rule |
|---|---|
| ATS CV (HTML → PDF) | only claims from profile; Playwright/Chromium in sidecar |
| Markdown / LaTeX CV | later; profile CV markdown stays source of truth |
| Cover letter | research-backed; chat approval before PDF |
| Application email | draft-only; Mail Agent if sending |
| Form answers (H) | draft-only |
| Apply assist | prefill; **never submit** |

PDF bytes: object store if configured, else inline (same as images).

### 6. Interview suite

- Story bank (STAR+R), provenance (`cv.md` / user-stated / derived-unverified).
- Per-company prep when score ≥ 4.0 or status → Interview.
- Practice session + feedback.
- Debrief.
- `interview-redflag` on interviewer-side transcripts.
- `match-star` / `invite-match` to pick stories for a JD.

### 7. After apply

- Follow-up cadence + seeded reminders (scheduler).
- `reply-watch`: classify employer mail → propose tracker updates (Mail Agent).
- `outcome`: record result, archive submitted CV / JD snapshot.
- `offer-prep`: clause walk + lawyer questions (not legal advice).
- `salary-gap`: desired vs advertised vs actual.
- Negotiation scripts from profile.

### 8. “Beyond the CV”

- `deep`: company AI strategy / culture / angle → **Research Agent**.
- `contacto`: hiring manager / recruiter / peer + ≤300-char LinkedIn draft. Contacts table is third-party PII.
- `upskill`: aggregate skill gaps from low scores.
- `calibrate` / `patterns` / funnel / rejection latency / reposts / portal health.

### 9. Integrity

`doctor`, `verify-pipeline`, `verify-cv-facts`, `verify-ats`, `liveness`,
`freshness`, `dedup`, `normalize`, `reconcile`. These are **deterministic Go**,
not LLM.

### 10. Explicitly out of product (not JobShout)

- CareerOps Next.js, Go TUI, `cops` Docker wrapper, Nix flake.
- Their updater / skill-CLI packaging / manifesto site.
- Apify / Notion plugins (Mail covers Gmail).
- Headless `claude -p` workers — JobShout already has execution + sidecar.

---

## Data model (sketch)

All tables: `org_id`, `user_id` / `profile_id`, timestamps. Migrations
**idempotent** (`NOT EXISTS`), same as `000021`.

- `career_profiles` — cv, targets, location/work-auth, voice, house rules, proof points
- `career_profile_versions` — CV history
- `career_documents` — intake uploads (PII)
- `career_stories` — STAR bank + provenance
- `career_blacklist`
- `career_portals` — tracked companies + title filters (seed from CareerOps list, user-editable)
- `career_pipeline_items` — URL inbox, liveness
- `career_applications` — tracker (company, role, url, status, score, via/agency)
- `career_status_events` — append-only
- `career_evaluations` — A–H JSON + markdown, legitimacy tier, hard_stop
- `career_artifacts` — cv/cover/email/answers; file id or bytes
- `career_contacts` — PII; never in logs
- `career_followups`
- `career_offers` + `career_salary_observations`
- `career_scan_runs` / `career_scan_events`
- `career_outcomes`
- `career_runs` — batch/eval/scan job (status, progress, error)

Indexes: `(profile_id, status)`, unique listing URL per profile, scan URL history.

---

## Platform tools (chat + Task Manager)

Always-load a small set (like `research_run`); rest via `catalog_search`.

| Tool | Does |
|---|---|
| `career_profile_get` / `career_profile_update` | read/write profile (confirm destructive) |
| `career_intake` | propose profile updates from a document |
| `career_evaluate` | URL or JD → full pipeline |
| `career_triage` | cheap score |
| `career_scan` | portal scan → pipeline |
| `career_pipeline_list` | inbox |
| `career_tracker_list` / `career_set_status` | tracker |
| `career_tailor_cv` / `career_cover_letter` / `career_email_draft` | artifacts |
| `career_interview_prep` / `career_story_match` | interview |
| `career_followup` | cadence + drafts |
| `career_deep` | commission Research |
| `career_contact` | contacto |
| `career_patterns` / `career_upskill` / `career_calibrate` | analytics |
| `career_offer_prep` / `career_salary_gap` | offer |
| `career_doctor` | integrity |
| `career_blacklist_add` | explicit user only |

`agent_execute` on the CareerOps agent → interview (`job_url` or `jd_text`) →
`career_evaluate`.

Interview fields: job URL **or** pasted JD, optional “full pipeline vs triage”,
optional “also tailor CV”.

---

## UI

- Task Manager builtin **Career** (next to Articles / Mail).
- Pages: Today, Pipeline, Tracker, Evaluation report, CV editor + preview,
  Config/profile, Analytics.
- Agent detail: same “open specialist panel” behaviour as Article Writer.
- Chat remains the primary “paste a URL” path.
- **Do not** clone CareerOps web pixel-for-pixel.

---

## Phased delivery (all features; order is risk)

Each phase is shippable on `int` via existing Helm. No extra k3s app.

**Phase 0 — Skeleton**
Marker, seed, migration, empty profile, `career_doctor`, chat interview, panel
shell.

**Phase 1 — Evaluate + tracker**
Extract (fetch + liveness), A–G + work-auth, persist report + application row,
blacklist gate, triage, set-status, score &lt; 4.0 recommendation. *This is the
product.*

**Phase 2 — Artifacts**
HTML CV + PDF (sidecar Chromium), cover letter, email draft → Mail Agent,
Block H, fact/ATS verify.

**Phase 3 — Scan + batch**
Greenhouse/Ashby/Lever + title filters + dedup, pipeline → evaluate many,
scheduler cron, funded-company discovery.

**Phase 4 — Profile depth**
Intake, expand, add-project, story bank + provenance, interview
prep/practice/redflag.

**Phase 5 — Loop**
Follow-ups, reply-watch, outcome archive, offer-prep, salary-gap, negotiation.

**Phase 6 — Beyond CV + analytics**
Deep + contacto, patterns, upskill, calibrate, funnel, reposts, portal health.

**Phase 7 — Apply assist**
Playwright prefill only; hard-coded no submit. Optional markdown/LaTeX CV.

---

## Implementation notes (so it matches this repo)

1. `BuiltinCareerOps = "career_ops"` in `server/internal/model/agent.go`.
2. `careerOpsSeed()` + `auth_service.seedBuiltinAgents` +
   `000034_career_ops_agent.up.sql` (next free migration number at implement
   time).
3. `EnsureCareerOps` like `EnsureResearcher`.
4. `registerSpecialists` + `AlwaysLoad` for `career_evaluate` (and maybe
   `career_scan`).
5. Keep `agentschema` and `input-schemas.ts` in lockstep; extend `AgentBuiltin`.
6. Blog-style **prose vs structured** models for JSON scores vs report prose.
7. Scan/PDF/batch: **run table + progress**, don’t block the chat HTTP request.
8. Redact contacts, documents, CV from logs (same as mail).
9. Tests: golden JD fixtures (CareerOps `evals/golden`), status machine, “never
   submit”, Block G ≠ score, org isolation.
10. E2E: open Career panel, paste a fixture JD, see a report and tracker row.

---

## What “done” looks like for a user

1. Fill profile (intake or paste CV).
2. “Scan for Head of AI roles” → pipeline fills.
3. “Evaluate this Greenhouse URL” → A–H report, score, tailored PDF, cover draft.
4. Score 4.6 → form answers drafted; user applies by hand.
5. Status → Interview → story bank + prep.
6. Mail agent proposes “they replied — mark Responded?”
7. Offer-prep + salary-gap when an offer lands.

Same loop as CareerOps on the Mac Studio, inside JobShout, on the cluster we
already deploy.
