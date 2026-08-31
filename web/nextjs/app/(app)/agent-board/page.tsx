"use client";

import { useMemo } from "react";
import {
  Bot,
  Brain,
  Hammer,
  Eye,
  AlertTriangle,
  Coffee,
  GitPullRequest,
} from "lucide-react";
import {
  useAgentBoard,
} from "@/lib/hooks/useAgentBoard";
import type {
  AgentBoardEntry,
  AgentActivity,
} from "@/lib/api/agent-board";
import { cn } from "@/lib/utils/cn";

// ---------------------------------------------------------------------------
// Column config
// ---------------------------------------------------------------------------

interface ColumnDef {
  key: AgentActivity;
  label: string;
  // Compact JIRA-style helper text under the column header.
  hint: string;
  icon: React.ElementType;
  // Tailwind classes that pull from the JIRA-aligned status palette in
  // tailwind.config.js / globals.css. We avoid hard-coded hex so light/dark
  // modes stay in sync.
  accent: string;
  dot: string;
}

const COLUMNS: ColumnDef[] = [
  {
    key: "idle",
    label: "Idle",
    hint: "Available for work",
    icon: Coffee,
    accent: "border-status-idle/40",
    dot: "bg-status-idle",
  },
  {
    key: "planning",
    label: "Planning",
    hint: "Drafting an execution plan",
    icon: Brain,
    accent: "border-status-todo/40",
    dot: "bg-status-todo",
  },
  {
    key: "executing",
    label: "Executing",
    hint: "Running the plan",
    icon: Hammer,
    accent: "border-status-progress/40",
    dot: "bg-status-progress",
  },
  {
    key: "reviewing",
    label: "Reviewing",
    hint: "Checking the result",
    icon: Eye,
    accent: "border-status-review/40",
    dot: "bg-status-review",
  },
  {
    key: "publishing",
    label: "Publishing",
    hint: "Shipping work to a real system",
    icon: GitPullRequest,
    accent: "border-status-done/40",
    dot: "bg-status-done",
  },
  {
    key: "failed",
    label: "Failed",
    hint: "Last job errored",
    icon: AlertTriangle,
    accent: "border-status-blocked/40",
    dot: "bg-status-blocked",
  },
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function AgentBoardView({
  hideHeader = false,
}: {
  hideHeader?: boolean;
}) {
  const { data: entries, isLoading, isError } = useAgentBoard();

  // Group agents by activity once per render.
  const grouped = useMemo(() => {
    const out: Record<AgentActivity, AgentBoardEntry[]> = {
      idle: [],
      planning: [],
      executing: [],
      reviewing: [],
      publishing: [],
      failed: [],
    };
    for (const e of entries ?? []) {
      out[e.activity]?.push(e);
    }
    return out;
  }, [entries]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-6">
      {hideHeader ? (
        <div className="flex justify-end">
          <LiveIndicator loading={isLoading} />
        </div>
      ) : (
        <header className="flex items-start justify-between gap-4">
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-tight text-foreground">
              Agent Board
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Live view of every agent and what they&apos;re currently doing.
              Updates every five seconds.
            </p>
          </div>
          <LiveIndicator loading={isLoading} />
        </header>
      )}

      {isError ? (
        <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
          Failed to load agent board. The backend may be unreachable.
        </div>
      ) : null}

      {/* Board */}
      <div className="flex flex-1 gap-4 overflow-x-auto pb-4 scrollbar-thin">
        {COLUMNS.map((col) => (
          <BoardColumn
            key={col.key}
            column={col}
            entries={grouped[col.key]}
            loading={isLoading}
          />
        ))}
      </div>
    </div>
  );
}

export default function AgentBoardPage() {
  return <AgentBoardView />;
}

// ---------------------------------------------------------------------------
// Live indicator
// ---------------------------------------------------------------------------

function LiveIndicator({ loading }: { loading: boolean }) {
  return (
    <div className="flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1.5 text-xs font-medium text-muted-foreground">
      <span
        className={cn(
          "h-1.5 w-1.5 rounded-full",
          loading ? "bg-status-idle" : "bg-status-done animate-pulse"
        )}
        aria-hidden="true"
      />
      {loading ? "Loading…" : "Live"}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Column
// ---------------------------------------------------------------------------

function BoardColumn({
  column,
  entries,
  loading,
}: {
  column: ColumnDef;
  entries: AgentBoardEntry[];
  loading: boolean;
}) {
  const Icon = column.icon;
  return (
    <section
      className={cn(
        "flex w-72 shrink-0 flex-col rounded-lg border bg-secondary/40",
        column.accent
      )}
    >
      <header className="flex items-center justify-between border-b border-border px-3 py-2.5">
        <div className="flex items-center gap-2">
          <span className={cn("h-1.5 w-1.5 rounded-full", column.dot)} />
          <Icon className="h-4 w-4 text-muted-foreground" />
          <span className="text-xs font-semibold uppercase tracking-wide text-foreground">
            {column.label}
          </span>
        </div>
        <span className="rounded bg-background px-2 py-0.5 text-2xs font-semibold text-muted-foreground">
          {entries.length}
        </span>
      </header>

      {/* Helper text only — JIRA omits the hint inside the column to save
          vertical space, but our users don't have multi-day muscle memory yet
          so we keep a single subtle line. */}
      <p className="px-3 pt-2 text-2xs text-muted-foreground">{column.hint}</p>

      <div className="flex flex-1 flex-col gap-2 p-2 overflow-y-auto scrollbar-thin">
        {loading
          ? Array.from({ length: 3 }).map((_, i) => <CardSkeleton key={i} />)
          : entries.length === 0
          ? (
            <div className="flex flex-1 items-center justify-center px-3 py-8 text-center text-2xs text-muted-foreground">
              No agents here.
            </div>
          )
          : entries.map((e) => <AgentBoardCard key={e.agent_id} entry={e} />)}
      </div>
    </section>
  );
}

// ---------------------------------------------------------------------------
// Card
// ---------------------------------------------------------------------------

function AgentBoardCard({ entry }: { entry: AgentBoardEntry }) {
  return (
    <article className="group rounded-md border border-border bg-card p-3 shadow-card transition-shadow hover:shadow-card-hover">
      <div className="flex items-start gap-2.5">
        <Avatar entry={entry} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <h3 className="truncate text-sm font-semibold text-foreground">
              {entry.name}
            </h3>
          </div>
          <p className="truncate text-xs text-muted-foreground">{entry.role}</p>
        </div>
      </div>

      {entry.current_job_prompt ? (
        <p className="mt-2.5 line-clamp-2 rounded bg-muted/60 px-2 py-1.5 text-2xs text-foreground/80">
          {entry.current_job_prompt}
        </p>
      ) : null}

      <footer className="mt-2 flex items-center justify-between text-2xs text-muted-foreground">
        {entry.job_role ? (
          <span className="rounded bg-accent px-1.5 py-0.5 font-medium text-accent-foreground capitalize">
            {entry.job_role}
          </span>
        ) : (
          <span />
        )}
        {entry.last_active_at ? (
          <time dateTime={entry.last_active_at} title={entry.last_active_at}>
            {formatRelative(entry.last_active_at)}
          </time>
        ) : null}
      </footer>
    </article>
  );
}

function Avatar({ entry }: { entry: AgentBoardEntry }) {
  if (entry.avatar_url) {
    return (
      // Keep avatar as a plain img so we don't depend on next/image domain config.
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={entry.avatar_url}
        alt={entry.name}
        className="h-8 w-8 shrink-0 rounded-full object-cover"
      />
    );
  }
  // Fallback: initial in a tinted square — JIRA avatar style.
  const initial = entry.name.trim().charAt(0).toUpperCase() || "A";
  return (
    <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-accent text-xs font-semibold text-accent-foreground">
      {initial}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function CardSkeleton() {
  return (
    <div className="rounded-md border border-border bg-card p-3">
      <div className="flex items-center gap-2.5">
        <div className="h-8 w-8 animate-pulse rounded-md bg-muted" />
        <div className="flex-1 space-y-1.5">
          <div className="h-3 w-2/3 animate-pulse rounded bg-muted" />
          <div className="h-2 w-1/2 animate-pulse rounded bg-muted" />
        </div>
      </div>
    </div>
  );
}

/** "5m ago" / "2h ago" / "yesterday" — small, locale-independent. */
function formatRelative(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const diffMs = Date.now() - then;
  const diffSec = Math.max(0, Math.round(diffMs / 1000));
  if (diffSec < 60) return `${diffSec}s ago`;
  const diffMin = Math.round(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffH = Math.round(diffMin / 60);
  if (diffH < 24) return `${diffH}h ago`;
  const diffD = Math.round(diffH / 24);
  return `${diffD}d ago`;
}
