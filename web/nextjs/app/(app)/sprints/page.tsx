"use client";

import { useMemo, useState } from "react";
import { Plus, Target, CalendarDays, Users, CheckCircle2, AlertCircle, Loader2 } from "lucide-react";
import {
  useSprints,
  useSprint,
  useCreateSprint,
  useUpdateSprintStatus,
} from "@/lib/hooks/useSprints";
import type {
  Sprint,
  SprintDetail,
  SprintJob,
  SprintStatus,
} from "@/lib/api/sprints";
import { cn } from "@/lib/utils/cn";

// JIRA-style sprint board: list of sprints on the left rail, selected sprint
// detail on the right with jobs grouped by lifecycle phase.

export default function SprintsPage() {
  const { data: sprints = [], isLoading: sprintsLoading } = useSprints();
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // Auto-select the first sprint once data arrives so the right pane never
  // shows an empty placeholder when content exists.
  const effectiveId =
    selectedId ?? (sprints.length > 0 ? sprints[0].id : null);

  const { data: detail, isLoading: detailLoading } = useSprint(effectiveId ?? undefined);

  return (
    <div className="flex h-full min-h-[calc(100vh-8rem)] flex-col gap-6">
      <header>
        <h1 className="font-display text-2xl font-semibold tracking-tight text-foreground">
          Sprints
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Plan, run, and review sprint-style iterations of multi-agent work.
        </p>
      </header>

      <div className="grid flex-1 grid-cols-1 gap-6 lg:grid-cols-[280px_minmax(0,1fr)]">
        <SprintList
          sprints={sprints}
          loading={sprintsLoading}
          selectedId={effectiveId}
          onSelect={setSelectedId}
        />
        <SprintDetailPane detail={detail ?? null} loading={detailLoading} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sprint list (left rail)
// ---------------------------------------------------------------------------

function SprintList({
  sprints,
  loading,
  selectedId,
  onSelect,
}: {
  sprints: Sprint[];
  loading: boolean;
  selectedId: string | null;
  onSelect: (id: string) => void;
}) {
  const [showCreate, setShowCreate] = useState(false);

  return (
    <aside className="flex flex-col gap-3 rounded-lg border border-border bg-card">
      <div className="flex items-center justify-between border-b border-border px-3 py-2.5">
        <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Sprints
        </span>
        <button
          type="button"
          onClick={() => setShowCreate(true)}
          className="inline-flex items-center gap-1 rounded-md bg-primary px-2 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90"
        >
          <Plus className="h-3.5 w-3.5" />
          New
        </button>
      </div>

      <div className="flex-1 space-y-1 overflow-y-auto px-2 pb-2 scrollbar-thin">
        {loading ? (
          <div className="space-y-1">
            {Array.from({ length: 4 }).map((_, i) => (
              <div key={i} className="h-12 animate-pulse rounded bg-muted" />
            ))}
          </div>
        ) : sprints.length === 0 ? (
          <div className="px-2 py-6 text-center text-2xs text-muted-foreground">
            No sprints yet. Create one to start planning.
          </div>
        ) : (
          sprints.map((s) => (
            <button
              key={s.id}
              type="button"
              onClick={() => onSelect(s.id)}
              className={cn(
                "flex w-full flex-col items-start gap-1 rounded-md border-l-2 px-2.5 py-2 text-left transition-colors",
                selectedId === s.id
                  ? "border-primary bg-accent"
                  : "border-transparent hover:bg-muted"
              )}
            >
              <span className="line-clamp-1 text-sm font-medium text-foreground">
                {s.name}
              </span>
              <SprintStatusBadge status={s.status} />
            </button>
          ))
        )}
      </div>

      {showCreate ? (
        <CreateSprintInline onDone={() => setShowCreate(false)} />
      ) : null}
    </aside>
  );
}

// ---------------------------------------------------------------------------
// Sprint detail pane (right)
// ---------------------------------------------------------------------------

function SprintDetailPane({
  detail,
  loading,
}: {
  detail: SprintDetail | null;
  loading: boolean;
}) {
  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border border-border bg-card text-sm text-muted-foreground">
        <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Loading sprint…
      </div>
    );
  }
  if (!detail) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border border-dashed border-border bg-card text-sm text-muted-foreground">
        Select a sprint to see its board.
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-5">
      <SprintHeader detail={detail} />
      <SprintAgents agents={detail.agents} />
      <SprintBoard jobs={detail.jobs} />
    </div>
  );
}

