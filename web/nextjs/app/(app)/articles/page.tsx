"use client";

import { useState } from "react";
import Link from "next/link";
import {
  AlertTriangle,
  Ban,
  CheckCircle2,
  Clock,
  FileText,
  Newspaper,
  Plus,
  RotateCw,
  Search,
  Send,
  Trash2,
} from "lucide-react";
import {
  useBlogRuns,
  useCancelBlogRun,
  useDeleteBlogRun,
  useRetryBlogRun,
} from "@/lib/hooks/useBlog";
import { GenerateArticleDialog } from "@/components/blog/GenerateArticleDialog";
import { SignalDot } from "@/components/ui/signal-dot";
import { cn } from "@/lib/utils/cn";
import type { BlogRun, BlogRunStatus } from "@/lib/types/blog";

const STATUS_META: Record<
  BlogRunStatus,
  { label: string; icon: React.ElementType; className: string }
> = {
  pending: {
    label: "Queued",
    icon: Clock,
    className: "bg-status-todo/15 text-status-todo",
  },
  running: {
    label: "Writing",
    icon: Clock,
    className: "bg-status-progress/15 text-status-progress",
  },
  completed: {
    label: "Ready",
    icon: CheckCircle2,
    className: "bg-status-done/15 text-status-done",
  },
  failed: {
    label: "Failed",
    icon: AlertTriangle,
    className: "bg-status-blocked/15 text-status-blocked",
  },
};

function StatusBadge({ status }: { status: BlogRunStatus }) {
  const meta = STATUS_META[status] ?? STATUS_META.pending;
  const Icon = meta.icon;
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-2xs font-medium",
        meta.className
      )}
    >
      <Icon className="h-3 w-3" />
      {meta.label}
    </span>
  );
}

/** The step the run is on right now, for the card's live line. */
function currentStepLabel(run: BlogRun): string | null {
  return run.steps.find((s) => s.status === "running")?.label ?? null;
}

function RunCard({ run }: { run: BlogRun }) {
  const step = currentStepLabel(run);
  const retry = useRetryBlogRun();
  const cancel = useCancelBlogRun();
  const remove = useDeleteBlogRun();

  // The whole card is a link, so an action inside it has to claim the click
  // outright — otherwise deleting a run also navigates to the run being deleted.
  const act = (e: React.MouseEvent, fn: () => void) => {
    e.preventDefault();
    e.stopPropagation();
    fn();
  };
  const title =
    run.topics.length === 1
      ? run.topics[0]
      : `${run.topics.length} articles`;

  return (
    <Link
      href={`/articles/${run.id}`}
      className="group flex flex-col rounded-xl border border-border bg-card p-5 shadow-card transition-shadow hover:shadow-card-hover"
    >
      <div className="flex items-start justify-between gap-3">
        <h3 className="line-clamp-2 text-sm font-semibold text-foreground">
          {title}
        </h3>
        <StatusBadge status={run.status} />
      </div>

      {run.topics.length > 1 && (
        <p className="mt-1.5 line-clamp-2 text-xs text-muted-foreground">
          {run.topics.join(" · ")}
        </p>
      )}

      {run.status === "running" && step && (
        <p className="mt-3 flex items-center gap-2 rounded bg-muted/60 px-2 py-1.5 text-2xs text-foreground/80">
          <SignalDot status="live" size="sm" />
          <span className="truncate">{step}</span>
        </p>
      )}

      {run.status === "failed" && run.error_message && (
        <p
          className="mt-3 line-clamp-3 rounded bg-destructive/5 px-2 py-1.5 text-2xs text-destructive"
          title={run.error_message}
        >
          {run.error_message}
        </p>
      )}

      <div className="mt-auto flex items-center justify-between gap-3 pt-4 text-2xs text-muted-foreground">
        <span className="inline-flex items-center gap-1.5">
          <FileText className="h-3.5 w-3.5" />
          {run.articles.length} article{run.articles.length === 1 ? "" : "s"}
        </span>
        {run.published_at ? (
          <span className="inline-flex items-center gap-1 text-primary">
            <Send className="h-3.5 w-3.5" />
            Drafted in {run.cms_namespace ?? "the CMS"}
          </span>
        ) : (
          <time dateTime={run.created_at}>
            {new Date(run.created_at).toLocaleDateString()}
          </time>
        )}
      </div>

      {/* A run that is still writing can be cancelled. Retry and delete wait
          until it is no longer in flight. */}
      {(run.status === "running" || run.status === "pending") && (
        <div className="mt-3 flex items-center gap-2 border-t border-border pt-3">
          <button
            type="button"
            disabled={cancel.isPending}
            onClick={(e) =>
              act(e, () => {
                if (confirm("Stop this run? Work in progress will be lost.")) {
                  cancel.mutate(run.id);
                }
              })
            }
            className="inline-flex items-center gap-1.5 rounded-md border border-border px-2 py-1 text-2xs text-muted-foreground transition-colors hover:border-destructive/40 hover:text-destructive disabled:opacity-50"
          >
            <Ban className="h-3 w-3" />
            {cancel.isPending ? "Stopping..." : "Cancel"}
          </button>
        </div>
      )}
      {run.status !== "running" && run.status !== "pending" && (
        <div className="mt-3 flex items-center gap-2 border-t border-border pt-3">
          {run.status === "failed" && (
            <button
              type="button"
              disabled={retry.isPending}
              onClick={(e) => act(e, () => retry.mutate(run.id))}
              className="inline-flex items-center gap-1.5 rounded-md border border-border px-2 py-1 text-2xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50"
            >
              <RotateCw className="h-3 w-3" />
              {retry.isPending ? "Retrying..." : "Retry"}
            </button>
          )}
          <button
            type="button"
            disabled={remove.isPending}
            onClick={(e) =>
              act(e, () => {
                if (confirm(`Delete this run and its articles?`)) {
                  remove.mutate(run.id);
                }
              })
            }
            className="ml-auto inline-flex items-center gap-1.5 rounded-md p-1 text-muted-foreground transition-colors hover:text-destructive disabled:opacity-50"
            aria-label="Delete run"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      )}
    </Link>
  );
}

