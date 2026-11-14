import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { markAllReadAction, openNotificationAction, savePreferenceAction } from "@/app/notifications/actions";
import { BellIcon } from "@/components/icons";
import { EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import { relativeTime } from "@/lib/format";
import {
  getNotificationPreferences,
  listNotifications,
  type AppNotification,
  type NotificationPreference,
} from "@/lib/notifications";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Notifications",
  robots: { index: false, follow: false },
};

export default async function NotificationsPage({
  searchParams,
}: {
  searchParams: { done?: string; error?: string };
}) {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=%2Fnotifications");

  let items: AppNotification[] = [];
  let unread = 0;
  let prefs: NotificationPreference[] = [];
  let error = "";
  try {
    [{ data: items, unread }, prefs] = await Promise.all([
      listNotifications(viewer),
      getNotificationPreferences(viewer),
    ]);
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load notifications";
  }

  return (
    <div className="mx-auto max-w-3xl px-5 pb-24 pt-8 sm:px-8">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="font-display text-3xl font-semibold tracking-[-0.03em] text-ink">Notifications</h1>
          <p className="mt-2 text-mute">{unread > 0 ? `${unread} unread` : "You are all caught up."}</p>
        </div>
        {unread > 0 ? (
          <form action={markAllReadAction}>
            <button type="submit" className={buttonClass("secondary", "md")}>
              Mark all read
            </button>
          </form>
        ) : null}
      </div>

      {searchParams.done === "saved" ? (
        <p role="status" className="mt-6 text-sm font-medium text-good">Setting saved.</p>
      ) : null}
      {searchParams.error || error ? (
        <div className="mt-6">
          <ErrorNotice title={searchParams.error || error} />
        </div>
      ) : null}

      {!error && items.length === 0 ? (
        <div className="mt-10">
          <EmptyState
            icon={<BellIcon className="h-6 w-6" />}
            title="No notifications yet"
            body="You will hear here when someone applies to your job, your application moves on, or someone messages or connects with you."
          />
        </div>
      ) : (
        <ul className="mt-8 divide-y divide-line rounded-card border border-line bg-surface">
          {items.map((n) => (
            <li key={n.id}>
              <form action={openNotificationAction}>
                <input type="hidden" name="id" value={n.id} />
                <input type="hidden" name="href" value={n.href} />
                <button
                  type="submit"
                  className="flex w-full items-start gap-3 px-5 py-4 text-left hover:bg-raised"
                >
                  <span
                    aria-hidden
                    className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${n.read_at ? "bg-transparent" : "bg-shout"}`}
                  />
                  <span className="min-w-0">
                    <span className={`block text-sm ${n.read_at ? "text-body" : "font-semibold text-ink"}`}>
                      {n.title}
                    </span>
                    {n.body ? <span className="mt-0.5 block truncate text-sm text-mute">{n.body}</span> : null}
                    <span className="mt-1 block text-xs text-mute">
                      {relativeTime(n.created_at)}
                      {n.read_at ? "" : " · unread"}
                    </span>
                  </span>
                </button>
              </form>
            </li>
          ))}
        </ul>
      )}

      {prefs.length > 0 ? (
        <section id="settings" className="mt-14">
          <h2 className="font-display text-xl font-semibold text-ink">Settings</h2>
          <p className="mt-1 text-sm text-mute">Choose what shows up here. Changes save one at a time.</p>
          <ul className="mt-4 divide-y divide-line rounded-card border border-line bg-surface">
            {prefs.map((p) => (
              <li key={p.kind}>
                <form action={savePreferenceAction} className="flex items-center justify-between gap-4 px-5 py-3">
                  <input type="hidden" name="kind" value={p.kind} />
                  <label className="flex items-center gap-3 text-sm text-ink">
                    <input type="checkbox" name="in_app" defaultChecked={p.in_app} className="h-4 w-4" />
                    {p.label}
                  </label>
                  <button type="submit" className={buttonClass("ghost", "sm")}>
                    Save
                  </button>
                </form>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}
