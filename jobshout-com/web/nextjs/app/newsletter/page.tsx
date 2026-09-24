import Link from "next/link";
import type { Metadata } from "next";
import { MailIcon, RssIcon } from "@/components/icons";
import { InsightRow } from "@/components/insights/InsightCard";
import { NewsletterSignup } from "@/components/insights/NewsletterSignup";
import { Badge, EmptyState, ErrorNotice } from "@/components/ui";
import { listDigests, type InsightDigest } from "@/lib/insights";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Newsletter",
  description: "The week in AI and work: a weekly digest of JobShout Insights.",
  alternates: { canonical: "/newsletter" },
};

function weekLabel(d: InsightDigest): string {
  const start = new Date(`${d.starts_on}T00:00:00Z`);
  const end = new Date(start.getTime() + 6 * 86_400_000);
  const fmt = (x: Date) => x.toLocaleDateString("en-GB", { day: "numeric", month: "short", timeZone: "UTC" });
  return `${fmt(start)} – ${fmt(end)} ${end.getUTCFullYear()}`;
}

export default async function NewsletterPage() {
  let digests: InsightDigest[] = [];
  let error = "";
  try {
    digests = await listDigests(12);
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load the archive";
  }

  return (
    <div className="mx-auto max-w-board px-5 pb-8 pt-10 sm:px-8 sm:pt-14">
      <header className="max-w-2xl">
        <Badge tone="brand">
          <MailIcon className="h-3.5 w-3.5" />
          Weekly
        </Badge>
        <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
          The week in AI and work
        </h1>
        <p className="mt-4 text-base leading-relaxed text-mute">
          Every week’s Insights in one place. Subscribe to get it by email, or follow the{" "}
          <a href="/insights/rss.xml" className="inline-flex items-center gap-1 font-medium text-ink underline decoration-shout/50 underline-offset-4 hover:text-shout">
            <RssIcon className="h-3.5 w-3.5" />
            RSS feed
          </a>
          .
        </p>
      </header>

      <div className="mt-10">
        <NewsletterSignup />
      </div>

      <section aria-labelledby="archive-heading" className="mt-14">
        <h2 id="archive-heading" className="font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
          Archive
        </h2>
        {error ? (
          <div className="mt-6">
            <ErrorNotice title={error} />
          </div>
        ) : digests.length === 0 ? (
          <div className="mt-6">
            <EmptyState
              icon={<MailIcon className="h-6 w-6" />}
              title="No issues yet"
              body="The first issue goes out once something is published on Insights."
            />
          </div>
        ) : (
          <ol className="mt-6 grid gap-5 md:grid-cols-2">
            {digests.map((d) => (
              <li key={d.week} className="surface-card p-5 sm:p-6">
                <p className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">{d.week}</p>
                <h3 className="mt-1 font-display text-lg font-semibold text-ink">{weekLabel(d)}</h3>
                <div className="mt-2 divide-y divide-line">
                  {d.items.map((item) => (
                    <InsightRow key={item.id} item={item} />
                  ))}
                </div>
              </li>
            ))}
          </ol>
        )}
        <p className="mt-8 text-sm text-mute">
          Want to contribute? <Link href="/insights/new" className="font-medium text-ink underline decoration-shout/50 underline-offset-4 hover:text-shout">Write for Insights</Link>.
        </p>
      </section>
    </div>
  );
}
