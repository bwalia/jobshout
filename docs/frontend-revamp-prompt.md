# JobShout Frontend Revamp — Clean, Chat-First Workspace

> Execution prompt for revamping the JobShout frontend.

## Context

The frontend lives in `web/nextjs` (Next.js 14 app router, TypeScript, Tailwind,
shadcn/Radix components, Zustand, TanStack Query, lucide-react icons). Today the
app is a classic dashboard: a permanent left sidebar
(`components/layout/Sidebar.tsx`) with ~20 routes under `app/(app)/` —
dashboard, chat, agents, agents/pentest, agents/review, agent-board, sprints,
sessions, scheduler, projects, task-manager, articles, images, workflows,
plugins, skills, llm-providers, org-builder, marketplace, metrics, settings.

Revamp this into a clean, chat-first workspace where chat is the primary
point of contact and execution, and everything else is reachable through a
compact panel switcher. Reuse existing API clients, hooks, and data components
wherever possible — this is a UX/IA restructure, not a backend rewrite.

## Design philosophy: clean and focused, not stark

- Aim for clean and uncluttered — think Linear / ChatGPT — but NOT
  ultra-minimal or sterile. The UI should still feel alive and informative:
  status colors, progress indicators, counts, avatars, and helpful secondary
  text are welcome wherever they carry real information.
- Typography: **Sora** (variable) as the primary UI/body font — same as the
  diy-tax-return-uk project. Keep a mono font for logs, run output, and
  telemetry. A restrained type scale, but enough contrast between levels that
  hierarchy is obvious.
- One accent color on a neutral palette for brand moments; semantic colors
  (green/amber/red) for status everywhere status appears — task states, run
  states, schedule health. Don't reduce status to monochrome text.
- Hairline borders and subtle surface tints over heavy shadows and thick
  cards; generous whitespace, but keep lists dense enough to scan many items.
- Cut decoration that carries no information (gradients, glows, decorative
  dots), keep affordances that help (icons in menus, hover states, badges with
  counts, empty states with a clear next action).
- Subtle, fast motion (fades/slides); no bouncy animations.
- **Directional panel transitions**: when switching panels via the menu, the
  new panel doesn't appear instantly — it slides in smoothly from the bottom
  if the target sits below the current panel in the menu order, or from the
  top if it sits above (~300ms, ease-out, slight fade). No animation on
  initial load or plain chat navigation.

## Non-negotiables

- **Zero functionality loss.** Every capability that exists today — every
  page, form, action, agent feature, and setting across all ~20 current routes
  — must remain fully usable in the new IA. Features get relocated and
  consolidated, never dropped. Before deleting any old page, verify its
  functionality has a working home in the new layout (keep a migration
  checklist mapping old route → new location).
- **Easy to navigate and operate.** Anything should be reachable in at most
  two clicks (panel menu → panel, or one click inside a panel). Every screen
  makes its primary action obvious (one clear button, not buried in menus).
  Current location is always visible (highlighted menu item, panel title);
  destructive actions confirm; common flows (new chat, new task, run agent)
  never require hunting.

## Core layout

### 1. Chat is home

- `/` (and default post-login route) is the chat. It is the main execution
  surface: from chat the user can trigger any agent (PR review bot, pentest
  bot, article bot, image bot, etc.), create tasks/projects, and see run
  results inline as compact message cards (status, progress, link to full
  detail).
- Chat messages that represent agent runs or tasks deep-link into the relevant
  panel (e.g. a pentest run card opens that run's detail inside the Task
  Manager panel).

### 2. Left sidebar = chat sidebar

- The left sidebar is dedicated to chat: a "New chat" button at the top and the
  chat history list below it (grouped by recency, renamable, deletable,
  searchable). Reuse the existing sessions data for history.
- Keep it visually quiet: plain text rows, no icons per chat, active chat
  marked by background tint only. Collapsible.
- **Sidebar footer**: at the bottom of the sidebar sits the user profile
  (avatar + name, opens account menu: profile, workspace, sign out) and a
  **theme toggle** (dark/light). Both remain accessible in the collapsed
  state (avatar + icon stacked).

### 3. Panel switcher (menu button at top of sidebar)

