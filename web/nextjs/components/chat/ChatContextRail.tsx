"use client";

import { useMemo } from "react";
import Link from "next/link";
import {
  ArrowRight,
  BriefcaseBusiness,
  Coins,
  FileText,
  GitPullRequest,
  Image as ImageIcon,
  Mail,
  Search,
  ShieldCheck,
  Sparkles,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils/cn";
import { useAgentBoard } from "@/lib/hooks/useAgentBoard";
import { useAgents } from "@/lib/hooks/useAgents";
import { useTasks } from "@/lib/hooks/useTasks";
import { agentBoardHref, type AgentBoardEntry } from "@/lib/api/agent-board";
import type { ChatMessage, ChatResponse } from "@/lib/types/chat";

/**
 * The column beside the transcript.
 *
 * A chat page that centres one narrow column leaves half a desktop screen
 * empty. This fills it with the things a person actually asks for mid-chat:
 * what the agents are doing right now, what this conversation has cost, and
 * who else they can hand work to. Everything here is live API data — nothing
 * decorative.
 */
export function ChatContextRail({
  messages,
  onPick,
  liveModel,
  showSpecialists = true,
  className,
}: {
  messages: ChatMessage[];
  onPick: (prompt: string) => void;
  /** Model the server is using right now, ahead of anything in the history. */
  liveModel?: string | null;
  /** Off on the empty state, which already lists the roster in full. */
  showSpecialists?: boolean;
  className?: string;
}) {
  const board = useAgentBoard();
  const agentsQuery = useAgents({ per_page: 50 });
  // One row each — only the totals are read, so don't pull whole pages.
  const inProgress = useTasks({ status: "in_progress", per_page: 1 });
  const inReview = useTasks({ status: "review", per_page: 1 });
  const todo = useTasks({ status: "todo", per_page: 1 });

  const entries = board.data ?? [];
  const busy = entries.filter((e) => e.activity !== "idle");
  const idle = entries.length - busy.length;

  const spend = useMemo(() => totals(messages), [messages]);
  const servingModel = liveModel ?? spend.model;

  const specialists = useMemo(
    () =>
      !showSpecialists
        ? []
        : (agentsQuery.data?.data ?? [])
            .filter((a) => typeof a.metadata?.builtin === "string")
            .slice(0, 6),
    [agentsQuery.data, showSpecialists]
  );

  return (
    <aside
      aria-label="Workspace context"
      className={cn(
        "scrollbar-thin flex w-[320px] shrink-0 flex-col gap-5 overflow-y-auto border-l border-border py-4 pl-5 pr-4",
        className
      )}
    >
      <section>
        <RailHeading
          title="Agents at work"
          href="/panel/task-manager"
          hint={idle > 0 ? `${idle} idle` : undefined}
        />
        {board.isLoading ? (
          <RailSkeleton rows={2} />
        ) : busy.length === 0 ? (
          <p className="text-sm leading-6 text-muted-foreground">
            Nobody is running anything. Ask for a job below and it starts here.
          </p>
        ) : (
          <ul className="space-y-1.5">
            {busy.slice(0, 5).map((e) => (
              <li key={e.agent_id}>
                <BoardRow entry={e} />
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <RailHeading title="Your board" href="/panel/task-board" />
        <dl className="space-y-2">
          <Stat label="In progress" value={count(inProgress.data?.total)} />
          <Stat label="In review" value={count(inReview.data?.total)} />
          <Stat label="To do" value={count(todo.data?.total)} />
        </dl>
      </section>

      <section>
        <RailHeading title="This conversation" />
        <dl className="space-y-2">
          <Stat label="Turns" value={String(spend.turns)} />
          <Stat
            label="Tokens"
            value={spend.tokens > 0 ? spend.tokens.toLocaleString() : "—"}
          />
          <Stat
            label="Cost"
            value={spend.cost > 0 ? `$${spend.cost.toFixed(4)}` : "—"}
            icon={Coins}
          />
          {servingModel ? <Stat label="Model" value={servingModel} /> : null}
        </dl>
      </section>

      {specialists.length > 0 ? (
        <section>
          <RailHeading title="Hand it to a specialist" href="/panel/task-manager" />
          <ul className="space-y-1.5">
            {specialists.map((a) => {
              const key = String(a.metadata?.builtin ?? "");
              const Icon = BUILTIN_ICONS[key] ?? Sparkles;
              return (
                <li key={a.id}>
                  <button
                    type="button"
                    onClick={() => onPick(`Ask ${a.name} to `)}
                    className="group flex w-full items-center gap-2.5 rounded-lg px-2 py-2 text-left transition-colors hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-secondary text-muted-foreground transition-colors group-hover:bg-accent group-hover:text-primary">
                      <Icon className="h-4 w-4" />
                    </span>
                    <span className="min-w-0 flex-1 truncate text-sm text-foreground">
                      {a.name}
                    </span>
                    <ArrowRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
                  </button>
                </li>
              );
            })}
          </ul>
        </section>
      ) : null}
    </aside>
  );
}

function BoardRow({ entry }: { entry: AgentBoardEntry }) {
  const href = agentBoardHref(entry);
  const body = (
    <div className="flex items-start gap-2.5 rounded-lg border border-border bg-card px-2.5 py-2 transition-colors hover:border-primary/40">
      <span
        className={cn(
          "mt-1.5 h-2 w-2 shrink-0 rounded-full",
          entry.activity === "failed"
            ? "bg-signal-error"
            : "animate-chat-pulse bg-signal-live"
        )}
      />
      <span className="min-w-0 flex-1">
        <span className="flex items-baseline justify-between gap-2">
          <span className="truncate text-sm font-medium text-foreground">
            {entry.name}
          </span>
          <span className="shrink-0 text-xs capitalize text-muted-foreground">
            {entry.activity}
          </span>
        </span>
        {entry.current_job_prompt ? (
          <span className="mt-0.5 line-clamp-2 text-xs leading-5 text-muted-foreground">
            {entry.current_job_prompt}
          </span>
        ) : null}
      </span>
    </div>
  );
  return href ? (
    <Link
      href={href}
      className="block focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      {body}
    </Link>
  ) : (
    body
  );
}

function RailHeading({
  title,
  href,
  hint,
}: {
  title: string;
  href?: string;
  hint?: string;
}) {
  return (
    <div className="mb-2 flex items-baseline justify-between gap-2">
      <h2 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {title}
      </h2>
      {hint ? <span className="text-xs text-muted-foreground">{hint}</span> : null}
      {href ? (
        <Link
          href={href}
          className="text-xs text-muted-foreground transition-colors hover:text-foreground"
        >
          Open
        </Link>
      ) : null}
    </div>
  );
}

function Stat({
  label,
  value,
  icon: Icon,
}: {
  label: string;
  value: string;
  icon?: LucideIcon;
}) {
  return (
    <div className="flex items-baseline justify-between gap-2">
      <dt className="flex items-center gap-1.5 text-sm text-muted-foreground">
        {Icon ? <Icon className="h-3.5 w-3.5" /> : null}
        {label}
      </dt>
      <dd className="tabular truncate text-sm font-medium text-foreground">{value}</dd>
    </div>
  );
}

function RailSkeleton({ rows }: { rows: number }) {
  return (
    <div className="space-y-1.5">
      {Array.from({ length: rows }).map((_, i) => (
        <div
          key={i}
          className="h-12 animate-chat-shimmer rounded-lg border border-border bg-muted/40"
        />
      ))}
    </div>
  );
}

const BUILTIN_ICONS: Record<string, LucideIcon> = {
  mail: Mail,
  career_ops: BriefcaseBusiness,
  researcher: Search,
  article_writer: FileText,
  pentester: ShieldCheck,
  pr_reviewer: GitPullRequest,
  images: ImageIcon,
};

function count(n: number | undefined): string {
  return typeof n === "number" ? String(n) : "—";
}

/** Usage summed from the response envelopes already in the transcript. */
function totals(messages: ChatMessage[]): {
  turns: number;
  tokens: number;
  cost: number;
  model: string | null;
} {
  let tokens = 0;
  let cost = 0;
  let model: string | null = null;
  let turns = 0;

  for (const m of messages) {
    if (m.role === "user" || m.role === "agent") turns += 1;
    const envelope = m.metadata?.response as ChatResponse | undefined;
    const usage = envelope?.usage;
    if (!usage) continue;
    tokens += (usage.input_tokens ?? 0) + (usage.output_tokens ?? 0);
    if (typeof usage.cost_usd === "number") cost += usage.cost_usd;
    if (usage.model) model = usage.model;
  }

  return { turns, tokens, cost, model };
}
