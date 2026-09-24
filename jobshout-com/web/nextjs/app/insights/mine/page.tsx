import Link from "next/link";
import type { Metadata } from "next";
import { PenIcon } from "@/components/icons";
import { KindIcon } from "@/components/insights/KindIcon";
import { SignInPrompt } from "@/components/insights/SignInPrompt";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import { deleteInsightAction } from "@/app/insights/actions";
import {
  KIND_META,
  STATUS_LABELS,
  formatDate,
  myInsights,
  type Insight,
  type InsightStatus,
} from "@/lib/insights";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = { title: "Your insights", robots: { index: false } };

const TONE: Record<InsightStatus, "neutral" | "brand" | "good" | "warn" | "signal"> = {
  draft: "neutral",
  pending_review: "signal",
  published: "good",
  rejected: "warn",
  archived: "neutral",
};

export default async function MyInsightsPage() {
  const viewer = await currentViewer();
  let items: Insight[] = [];
  let error = "";
  if (viewer) {
    try {
      items = await myInsights(viewer);
    } catch (e) {
      error = e instanceof Error ? e.message : "Could not load your insights";
    }
  }

  return (
    <div className="mx-auto max-w-4xl px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header className="flex flex-wrap items-end justify-between gap-5">
        <div>
          <h1 className="font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
            Your insights
          </h1>
          <p className="mt-3 text-base text-mute">Drafts, submissions and what is live.</p>
        </div>
        {viewer ? (
          <Link href="/insights/new" className={buttonClass("primary", "md")}>
            <PenIcon className="h-4 w-4" />
            Write something
          </Link>
        ) : null}
      </header>

      <div className="mt-10">
        {!viewer ? (
          <SignInPrompt next="/insights/mine" title="Sign in to see your insights" body="Your drafts and submissions are tied to your account." />
        ) : error ? (
          <ErrorNotice title={error} />
        ) : items.length === 0 ? (
          <EmptyState
            icon={<PenIcon className="h-6 w-6" />}
            title="Nothing here yet"
            body="Start with a quick post about something you noticed this week."
            action={
              <Link href="/insights/new?kind=post" className={buttonClass("primary", "md")}>
                Write a post
              </Link>
            }
          />
        ) : (
          <ul className="space-y-3">
            {items.map((item) => {
              const editable = item.status !== "published" && item.status !== "archived";
              return (
                <li key={item.id} className="surface-card p-5">
                  <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                    <div className="flex min-w-0 gap-3.5">
                      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-line bg-raised text-mute">
                        <KindIcon kind={item.kind} className="h-5 w-5" />
                      </span>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge tone={TONE[item.status]}>{STATUS_LABELS[item.status]}</Badge>
                          <span className="text-xs text-mute">
                            {KIND_META[item.kind].label} · updated {formatDate(item.updated_at)}
                          </span>
                        </div>
                        <h2 className="mt-2 break-words font-display text-lg font-semibold text-ink">
                          <Link href={`/insights/${item.slug}`} className="hover:text-shout">
                            {item.title}
                          </Link>
                        </h2>
                        {item.review_note ? (
                          <p className="mt-2 rounded-xl border border-warn/30 bg-warn/[0.06] px-3 py-2 text-sm text-body">
                            <span className="font-semibold">Editor’s note:</span> {item.review_note}
                          </p>
                        ) : null}
                      </div>
                    </div>
                    {editable ? (
                      <div className="flex shrink-0 gap-2">
                        <Link href={`/insights/${item.slug}/edit`} className={buttonClass("secondary", "sm")}>
                          Edit
                        </Link>
                        <form action={deleteInsightAction}>
                          <input type="hidden" name="id" value={item.id} />
                          <button type="submit" className={buttonClass("ghost", "sm")}>
                            Delete
                          </button>
                        </form>
                      </div>
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
