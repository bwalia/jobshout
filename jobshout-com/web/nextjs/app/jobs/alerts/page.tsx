import Link from "next/link";
import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { deleteJobAlertAction, saveJobAlertAction } from "@/app/actions";
import { ConfirmSubmit } from "@/components/ConfirmSubmit";
import { BellIcon } from "@/components/icons";
import { EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import { EMPLOYMENT_TYPES, listJobAlerts, type JobAlert } from "@/lib/api";
import { employmentLabel } from "@/lib/format";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Job alerts",
  robots: { index: false, follow: false },
};

const DONE: Record<string, string> = {
  created: "Alert created. You will get a notification when a matching job is published.",
  updated: "Alert updated.",
  deleted: "Alert deleted.",
};

type Prefill = { q?: string; where?: string; type?: string; remote?: string };

export default async function JobAlertsPage({
  searchParams,
}: {
  searchParams: Prefill & { done?: string; error?: string; edit?: string };
}) {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=%2Fjobs%2Falerts");

  let alerts: JobAlert[] = [];
  let error = "";
  try {
    alerts = await listJobAlerts(viewer);
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load job alerts";
  }

  const editing = alerts.find((a) => a.id === searchParams.edit);
  // "Alert me" on the board links here with the current search filled in.
  const initial = editing ?? {
    q: searchParams.q ?? "",
    location: searchParams.where ?? "",
    types: (searchParams.type ?? "").split(",").filter(Boolean),
    remote: searchParams.remote === "1",
  };

  return (
    <div className="mx-auto max-w-3xl px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header>
        <h1 className="font-display text-4xl font-semibold tracking-[-0.03em] text-ink">Job alerts</h1>
        <p className="mt-3 text-base leading-relaxed text-mute">
          Saved searches. When a newly published job matches one, you get a notification on JobShout.
          Email alerts will follow once email delivery is switched on.
        </p>
      </header>

      {searchParams.done && DONE[searchParams.done] ? (
        <p role="status" className="mt-6 text-sm font-medium text-good">
          {DONE[searchParams.done]}
        </p>
      ) : null}
      {searchParams.error || error ? (
        <div className="mt-6">
          <ErrorNotice title={searchParams.error || error} />
        </div>
      ) : null}

      <section className="surface-card mt-8 p-5 sm:p-6">
        <h2 className="font-display text-lg font-semibold text-ink">
          {editing ? "Edit alert" : "New alert"}
        </h2>
        <form action={saveJobAlertAction} className="mt-4 space-y-4">
          {editing ? <input type="hidden" name="alert_id" value={editing.id} /> : null}
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="block text-sm">
              <span className="font-medium text-ink">Keywords</span>
              <input
                name="q"
                defaultValue={initial.q}
                maxLength={120}
                placeholder="Rust, platform engineer…"
                className="mt-1 w-full rounded-lg border border-line bg-raised px-3 py-2 text-ink"
              />
            </label>
            <label className="block text-sm">
              <span className="font-medium text-ink">Location</span>
              <input
                name="location"
                defaultValue={initial.location}
                maxLength={120}
                placeholder="London, GB, or remote"
                className="mt-1 w-full rounded-lg border border-line bg-raised px-3 py-2 text-ink"
              />
            </label>
          </div>
          <fieldset>
            <legend className="text-sm font-medium text-ink">Work type</legend>
            <div className="mt-2 flex flex-wrap gap-3">
              {EMPLOYMENT_TYPES.map((t) => (
                <label key={t} className="flex items-center gap-2 text-sm text-body">
                  <input type="checkbox" name="types" value={t} defaultChecked={initial.types.includes(t)} />
                  {employmentLabel(t)}
                </label>
              ))}
            </div>
          </fieldset>
          <label className="flex items-center gap-2 text-sm text-body">
            <input type="checkbox" name="remote" defaultChecked={initial.remote} />
            Remote roles only
          </label>
          <div className="flex gap-3">
            <button type="submit" className={buttonClass("primary", "md")}>
              {editing ? "Save changes" : "Create alert"}
            </button>
            {editing ? (
              <Link href="/jobs/alerts" className={buttonClass("ghost", "md")}>
                Cancel
              </Link>
            ) : null}
          </div>
        </form>
      </section>

      <section className="mt-10">
        <h2 className="font-display text-lg font-semibold text-ink">
          Your alerts <span className="text-sm font-normal text-mute">{alerts.length}</span>
        </h2>
        {!error && alerts.length === 0 ? (
          <div className="mt-4">
            <EmptyState
              icon={<BellIcon className="h-6 w-6" />}
              title="No alerts yet"
              body="Create one above, or use “Alert me” on the job board after searching."
            />
          </div>
        ) : (
          <ul className="mt-4 divide-y divide-line rounded-card border border-line bg-surface">
            {alerts.map((a) => (
              <li key={a.id} className="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
                <p className="min-w-0 text-sm text-ink">{describe(a)}</p>
                <div className="flex gap-2">
                  <Link href={`/jobs/alerts?edit=${a.id}`} className={buttonClass("secondary", "sm")}>
                    Edit
                  </Link>
                  <form action={deleteJobAlertAction}>
                    <input type="hidden" name="alert_id" value={a.id} />
                    <ConfirmSubmit confirm="Delete this alert?" className={buttonClass("ghost", "sm")}>
                      Delete
                    </ConfirmSubmit>
                  </form>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function describe(a: JobAlert): string {
  const parts = [
    a.q ? `“${a.q}”` : "",
    a.location ? `in ${a.location}` : "",
    a.types.length ? a.types.map(employmentLabel).join(", ") : "",
    a.remote ? "remote only" : "",
  ].filter(Boolean);
  return parts.join(" · ");
}