- At the very top of the left sidebar sits a prominent hamburger/menu button —
  comfortably large (~40px hit area), **opened on hover** (with a short close
  delay so the pointer can travel into the flyout without it snapping shut);
  click also toggles it, Escape and outside click close it, and it is fully
  keyboard accessible. It opens a flyout listing all app panels — an icon +
  label list, generous row height, no descriptions or grouping headers.
  Selecting one opens that panel as the main content area (chat sidebar stays
  put so the user can jump back to chat anytime); the active panel is
  highlighted in the menu.
- Panels to include, in order (Chat is always the first menu entry):
  1. **Dashboard** — directly after Chat in the menu. The at-a-glance home
     for everything: merge today's `dashboard` + `metrics` into this single
     overview panel (active agents, running tasks, recent runs, key metrics).
  2. **Task Board** — kanban of ALL tasks across projects: columns by
     status/progress, each card shows only title, assigned agent, and project.
     Clicking a task opens a detail drawer: full description, timeline, agent
     logs/output, linked chat messages, artifacts. Merge today's `agent-board`
     + `tasks` views into this.
  3. **Task Manager** — the single control center for all agents and projects.
     Master rail on the left lists **Projects** (with live task counts) and
     **Agents**; the detail area switches based on what's selected:
     - **Project selected** → that project's task list with a prominent "New
       task" action (inline form or dialog: title, assign to agent, priority).
       Tasks show assigned agent, status, and last update. Create and manage
       projects from here too ("New project" in the panel header).
     - **Agent selected** → agent detail: a "Run agent" action plus full run
       history and per-run detail (see below).
     - Execute any agent from here: PR review bot, pentest bot, article bot,
       image bot, and any future bots — one consistent "run agent" flow (pick
       agent → configure → run).
     - Embed each agent's full functionality that currently lives on separate
       pages: pentest run history + detailed run results (currently
       `agents/pentest`, `PentestRunsList`, `PentestRunResult`), review bot
       history + details (`agents/review`, `ReviewRunsList`,
       `ReviewRunResult`), articles, images, etc. Use a master-detail layout
       inside this panel (agent list on the left, history + detail on the
       right) — do NOT keep separate top-level pages per agent.
  4. **Scheduler** — existing scheduler functionality.
  5. **Sprints** — existing sprints functionality.
  6. Other logical panels, consolidated aggressively: Workflows, Org Builder,
     Marketplace, Plugins & Skills (merge), LLM Providers, Settings.
- Kill the current multi-section nav: no separate top-level entries for
  individual agents.

## Behavior & polish

- Routing: keep URL-addressable panels (e.g. `/panel/task-board`,
  `/panel/task-manager?project=X&agent=pentest&run=Y`) so deep links and
  refresh work; chat sessions at `/chat/[sessionId]` or `/?session=`.
- Migrate old routes with redirects (e.g. `/agents/pentest` → task manager
  panel with pentest selected).
- State: Zustand for UI state (active panel, sidebar collapse, open task
  drawer); React Query for all server data as today.
- Responsive: on mobile the chat sidebar becomes a sheet/drawer; the panel
  switcher becomes a tappable menu.
- Theming: real light and dark themes driven by design tokens, toggled from
  the sidebar footer and persisted (next-themes is already a dependency).
  Strip existing gradients, glows (`shadow-signal`), and decorative dots.
- Fonts: load Sora (variable) as `--font-sans`/body; keep a mono font for
  logs and telemetry.
- Keyboard: Cmd+K command palette to jump to any panel/chat; Cmd+N new chat.
- Accessibility: the panel menu must be fully keyboard operable (focus trap,
  arrow keys, Escape to close).

## Deliverables

1. New app shell (chat sidebar with profile + theme footer, click-to-open
   panel menu, panel container) replacing the current `Sidebar`/`Topbar`
   layout.
2. Chat page upgraded to execution hub with compact agent-run message cards.
3. Task Board panel with task detail drawer.
4. Task Manager panel absorbing all agent pages (pentest, review, articles,
   images): project task lists with task creation, plus per-agent
   history/detail views.
5. Scheduler, Sprints, and consolidated remaining panels wired in.
6. Redirects from old routes; remove dead pages/nav only after confirming
   their functionality lives in the new layout (per the migration checklist);
   all lint/type checks passing.

Work incrementally: shell first, then chat, then Task Board, then Task
Manager, then remaining panels. Apply the design system (Sora, tokens, light +
dark) from the first commit — don't restyle at the end.
