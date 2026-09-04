"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import {
  ArrowRight,
  BriefcaseBusiness,
  FileText,
  GitPullRequest,
  Image as ImageIcon,
  LayoutDashboard,
  Mail,
  Search,
  ShieldCheck,
  Sparkles,
  Users,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils/cn";
import { useAgents } from "@/lib/hooks/useAgents";

/**
 * The old empty state was a composer and six unlabelled pills — it never said
 * what this page is for or who is on the other end. This one answers three
 * questions before the first keystroke: what JobShout does, which specialists
 * exist, and what a good first message looks like.
 */

type Suggestion = {
  label: string;
  prompt: string;
  icon: LucideIcon;
};

/** Grouped so the list reads as capabilities, not a bag of examples. */
const GROUPS: { name: string; items: Suggestion[] }[] = [
  {
    name: "Get oriented",
    items: [
      {
        label: "What can you do?",
        prompt: "What can you do? List your specialists and their tools.",
        icon: Sparkles,
      },
      {
        label: "List my agents",
        prompt: "List my agents",
        icon: Users,
      },
      {
        label: "What am I allowed to do?",
        prompt: "What am I allowed to do?",
        icon: ShieldCheck,
      },
    ],
  },
  {
    name: "Run the work",
    items: [
      {
        label: "Who is working on what?",
        prompt: "Who is working on what right now?",
        icon: LayoutDashboard,
      },
      {
        label: "Review a pull request",
        prompt: "Review pull request 184 and summarise the risks",
        icon: GitPullRequest,
      },
      {
        label: "Research a topic",
        prompt:
          "Research the current state of on-device LLM inference and give me a brief with citations",
        icon: Search,
      },
    ],
  },
  {
    name: "Track the cost",
    items: [
      {
        label: "This month's usage",
        prompt: "What's my usage this month?",
        icon: FileText,
      },
      {
        label: "Create a task",
        prompt: "Create a task to fix the login timeout",
        icon: BriefcaseBusiness,
      },
      {
        label: "Draft a reply",
        prompt: "Draft a reply to the most recent unread email",
        icon: Mail,
      },
    ],
  },
];

/** Icon + one-liner per built-in specialist, keyed by `metadata.builtin`. */
// Keys are the server's metadata.builtin values (server/internal/model/agent.go).
const BUILTIN_META: Record<string, { icon: LucideIcon; blurb: string }> = {
  mail: { icon: Mail, blurb: "Triages your inbox and drafts replies" },
  career_ops: {
    icon: BriefcaseBusiness,
    blurb: "Matches roles against your profile",
  },
  researcher: { icon: Search, blurb: "Digs through sources and cites them" },
  article_writer: {
    icon: FileText,
    blurb: "Writes and publishes long-form drafts",
  },
  pentester: {
    icon: ShieldCheck,
    blurb: "Probes a target and reports findings",
  },
  pr_reviewer: {
    icon: GitPullRequest,
    blurb: "Reads diffs and flags the risky ones",
  },
  images: { icon: ImageIcon, blurb: "Generates and edits pictures" },
};

export function ChatEmptyState({ onPick }: { onPick: (prompt: string) => void }) {
  const [group, setGroup] = useState(GROUPS[0].name);
  const agentsQuery = useAgents({ per_page: 50 });

  const specialists = useMemo(() => {
    const all = agentsQuery.data?.data ?? [];
    return all.filter((a) => typeof a.metadata?.builtin === "string").slice(0, 7);
  }, [agentsQuery.data]);

  const active = GROUPS.find((g) => g.name === group) ?? GROUPS[0];

  return (
    <div className="w-full">
      <div className="mb-6 text-center">
        <h1 className="font-display text-3xl tracking-tight text-foreground">
          What should we get done?
        </h1>
        <p className="mx-auto mt-2 max-w-xl text-base leading-7 text-muted-foreground">
          Describe a job in plain English. A specialist plans it, runs it in your real
          tools, and stops for your approval before anything irreversible.
        </p>
      </div>

      {/* Specialists come from the API, so this reflects the org's actual
          roster rather than a hardcoded marketing list. */}
      <div className="grid gap-6 lg:grid-cols-2">
        {agentsQuery.isLoading ? (
          <div className="grid grid-cols-2 gap-2">
            {Array.from({ length: 6 }).map((_, i) => (
              <div
                key={i}
                className="h-[52px] animate-chat-shimmer rounded-xl border border-border bg-muted/40"
              />
            ))}
          </div>
        ) : specialists.length > 0 ? (
          <div>
            <div className="mb-2 flex items-baseline justify-between">
              <h2 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                Your specialists
              </h2>
              <Link
                href="/panel/task-manager"
                className="flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
              >
                Manage
                <ArrowRight className="h-3 w-3" />
              </Link>
            </div>
            <div className="grid grid-cols-2 gap-2">
              {specialists.map((a) => {
                const key = String(a.metadata?.builtin ?? "");
                const meta = BUILTIN_META[key];
                const Icon = meta?.icon ?? Sparkles;
                return (
                  <button
                    key={a.id}
                    type="button"
                    onClick={() => onPick(`Ask ${a.name} to `)}
                    className="group flex items-start gap-2.5 rounded-xl border border-border bg-card p-2.5 text-left transition-colors duration-200 hover:border-primary/40 hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-secondary text-muted-foreground transition-colors group-hover:text-primary">
                      <Icon className="h-4 w-4" />
                    </span>
                    <span className="min-w-0">
                      <span className="block truncate text-sm font-medium text-foreground">
                        {a.name}
                      </span>
                      <span className="line-clamp-2 text-xs leading-5 text-muted-foreground">
                        {meta?.blurb ?? a.role}
                      </span>
                    </span>
                  </button>
                );
              })}
            </div>
          </div>
        ) : null}

        <div>
          <div
            role="tablist"
            aria-label="Suggestion categories"
            className="mb-2 flex flex-wrap gap-1"
          >
            {GROUPS.map((g) => (
              <button
                key={g.name}
                role="tab"
                aria-selected={g.name === group}
                type="button"
                onClick={() => setGroup(g.name)}
                className={cn(
                  "rounded-full px-3.5 py-1.5 text-sm font-medium transition-colors duration-200",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                  g.name === group
                    ? "bg-secondary text-foreground"
                    : "text-muted-foreground hover:bg-secondary/60 hover:text-foreground"
                )}
              >
                {g.name}
              </button>
            ))}
          </div>

          <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-card">
            {active.items.map((s) => (
              <li key={s.label}>
                <button
                  type="button"
                  onClick={() => onPick(s.prompt)}
                  className="group flex w-full items-center gap-3 px-3.5 py-3 text-left transition-colors duration-200 hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                >
                  <s.icon className="h-4 w-4 shrink-0 text-muted-foreground transition-colors group-hover:text-primary" />
                  <span className="min-w-0 flex-1 truncate text-[15px] text-foreground">
                    {s.label}
                  </span>
                  <ArrowRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground opacity-0 transition-opacity duration-200 group-hover:opacity-100" />
                </button>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  );
}
