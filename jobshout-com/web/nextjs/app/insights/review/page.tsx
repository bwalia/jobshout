import Link from "next/link";
import type { Metadata } from "next";
import { InboxIcon, ShieldIcon } from "@/components/icons";
import { KindBadge } from "@/components/insights/InsightCard";
import { ReviewActions } from "@/components/insights/ReviewActions";
import { SignInPrompt } from "@/components/insights/SignInPrompt";
import { Badge, EmptyState, ErrorNotice } from "@/components/ui";
import { consumeLabel, formatDate, listInsights, reviewQueue, type Insight } from "@/lib/insights";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = { title: "Review queue", robots: { index: false } };

function QueueItem({ item }: { item: Insight }) {
  return (
    <li className="surface-card p-5 sm:p-6">
      <div className="flex flex-wrap items-center gap-2">
        <KindBadge item={item} />
        {item.topics.map((t) => (
          <Badge key={t.slug}>{t.name}</Badge>
        ))}
        {item.featured ? <Badge tone="solid">Featured</Badge> : null}
      </div>
      <h2 className="mt-3 break-words font-display text-xl font-semibold text-ink">
        <Link href={`/insights/${item.slug}`} className="hover:text-shout">
          {item.title}
        </Link>
      </h2>
      <p className="mt-1 text-xs text-mute">
        {item.author_display_name}
        {item.author_email ? ` <${item.author_email}>` : ""} · {item.source} ·{" "}
        {formatDate(item.published_at || item.updated_at)} · {consumeLabel(item)}
      </p>
      {item.summary ? <p className="mt-3 text-sm leading-relaxed text-body">{item.summary}</p> : null}
      <div className="mt-5 border-t border-line pt-4">
        <ReviewActions id={item.id} status={item.status} featured={item.featured} />
      </div>
    </li>
  );
}

export default async function ReviewPage() {
  const viewer = await currentViewer();
  if (!viewer) {
    return (
      <div className="mx-auto max-w-4xl px-5 pb-24 pt-14 sm:px-8">
        <SignInPrompt next="/insights/review" title="Editors only" body="Sign in with an editor account to review submissions." />
      </div>
    );
  }

  let queue: Insight[] | null = null;
  let live: Insight[] = [];
  let error = "";
  try {
    [queue, { data: live }] = await Promise.all([reviewQueue(viewer), listInsights({ limit: 12 })]);
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load the queue";
  }

  return (
    <div className="mx-auto max-w-4xl px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header>
        <Badge tone="signal">
          <ShieldIcon className="h-3.5 w-3.5" />
          Editors
        </Badge>
        <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
          Review queue
        </h1>
        <p className="mt-3 text-base text-mute">
          Approve what is accurate and useful. When you request changes, say exactly what to fix.
        </p>
      </header>

      <div className="mt-10 space-y-12">
        {error ? (
          <ErrorNotice title={error} />
        ) : queue === null ? (
          <ErrorNotice
            title="Your account is not an Insights editor"
            body="Editors are set by INSIGHTS_STAFF_EMAILS on the marketplace API."
          />
        ) : (
          <>
            <section aria-labelledby="pending-heading">
              <h2 id="pending-heading" className="font-display text-xl font-semibold text-ink">
                Waiting for review <span className="text-mute">({queue.length})</span>
              </h2>
              {queue.length ? (
                <ul className="mt-5 space-y-4">
                  {queue.map((item) => (
                    <QueueItem key={item.id} item={item} />
                  ))}
                </ul>
              ) : (
                <div className="mt-5">
                  <EmptyState icon={<InboxIcon className="h-6 w-6" />} title="Queue is clear" body="Nothing is waiting. New submissions show up here." />
                </div>
              )}
            </section>
            <section aria-labelledby="live-heading">
              <h2 id="live-heading" className="font-display text-xl font-semibold text-ink">
                Recently published
              </h2>
              <ul className="mt-5 space-y-4">
                {live.map((item) => (
                  <QueueItem key={item.id} item={item} />
                ))}
              </ul>
            </section>
          </>
        )}
      </div>
    </div>
  );
}
