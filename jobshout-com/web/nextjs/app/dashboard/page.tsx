import Link from "next/link";
import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { BriefcaseIcon } from "@/components/icons";
import { Badge, EmptyState, buttonClass } from "@/components/ui";
import { getDashboard } from "@/lib/dashboard";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Dashboard",
  description: "Your JobShout command centre — what to do next, and why.",
  robots: { index: false, follow: false },
};

export default async function DashboardPage() {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=/dashboard");

  let dash = null;
  let error = "";
  try {
    dash = await getDashboard(viewer);
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load dashboard";
  }

  if (!dash && !error) {
    return (
      <div className="mx-auto max-w-board px-5 py-16 sm:px-8">
        <EmptyState
          icon={<BriefcaseIcon className="h-6 w-6" />}
          title="Dashboard coming soon"
          body="The professional command centre is ready in code but still dark on this ring. Ask an admin to set DASHBOARD_ENABLED."
        />
      </div>
    );
  }

  if (!dash) {
    return (
      <div className="mx-auto max-w-board px-5 py-16 sm:px-8">
        <p className="text-mute">{error}</p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-8 sm:px-8 sm:pt-10">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <Badge tone="signal">Home</Badge>
          <h1 className="mt-3 font-display text-3xl font-semibold tracking-[-0.03em] text-ink sm:text-4xl">
            What to do next
          </h1>
          <p className="mt-2 max-w-xl text-mute">
            A calm command centre — opportunities, in-flight work, and profile strength. Not a noisy feed.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Link href="/messages" className={buttonClass("secondary", "md")}>
            Messages{dash.unread_messages > 0 ? ` (${dash.unread_messages})` : ""}
          </Link>
          <Link href="/members/me" className={buttonClass("primary", "md")}>
            Edit profile
          </Link>
        </div>
      </div>

      <section className="mt-10">
        <h2 className="font-display text-xl font-semibold text-ink">Today</h2>
        <ul className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {dash.today.map((a) => (
            <li key={a.id} className="rounded-card border border-line bg-surface p-5">
              <Link href={a.href} className="font-medium text-ink hover:text-shout">
                {a.title}
              </Link>
              <p className="mt-2 text-sm leading-relaxed text-mute">{a.reason}</p>
            </li>
          ))}
        </ul>
      </section>

      <div className="mt-12 grid gap-10 lg:grid-cols-3">
        <section className="lg:col-span-2">
          <h2 className="font-display text-xl font-semibold text-ink">Opportunity radar</h2>
          {dash.opportunities.length === 0 ? (
            <p className="mt-3 text-sm text-mute">No ranked opportunities yet — complete your profile and skills.</p>
          ) : (
            <ul className="mt-4 space-y-3">
              {dash.opportunities.map((o) => (
                <li key={o.id} className="rounded-card border border-line bg-surface p-5">
                  <div className="flex items-start justify-between gap-4">
                    <div>
                      <Link href={o.href} className="font-medium text-ink hover:text-shout">
                        {o.title}
                      </Link>
                      <p className="mt-1 text-sm text-mute">{o.summary}</p>
                      <ul className="mt-2 list-inside list-disc text-sm text-body">
                        {o.reasons.map((r) => (
                          <li key={r}>{r}</li>
                        ))}
                      </ul>
                    </div>
                    <span className="shrink-0 rounded-pill bg-raised px-2.5 py-1 text-xs font-medium text-mute">
                      {Math.round(o.score * 100)}% match
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          )}

          <h2 className="mt-12 font-display text-xl font-semibold text-ink">In flight</h2>
          <ul className="mt-4 space-y-3">
            {dash.pipeline.map((p) => (
              <li key={p.id} className="flex items-center justify-between gap-4 rounded-card border border-line bg-surface px-5 py-4">
                <div>
                  <Link href={p.href} className="font-medium text-ink hover:text-shout">
                    {p.title}
                  </Link>
                  <p className="text-sm text-mute">
                    {p.stage} · next: {p.next_step}
                  </p>
                </div>
                <Badge tone="neutral">{p.kind}</Badge>
              </li>
            ))}
          </ul>
        </section>

        <aside className="space-y-8">
          <section className="rounded-card border border-line bg-surface p-5">
            <h2 className="font-display text-lg font-semibold text-ink">Profile strength</h2>
            <p className="mt-3 font-display text-4xl font-semibold text-ink">{dash.stats.profile_strength}%</p>
            {dash.stats.profile_fixes.length > 0 ? (
              <ul className="mt-3 list-inside list-disc text-sm text-mute">
                {dash.stats.profile_fixes.map((f) => (
                  <li key={f}>{f}</li>
                ))}
              </ul>
            ) : (
              <p className="mt-3 text-sm text-mute">Looking solid.</p>
            )}
          </section>

          <section className="rounded-card border border-line bg-surface p-5">
            <h2 className="font-display text-lg font-semibold text-ink">Credits</h2>
            <p className="mt-2 text-2xl font-semibold text-ink">{dash.credits.balance}</p>
            <p className="text-sm text-mute">Plan: {dash.credits.plan}</p>
          </section>

          <section className="rounded-card border border-line bg-surface p-5">
            <h2 className="font-display text-lg font-semibold text-ink">Weekly goals</h2>
            <ul className="mt-3 space-y-2">
              {dash.weekly_goals.map((g) => (
                <li key={g.id}>
                  <Link href={g.href} className="text-sm font-medium text-ink hover:text-shout">
                    {g.title}
                  </Link>
                </li>
              ))}
            </ul>
          </section>

          {dash.approval_inbox.length > 0 ? (
            <section className="rounded-card border border-line bg-surface p-5">
              <h2 className="font-display text-lg font-semibold text-ink">AI approval inbox</h2>
              <ul className="mt-3 space-y-2 text-sm">
                {dash.approval_inbox.map((d) => (
                  <li key={d.id}>
                    <span className="font-medium text-ink">{d.title}</span>
                    <p className="text-mute">{d.preview}</p>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
        </aside>
      </div>
    </div>
  );
}