function SprintHeader({ detail }: { detail: SprintDetail }) {
  const updateStatus = useUpdateSprintStatus();

  return (
    <header className="rounded-lg border border-border bg-card p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <h2 className="font-display text-xl font-semibold text-foreground">
            {detail.name}
          </h2>
          {detail.goal ? (
            <p className="mt-1 flex items-start gap-1.5 text-sm text-muted-foreground">
              <Target className="mt-0.5 h-3.5 w-3.5 shrink-0 text-status-progress" />
              <span>{detail.goal}</span>
            </p>
          ) : null}
        </div>
        <div className="flex shrink-0 flex-col items-end gap-2">
          <SprintStatusBadge status={detail.status} />
          <SprintStatusActions
            status={detail.status}
            onTransition={(next) =>
              updateStatus.mutate({ id: detail.id, status: next })
            }
          />
        </div>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat label="Total" value={detail.stats.total_jobs} />
        <Stat
          label="In flight"
          value={detail.stats.in_flight_jobs}
          tone="progress"
        />
        <Stat
          label="Completed"
          value={detail.stats.completed_jobs}
          tone="done"
        />
        <Stat label="Failed" value={detail.stats.failed_jobs} tone="blocked" />
      </div>

      {detail.start_at || detail.end_at ? (
        <div className="mt-3 flex items-center gap-2 text-xs text-muted-foreground">
          <CalendarDays className="h-3.5 w-3.5" />
          <span>
            {formatRange(detail.start_at, detail.end_at)}
          </span>
        </div>
      ) : null}
    </header>
  );
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone?: "progress" | "done" | "blocked";
}) {
  const dot =
    tone === "progress"
      ? "bg-status-progress"
      : tone === "done"
      ? "bg-status-done"
      : tone === "blocked"
      ? "bg-status-blocked"
      : "bg-status-idle";
  return (
    <div className="rounded-md border border-border bg-background px-3 py-2">
      <div className="flex items-center gap-1.5 text-2xs uppercase tracking-wide text-muted-foreground">
        <span className={cn("h-1.5 w-1.5 rounded-full", dot)} />
        {label}
      </div>
      <div className="mt-1 font-display text-xl font-semibold text-foreground">
        {value}
      </div>
    </div>
  );
}

function SprintStatusActions({
  status,
  onTransition,
}: {
  status: SprintStatus;
  onTransition: (next: SprintStatus) => void;
}) {
  // Each status transitions to the obvious next state. JIRA exposes more, but
  // four is enough to ship a working iteration loop.
  if (status === "planning") {
    return (
      <button
        type="button"
        onClick={() => onTransition("active")}
        className="rounded-md border border-primary bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary hover:bg-primary/20"
      >
        Start sprint
      </button>
    );
  }
  if (status === "active") {
    return (
      <button
        type="button"
        onClick={() => onTransition("completed")}
        className="rounded-md border border-status-done/40 bg-status-done/10 px-2.5 py-1 text-xs font-medium text-status-done hover:bg-status-done/20"
      >
        Complete sprint
      </button>
    );
  }
  return null;
}

