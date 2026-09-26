"use client";

import Link from "next/link";
import {
  AlertTriangle,
  CheckCircle2,
  Clock,
  ExternalLink,
  FileText,
  Globe,
  Loader2,
  Newspaper,
  Sparkles,
} from "lucide-react";
import {
  useBlogArticles,
  useBlogConfig,
  useBlogRuns,
  useGenerateBlog,
  usePublishBlogRunLive,
} from "@/lib/hooks/useBlog";
import { cn } from "@/lib/utils/cn";
import { blogRunTitle, type BlogRun } from "@/lib/types/blog";

/** The builtin this tab belongs to; its runs are listed with ?writer=. */
const WRITER = "jobshout_com_writer";

type Stage = "writing" | "failed" | "draft" | "unfiled" | "live";

/** Where a run is in write → draft → live, from what the server recorded. */
function stageOf(run: BlogRun): Stage {
  if (run.status === "running" || run.status === "pending") return "writing";
  if (run.status === "failed") return "failed";
  if (run.insights_published_at) return "live";
  if (run.published_at) return "draft";
  return "unfiled";
}

const STAGE_META: Record<Stage, { label: string; icon: typeof Clock; className: string }> = {
  writing: { label: "Writing", icon: Loader2, className: "bg-signal-live/10 text-signal-live" },
  failed: { label: "Failed", icon: AlertTriangle, className: "bg-signal-error/10 text-signal-error" },
  unfiled: { label: "Written", icon: FileText, className: "bg-muted text-muted-foreground" },
  draft: { label: "Draft in CMS", icon: Clock, className: "bg-signal/10 text-signal" },
  live: { label: "Live", icon: CheckCircle2, className: "bg-status-done/10 text-status-done" },
};

function StageBadge({ stage }: { stage: Stage }) {
  const meta = STAGE_META[stage];
  const Icon = meta.icon;
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-2xs font-medium",
        meta.className
      )}
    >
      <Icon className={cn("h-3 w-3", stage === "writing" && "animate-spin")} />
      {meta.label}
    </span>
  );
}

/** Links to the published articles on this ring's jobshout.com. */
function LiveLinks({ runId, siteURL }: { runId: string; siteURL: string }) {
  const { data: articles } = useBlogArticles(runId);
  const live = (articles ?? []).filter((a) => a.insights_slug);
  if (!siteURL || live.length === 0) return null;
  return (
    <>
      {live.map((a) => (
        <a
          key={a.id}
          href={`${siteURL}/insights/${a.insights_slug}`}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
        >
          <Globe className="h-3.5 w-3.5" />
          View on jobshout.com
          <ExternalLink className="h-3 w-3" />
        </a>
      ))}
    </>
  );
}

function RunRow({
  run,
  canPublishLive,
  siteURL,
}: {
  run: BlogRun;
  canPublishLive: boolean;
  siteURL: string;
}) {
  const stage = stageOf(run);
  const publish = usePublishBlogRunLive();
  const words = run.articles.reduce((n, a) => n + (a.word_count || 0), 0);
  const step = run.steps.find((s) => s.status === "running")?.label;
  const canPress = canPublishLive && (stage === "draft" || stage === "unfiled");

  const onPublish = () => {
    const ok = window.confirm(
      `Publish "${blogRunTitle(run)}" live?\n\nThe CMS post becomes public and the article is published on jobshout.com straight away — there is no second review.`
    );
    if (ok) publish.mutate(run.id);
  };

  return (
    <li className="rounded-xl border border-border bg-card p-4 shadow-card">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="line-clamp-2 text-sm font-semibold text-foreground">{blogRunTitle(run)}</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            {new Date(run.created_at).toLocaleString()}
            {run.source === "schedule" ? " · scheduled" : ""}
            {words > 0 ? ` · ${words.toLocaleString()} words` : ""}
          </p>
          {stage === "writing" && step && (
            <p className="mt-1 text-xs text-muted-foreground">{step}</p>
          )}
          {run.error_message && stage !== "writing" && (
            <p className="mt-1 line-clamp-2 text-xs text-signal-error">{run.error_message}</p>
          )}
        </div>
        <StageBadge stage={stage} />
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-3">
        <Link
          href={`/articles/${run.id}`}
          className="inline-flex items-center gap-1 text-xs font-medium text-foreground hover:underline"
        >
          <FileText className="h-3.5 w-3.5" />
          Read
        </Link>
        {stage === "live" && <LiveLinks runId={run.id} siteURL={siteURL} />}
        {(stage === "draft" || stage === "unfiled") && (
          <button
            type="button"
            onClick={onPublish}
            disabled={!canPress || publish.isPending}
            title={canPublishLive ? undefined : "Publishing live is not configured on this deployment"}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {publish.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Globe className="h-3.5 w-3.5" />}
            Publish live
          </button>
        )}
      </div>
    </li>
  );
}

/**
 * JobShout.com Content Writer tab: its runs, and the one action that is its
 * own — publishing an article live, which makes the CMS post public and
 * publishes it on jobshout.com. Runs with a topic start from New task / Run
 * task / chat, which use the one server-side launch schema.
 */
export function InsightsWriterClient() {
  const { data, isLoading } = useBlogRuns({ per_page: 50, writer: WRITER });
  const { data: config } = useBlogConfig();
  const generate = useGenerateBlog();
  const runs = data?.data ?? [];
  const canPublishLive = Boolean(config?.can_publish_live);
  const siteURL = config?.insights_site_url ?? "";

  const writeTrending = () =>
    generate.mutate({ briefs: [], writer: WRITER, trending: true, trending_count: 1, auto_publish: true });

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-prose text-sm text-muted-foreground">
          Long-form AI-trends articles for jobshout.com Insights. Each finished article is filed in the CMS
          as a draft; <span className="font-medium text-foreground">Publish live</span> makes it public there
          and on jobshout.com.
        </p>
        <button
          type="button"
          onClick={writeTrending}
          disabled={generate.isPending}
          className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-muted disabled:opacity-50"
        >
          {generate.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Sparkles className="h-3.5 w-3.5" />}
          Write a trending article now
        </button>
      </div>

      {config && !canPublishLive && (
        <p className="rounded-md border border-signal/30 bg-signal/5 px-3 py-2 text-xs text-muted-foreground">
          Publishing live is not configured on this deployment: it needs the opsapi CMS (with the cms:update
          scope) and jobshout.com Insights.
        </p>
      )}

      {isLoading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : runs.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
          <Newspaper className="mx-auto mb-2 h-6 w-6" />
          No articles yet. Write a trending one now, give it a topic from{" "}
          <span className="font-medium">New task</span>, or wait for the daily schedule.
        </div>
      ) : (
        <ul className="space-y-3" aria-label="Insights articles">
          {runs.map((r) => (
            <RunRow key={r.id} run={r} canPublishLive={canPublishLive} siteURL={siteURL} />
          ))}
        </ul>
      )}
    </div>
  );
}
