"use client";

import { useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { CheckCircle2, ArrowLeft, Newspaper, Send } from "lucide-react";
import {
  useBlogArticles,
  useBlogConfig,
  useBlogRun,
  usePublishBlogRun,
} from "@/lib/hooks/useBlog";
import { RunSteps } from "@/components/blog/RunSteps";
import { ArticleViewer } from "@/components/blog/ArticleViewer";
import { cn } from "@/lib/utils/cn";

export default function ArticleRunPage() {
  const params = useParams();
  const runId = String(params.runId ?? "");

  const { data: run, isLoading, isError } = useBlogRun(runId);
  const { data: config } = useBlogConfig();
  const publish = usePublishBlogRun();

  // No point asking for bodies until generation has produced them.
  const { data: articles } = useBlogArticles(
    runId,
    run?.status === "completed"
  );

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const selected =
    articles?.find((a) => a.id === selectedId) ?? articles?.[0] ?? null;

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="h-8 w-64 animate-pulse rounded bg-muted" />
        <div className="h-64 animate-pulse rounded-xl bg-muted" />
      </div>
    );
  }

  if (isError || !run) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
        Could not load this run.
      </div>
    );
  }

  const isPublished = Boolean(run.published_at);
  const canPublish =
    Boolean(config?.can_publish) && run.status === "completed" && !isPublished;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <Link
          href="/articles"
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Articles
        </Link>

        <div className="mt-2 flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h1 className="text-2xl font-bold tracking-tight text-foreground">
              {run.topics.length === 1
                ? run.topics[0]
                : `${run.topics.length} articles`}
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Written by the Article Writer ·{" "}
              {new Date(run.created_at).toLocaleString()}
            </p>
          </div>

          <div className="flex shrink-0 items-center gap-2">
            {isPublished ? (
              <span className="inline-flex items-center gap-2 rounded-md border border-border px-4 py-2 text-sm font-medium text-foreground">
                <CheckCircle2 className="h-4 w-4 text-status-done" />
                Drafted in {run.cms_namespace ?? "the CMS"}
              </span>
            ) : (
              <button
                type="button"
                disabled={!canPublish || publish.isPending}
                onClick={() => publish.mutate(runId)}
                title={
                  config?.can_publish === false
                    ? "Publishing needs the CMS connection configured on the server"
                    : run.status !== "completed"
                      ? "Wait for the articles to finish"
                      : undefined
                }
                className={cn(
                  "inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors",
                  "bg-primary text-primary-foreground hover:bg-primary/90",
                  "disabled:cursor-not-allowed disabled:opacity-50"
                )}
              >
                <Send className="h-4 w-4" />
                {publish.isPending ? "Sending..." : "Send to CMS"}
              </button>
            )}
          </div>
        </div>

        {config?.can_publish === false && (
          <p className="mt-3 rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
            Publishing is unavailable — the server has no CMS connection
            configured. Articles can still be written, read and downloaded.
          </p>
        )}

        {isPublished && (
          <p className="mt-3 rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
            Sent as {run.articles.length} draft
            {run.articles.length === 1 ? "" : "s"}. Nothing is live until
            it&apos;s published in the CMS.
          </p>
        )}

        {run.error_message && (
          <p className="mt-3 rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2 text-xs text-destructive">
            {run.error_message}
          </p>
        )}
      </div>

      {/* minmax(0,1fr) rather than 1fr: a plain 1fr track has an auto minimum,
          so a long code line or file path in the article would force the track
          wider than the viewport and push the whole page sideways. */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[18rem_minmax(0,1fr)]">
        {/* Left rail: progress + article picker */}
        <div className="min-w-0 space-y-6">
          <section className="rounded-xl border border-border bg-card p-5 shadow-card">
            <h2 className="mb-4 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Progress
            </h2>
            <RunSteps steps={run.steps} />
          </section>

          {articles && articles.length > 1 && (
            <section className="rounded-xl border border-border bg-card p-3 shadow-card">
              <h2 className="mb-2 px-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                Articles
              </h2>
              <ul className="space-y-0.5">
                {articles.map((a) => (
                  <li key={a.id}>
                    <button
                      type="button"
                      onClick={() => setSelectedId(a.id)}
                      className={cn(
                        "w-full truncate rounded-md px-2 py-1.5 text-left text-xs transition-colors",
                        selected?.id === a.id
                          ? "bg-primary/10 font-medium text-primary"
                          : "text-muted-foreground hover:bg-accent hover:text-foreground"
                      )}
                    >
                      {a.topic}
                    </button>
                  </li>
                ))}
              </ul>
            </section>
          )}
        </div>

        {/* Right: the article itself */}
        <section className="flex min-h-[28rem] min-w-0 flex-col rounded-xl border border-border bg-card p-5 shadow-card">
          {selected ? (
            <ArticleViewer article={selected} />
          ) : (
            <div className="flex flex-1 flex-col items-center justify-center text-center">
              <Newspaper className="h-10 w-10 text-muted-foreground/40" />
              <p className="mt-3 text-sm text-muted-foreground">
                {run.status === "running" || run.status === "pending"
                  ? "The Article Writer is still working. This updates automatically."
                  : run.status === "failed"
                    ? "This run failed before producing an article."
                    : "No articles were produced."}
              </p>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
