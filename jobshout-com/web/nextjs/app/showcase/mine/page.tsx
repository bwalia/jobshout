import Link from "next/link";
import type { Metadata } from "next";
import { PlusIcon, StarIcon } from "@/components/icons";
import { SignInPrompt } from "@/components/insights/SignInPrompt";
import { AppLogo } from "@/components/showcase/AppCard";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import { deleteAppAction } from "@/app/showcase/actions";
import { formatDate } from "@/lib/insights";
import {
  APP_TYPES,
  STATUS_LABELS,
  VISIBILITY,
  myApps,
  type AppStatus,
  type ShowcaseApp,
} from "@/lib/showcase";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = { title: "Your apps", robots: { index: false } };

const TONE: Record<AppStatus, "neutral" | "brand" | "good" | "warn" | "signal"> = {
  draft: "neutral",
  pending_review: "signal",
  published: "good",
  rejected: "warn",
  archived: "neutral",
};

export default async function MyAppsPage() {
  const viewer = await currentViewer();
  let apps: ShowcaseApp[] = [];
  let error = "";
  if (viewer) {
    try {
      apps = await myApps(viewer);
    } catch (e) {
      error = e instanceof Error ? e.message : "Could not load your apps";
    }
  }

  return (
    <div className="mx-auto max-w-4xl px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header className="flex flex-wrap items-end justify-between gap-5">
        <div>
          <h1 className="font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">Your apps</h1>
          <p className="mt-3 text-base text-mute">Drafts, submissions and what is live in the AI Showcase.</p>
        </div>
        {viewer ? (
          <Link href="/showcase/new" className={buttonClass("primary", "md")}>
            <PlusIcon className="h-4 w-4" />
            Add an app
          </Link>
        ) : null}
      </header>

      <div className="mt-10">
        {!viewer ? (
          <SignInPrompt next="/showcase/mine" title="Sign in to see your apps" body="Your apps are tied to your account." />
        ) : error ? (
          <ErrorNotice title={error} />
        ) : apps.length === 0 ? (
          <EmptyState
            icon={<PlusIcon className="h-6 w-6" />}
            title="Nothing here yet"
            body="Built something with AI? Add it, say who and which agents built it, and submit it for review."
            action={
              <Link href="/showcase/new" className={buttonClass("primary", "md")}>
                Showcase your app
              </Link>
            }
          />
        ) : (
          <ul className="space-y-3">
            {apps.map((app) => (
              <li key={app.id} className="surface-card p-5">
                <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                  <div className="flex min-w-0 gap-3.5">
                    <AppLogo app={app} size="sm" />
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge tone={TONE[app.status]}>{STATUS_LABELS[app.status]}</Badge>
                        <span className="text-xs text-mute">
                          {APP_TYPES[app.app_type]} · {VISIBILITY[app.visibility].label} · updated{" "}
                          {formatDate(app.updated_at)}
                        </span>
                        {app.status === "published" ? (
                          <span className="inline-flex items-center gap-1 text-xs text-mute">
                            <StarIcon className="h-3.5 w-3.5" /> {app.star_count}
                          </span>
                        ) : null}
                      </div>
                      <h2 className="mt-2 break-words font-display text-lg font-semibold text-ink">
                        <Link href={`/showcase/${app.slug}`} className="hover:text-shout">
                          {app.name}
                        </Link>
                      </h2>
                      {app.review_note ? (
                        <p className="mt-2 rounded-xl border border-warn/30 bg-warn/[0.06] px-3 py-2 text-sm text-body">
                          <span className="font-semibold">Editor’s note:</span> {app.review_note}
                        </p>
                      ) : null}
                    </div>
                  </div>
                  {app.status !== "archived" ? (
                    <div className="flex shrink-0 gap-2">
                      <Link href={`/showcase/${app.slug}/edit`} className={buttonClass("secondary", "sm")}>
                        Edit
                      </Link>
                      <form action={deleteAppAction}>
                        <input type="hidden" name="id" value={app.id} />
                        <button type="submit" className={buttonClass("ghost", "sm")}>
                          Delete
                        </button>
                      </form>
                    </div>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