export default function ArticlesPage() {
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const { data, isLoading, isError } = useBlogRuns({ per_page: 50 });

  const runs = (data?.data ?? []).filter((run) =>
    searchQuery
      ? run.topics.some((t) =>
          t.toLowerCase().includes(searchQuery.toLowerCase())
        )
      : true
  );

  return (
    <>
      <div className="space-y-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">
              Articles
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Everything the Article Writer has produced. Review here, then
              file it in the CMS as a draft when it&apos;s ready.
            </p>
          </div>
          <button
            type="button"
            onClick={() => setIsCreateOpen(true)}
            className="inline-flex shrink-0 items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
          >
            <Plus className="h-4 w-4" />
            Write articles
          </button>
        </div>

        <div className="relative max-w-md">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="search"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search by topic..."
            className="w-full rounded-md border border-border bg-background py-2 pl-9 pr-3 text-sm text-foreground placeholder:text-muted-foreground"
          />
        </div>

        {isLoading && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="h-40 animate-pulse rounded-xl bg-muted" />
            ))}
          </div>
        )}

        {isError && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            Failed to load articles. The backend may be unreachable.
          </div>
        )}

        {!isLoading && !isError && runs.length === 0 && (
          <div className="rounded-xl border border-dashed border-border py-16 text-center">
            <Newspaper className="mx-auto h-10 w-10 text-muted-foreground/50" />
            <p className="mt-3 text-sm text-muted-foreground">
              {searchQuery
                ? "No articles match that search."
                : "No articles yet."}
            </p>
            {!searchQuery && (
              <button
                type="button"
                onClick={() => setIsCreateOpen(true)}
                className="mt-4 inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
              >
                <Plus className="h-4 w-4" />
                Write your first article
              </button>
            )}
          </div>
        )}

        {!isLoading && !isError && runs.length > 0 && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {runs.map((run) => (
              <RunCard key={run.id} run={run} />
            ))}
          </div>
        )}
      </div>

      <GenerateArticleDialog
        open={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
      />
    </>
  );
}