function SprintAgents({ agents }: { agents: SprintDetail["agents"] }) {
  if (agents.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border bg-card px-4 py-3 text-xs text-muted-foreground">
        <Users className="mr-1.5 inline h-3.5 w-3.5" /> No agents assigned to
        this sprint yet.
      </div>
    );
  }
  return (
    <section className="rounded-lg border border-border bg-card p-3">
      <h3 className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        <Users className="h-3.5 w-3.5" /> Squad
      </h3>
      <ul className="flex flex-wrap gap-2">
        {agents.map((a) => (
          <li
            key={`${a.agent_id}-${a.role_label}`}
            className="flex items-center gap-2 rounded-full border border-border bg-background px-2 py-1 text-xs"
          >
            <span className="flex h-5 w-5 items-center justify-center rounded-full bg-accent text-2xs font-semibold text-accent-foreground">
              {a.name.charAt(0).toUpperCase()}
            </span>
            <span className="font-medium text-foreground">{a.name}</span>
            <span className="rounded bg-muted px-1.5 py-0.5 text-2xs font-medium uppercase tracking-wide text-muted-foreground">
              {a.role_label}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

// ---------------------------------------------------------------------------
// Sprint board: jobs grouped by lifecycle column
// ---------------------------------------------------------------------------

const JOB_COLUMNS: { key: string; label: string; matcher: (j: SprintJob) => boolean; tone: string }[] = [
  {
    key: "pending",
    label: "To Do",
    matcher: (j) => j.status === "pending",
    tone: "bg-status-todo",
  },
  {
    key: "planning",
    label: "Planning",
    matcher: (j) => j.status === "planning",
    tone: "bg-status-todo",
  },
  {
    key: "executing",
    label: "Executing",
    matcher: (j) => j.status === "executing" || j.status === "reviewing",
    tone: "bg-status-progress",
  },
  {
    key: "completed",
    label: "Completed",
    matcher: (j) => j.status === "completed",
    tone: "bg-status-done",
  },
  {
    key: "failed",
    label: "Failed",
    matcher: (j) => j.status === "failed",
    tone: "bg-status-blocked",
  },
];

function SprintBoard({ jobs }: { jobs: SprintJob[] }) {
  const grouped = useMemo(() => {
    return JOB_COLUMNS.map((c) => ({ ...c, jobs: jobs.filter(c.matcher) }));
  }, [jobs]);

  return (
    <section className="flex flex-1 gap-3 overflow-x-auto pb-4 scrollbar-thin">
      {grouped.map((col) => (
        <div
          key={col.key}
          className="flex w-72 shrink-0 flex-col rounded-lg border border-border bg-secondary/40"
        >
          <header className="flex items-center justify-between border-b border-border px-3 py-2.5">
            <div className="flex items-center gap-2">
              <span className={cn("h-1.5 w-1.5 rounded-full", col.tone)} />
              <span className="text-xs font-semibold uppercase tracking-wide text-foreground">
                {col.label}
              </span>
            </div>
            <span className="rounded bg-background px-2 py-0.5 text-2xs font-semibold text-muted-foreground">
              {col.jobs.length}
            </span>
          </header>
          <div className="flex flex-1 flex-col gap-2 p-2">
            {col.jobs.length === 0 ? (
              <div className="px-3 py-4 text-center text-2xs text-muted-foreground">
                Empty
              </div>
            ) : (
              col.jobs.map((j) => <JobCard key={j.id} job={j} />)
            )}
          </div>
        </div>
      ))}
    </section>
  );
}

function JobCard({ job }: { job: SprintJob }) {
  return (
    <article className="rounded-md border border-border bg-card p-3 shadow-card">
      <p className="line-clamp-3 text-xs text-foreground/90">{job.task_prompt}</p>
      <footer className="mt-2 flex items-center justify-between text-2xs text-muted-foreground">
        <span>
          {job.iterations}/{job.max_review} cycles
        </span>
        {job.status === "completed" && job.approved ? (
          <span className="flex items-center gap-1 text-status-done">
            <CheckCircle2 className="h-3 w-3" /> approved
          </span>
        ) : job.status === "failed" ? (
          <span className="flex items-center gap-1 text-status-blocked">
            <AlertCircle className="h-3 w-3" /> failed
          </span>
        ) : null}
      </footer>
    </article>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function SprintStatusBadge({ status }: { status: SprintStatus }) {
  const map: Record<SprintStatus, { label: string; cls: string }> = {
    planning: { label: "Planning", cls: "bg-status-todo/15 text-status-todo" },
    active: { label: "Active", cls: "bg-status-progress/15 text-status-progress" },
    completed: { label: "Completed", cls: "bg-status-done/15 text-status-done" },
    cancelled: { label: "Cancelled", cls: "bg-status-blocked/15 text-status-blocked" },
  };
  const { label, cls } = map[status] ?? {
    label: status,
    cls: "bg-muted text-muted-foreground",
  };
  return (
    <span
      className={cn(
        "inline-flex items-center rounded px-1.5 py-0.5 text-2xs font-semibold uppercase tracking-wide",
        cls
      )}
    >
      {label}
    </span>
  );
}

function CreateSprintInline({ onDone }: { onDone: () => void }) {
  const [name, setName] = useState("");
  const [goal, setGoal] = useState("");
  const create = useCreateSprint();

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (!name.trim()) return;
        create.mutate(
          { name: name.trim(), goal: goal.trim() || undefined },
          {
            onSuccess: () => {
              setName("");
              setGoal("");
              onDone();
            },
          }
        );
      }}
      className="border-t border-border p-2.5"
    >
      <input
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Sprint name"
        className="mb-2 h-8 w-full rounded border border-input bg-background px-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      <input
        value={goal}
        onChange={(e) => setGoal(e.target.value)}
        placeholder="Goal (optional)"
        className="mb-2 h-8 w-full rounded border border-input bg-background px-2 text-xs placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={create.isPending || !name.trim()}
          className="flex-1 rounded-md bg-primary px-2 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-60"
        >
          {create.isPending ? "Creating…" : "Create"}
        </button>
        <button
          type="button"
          onClick={onDone}
          className="rounded-md border border-border px-2 py-1 text-xs"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}

function formatRange(start?: string, end?: string): string {
  const fmt = (s?: string) =>
    s ? new Date(s).toLocaleDateString(undefined, { month: "short", day: "numeric" }) : "?";
  if (!start && !end) return "";
  return `${fmt(start)} → ${fmt(end)}`;
}
