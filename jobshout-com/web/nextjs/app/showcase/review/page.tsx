import Link from "next/link";
import type { Metadata } from "next";
import { InboxIcon, ShieldIcon } from "@/components/icons";
import { SignInPrompt } from "@/components/insights/SignInPrompt";
import { AppLogo, BuildBadge, MaturityBadge } from "@/components/showcase/AppCard";
import { ShowcaseReviewActions } from "@/components/showcase/ShowcaseReviewActions";
import { Badge, EmptyState, ErrorNotice } from "@/components/ui";
import { formatDate } from "@/lib/insights";
import {
  APP_TYPES,
  VISIBILITY,
  displayUrl,
  listApps,
  showcaseReviewQueue,
  type ShowcaseApp,
} from "@/lib/showcase";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = { title: "Showcase review queue", robots: { index: false } };

function QueueItem({ app }: { app: ShowcaseApp }) {
  const links = [app.repo_url, app.demo_url, app.website_url, app.docs_url, ...app.screenshots].filter(Boolean);
  return (
    <li className="surface-card p-5 sm:p-6">
      <div className="flex gap-3.5">
        <AppLogo app={app} size="sm" />
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <MaturityBadge app={app} />
            <BuildBadge app={app} />
            <Badge>{APP_TYPES[app.app_type]}</Badge>
            <Badge>{VISIBILITY[app.visibility].label}</Badge>
            {app.featured ? <Badge tone="solid">Featured</Badge> : null}
          </div>
          <h2 className="mt-3 break-words font-display text-xl font-semibold text-ink">
            <Link href={`/showcase/${app.slug}`} className="hover:text-shout">
              {app.name}
            </Link>
          </h2>
          <p className="mt-1 text-xs text-mute">
            {app.creator_display_name}
            {app.creator_email ? ` <${app.creator_email}>` : ""} · {formatDate(app.published_at || app.updated_at)}
          </p>
          {app.tagline ? <p className="mt-3 text-sm leading-relaxed text-body">{app.tagline}</p> : null}
          {links.length ? (
            <div className="mt-3">
              <p className="text-xs font-semibold text-ink">Check where these go before approving:</p>
              <ul className="mt-1 space-y-0.5 text-xs text-mute">
                {links.map((l) => (
                  <li key={l} className="truncate">
                    <a href={l} target="_blank" rel="noopener noreferrer nofollow" className="underline decoration-line underline-offset-2 hover:text-shout">
                      {displayUrl(l)}
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      </div>
      <div className="mt-5 border-t border-line pt-4">
        <ShowcaseReviewActions id={app.id} status={app.status} featured={app.featured} />
      </div>
    </li>
  );
}

export default async function ShowcaseReviewPage() {
  const viewer = await currentViewer();
  if (!viewer) {
    return (
      <div className="mx-auto max-w-4xl px-5 pb-24 pt-14 sm:px-8">
        <SignInPrompt next="/showcase/review" title="Editors only" body="Sign in with an editor account to review apps." />
      </div>
    );
  }

  let queue: ShowcaseApp[] | null = null;
  let live: ShowcaseApp[] = [];
  let error = "";
  try {
    [queue, { data: live }] = await Promise.all([
      showcaseReviewQueue(viewer),
      listApps({ sort: "updated", limit: 12 }, viewer),
    ]);
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
          Showcase review queue
        </h1>
        <p className="mt-3 text-base text-mute">
          Open every link before you approve: visitors leave JobShout through them. When you request changes, say
          exactly what to fix.
        </p>
      </header>

      <div className="mt-10 space-y-12">
        {error ? (
          <ErrorNotice title={error} />
        ) : queue === null ? (
          <ErrorNotice
            title="Your account is not an editor"
            body="Editors are set by INSIGHTS_STAFF_EMAILS on the marketplace API, and moderate both Insights and the showcase."
          />
        ) : (
          <>
            <section aria-labelledby="pending-heading">
              <h2 id="pending-heading" className="font-display text-xl font-semibold text-ink">
                Waiting for review <span className="text-mute">({queue.length})</span>
              </h2>
              {queue.length ? (
                <ul className="mt-5 space-y-4">
                  {queue.map((app) => (
                    <QueueItem key={app.id} app={app} />
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
                Recently updated
              </h2>
              <ul className="mt-5 space-y-4">
                {live.map((app) => (
                  <QueueItem key={app.id} app={app} />
                ))}
              </ul>
            </section>
          </>
        )}
      </div>
    </div>
  );
}
