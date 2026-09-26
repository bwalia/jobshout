import Link from "next/link";
import type { Metadata } from "next";
import { PenIcon, RssIcon, SearchIcon, SparkIcon } from "@/components/icons";
import { InsightCard, InsightHero } from "@/components/insights/InsightCard";
import {
  InsightSearch,
  KindTabs,
  TopicChips,
  insightHref,
  type InsightFilterState,
} from "@/components/insights/InsightFilters";
import { NewsletterSignup } from "@/components/insights/NewsletterSignup";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import {
  KIND_META,
  isInsightKind,
  listInsights,
  listTopics,
  type Insight,
  type InsightTopic,
} from "@/lib/insights";

export const dynamic = "force-dynamic";

const PAGE_SIZE = 12;

type SearchParams = Record<string, string | string[] | undefined>;

function one(v: string | string[] | undefined): string {
  return (Array.isArray(v) ? v[0] : v)?.trim() ?? "";
}

export async function generateMetadata({
  searchParams,
}: {
  searchParams: SearchParams;
}): Promise<Metadata> {
  const kind = one(searchParams.kind);
  const title = isInsightKind(kind) ? `${KIND_META[kind].plural} · Insights` : "Insights";
  return {
    title,
    description:
      "AI trends, job-market shifts, and practical analysis for candidates and employers — articles, posts, podcasts and videos from JobShout.",
    alternates: {
      canonical: "/insights",
      types: { "application/rss+xml": "/insights/rss.xml" },
    },
  };
}

export default async function InsightsPage({ searchParams }: { searchParams: SearchParams }) {
  const kindParam = one(searchParams.kind);
  const filters: InsightFilterState = {
    kind: isInsightKind(kindParam) ? kindParam : null,
    topic: one(searchParams.topic) || null,
    q: one(searchParams.q),
  };
  const page = Math.max(1, Number.parseInt(one(searchParams.page) || "1", 10) || 1);
  const filtered = Boolean(filters.kind || filters.topic || filters.q);
  const showHero = !filtered && page === 1;

  let items: Insight[] = [];
  let total = 0;
  let hero: Insight | null = null;
  let topics: InsightTopic[] = [];
  let error = "";
  try {
    const [list, topicList, featured] = await Promise.all([
      listInsights({ ...filters, limit: PAGE_SIZE + (showHero ? 1 : 0), offset: (page - 1) * PAGE_SIZE }),
      listTopics(),
      showHero ? listInsights({ featured: true, limit: 1 }) : Promise.resolve(null),
    ]);
    topics = topicList;
    total = list.total;
    hero = featured?.data[0] ?? (showHero ? (list.data[0] ?? null) : null);
    items = list.data.filter((i) => i.id !== hero?.id).slice(0, PAGE_SIZE);
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load insights";
  }

  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const topicName = topics.find((t) => t.slug === filters.topic)?.name;

  return (
    <div>
      <section className="relative overflow-hidden border-b border-line">
        <div aria-hidden className="pointer-events-none absolute inset-0 halo" />
        <div aria-hidden className="pointer-events-none absolute inset-0 grid-field" />
        <div className="relative mx-auto max-w-board px-5 pb-10 pt-12 sm:px-8 sm:pb-12 sm:pt-16">
          <div className="flex flex-wrap items-end justify-between gap-6">
            <div className="max-w-2xl">
              <Badge tone="brand">
                <SparkIcon className="h-3.5 w-3.5" />
                JobShout Insights
              </Badge>
              <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-[3.25rem] sm:leading-[1.05]">
                AI, work and the job market
              </h1>
              <p className="mt-4 text-base leading-relaxed text-mute sm:text-lg">
                What is changing in AI, what it means for hiring and careers, and what to do about it.
                Articles, quick posts, podcasts and videos from the JobShout team and community.
              </p>
            </div>
            <div className="flex flex-wrap gap-2.5">
              <Link href="/insights/new" className={buttonClass("primary", "md")}>
                <PenIcon className="h-4 w-4" />
                Write for Insights
              </Link>
              <a href="/insights/rss.xml" className={buttonClass("secondary", "md")}>
                <RssIcon className="h-4 w-4" />
                RSS
              </a>
            </div>
          </div>
        </div>
      </section>

      <div className="mx-auto max-w-board px-5 pb-8 pt-8 sm:px-8">
        {error ? (
          <ErrorNotice
            title={error}
            body="Insights could not reach the marketplace API. Check it is running, then refresh."
          />
        ) : null}

        {hero ? (
          <div className="animate-rise-in">
            <InsightHero item={hero} />
          </div>
        ) : null}

        <div className={hero ? "mt-10" : ""}>
          <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
            <KindTabs filters={filters} />
            <InsightSearch filters={filters} />
          </div>
          {topics.length ? (
            <div className="mt-4">
              <TopicChips topics={topics} filters={filters} />
            </div>
          ) : null}
        </div>

        <section aria-labelledby="latest-heading" className="mt-8">
          <div className="flex flex-wrap items-baseline justify-between gap-3 border-b border-line pb-4">
            <h2 id="latest-heading" className="font-display text-xl font-semibold text-ink">
              {filters.q
                ? `Results for “${filters.q}”`
                : filters.kind
                  ? `Latest ${KIND_META[filters.kind].plural.toLowerCase()}`
                  : "Latest"}
              {topicName ? <span className="text-mute"> in {topicName}</span> : null}
            </h2>
            <p className="text-sm text-mute">
              {total} {total === 1 ? "item" : "items"}
              {filtered ? (
                <>
                  {" · "}
                  <Link href="/insights" className="font-medium text-ink underline decoration-line underline-offset-4 hover:text-shout">
                    Clear filters
                  </Link>
                </>
              ) : null}
            </p>
          </div>

          {items.length > 0 ? (
            <ul className="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
              {items.map((item) => (
                <li key={item.id}>
                  <InsightCard item={item} />
                </li>
              ))}
            </ul>
          ) : !error && !hero ? (
            <div className="mt-6">
              {filtered ? (
                <EmptyState
                  icon={<SearchIcon className="h-6 w-6" />}
                  title="Nothing matches that yet"
                  body="Try another format or topic, or search for something broader."
                  action={
                    <Link href="/insights" className={buttonClass("primary", "md")}>
                      See everything
                    </Link>
                  }
                />
              ) : (
                <EmptyState
                  icon={<PenIcon className="h-6 w-6" />}
                  title="No insights published yet"
                  body="Be the first to share what you are seeing in AI and the job market."
                  action={
                    <Link href="/insights/new" className={buttonClass("primary", "md")}>
                      Write the first one
                    </Link>
                  }
                />
              )}
            </div>
          ) : null}

          {pages > 1 ? (
            <nav aria-label="Pagination" className="mt-10 flex items-center justify-between gap-4">
              {page > 1 ? (
                <Link
                  href={`${insightHref(filters)}${insightHref(filters).includes("?") ? "&" : "?"}page=${page - 1}`}
                  className={buttonClass("secondary", "md")}
                >
                  Newer
                </Link>
              ) : (
                <span />
              )}
              <span className="text-sm text-mute">
                Page {page} of {pages}
              </span>
              {page < pages ? (
                <Link
                  href={`${insightHref(filters)}${insightHref(filters).includes("?") ? "&" : "?"}page=${page + 1}`}
                  className={buttonClass("secondary", "md")}
                >
                  Older
                </Link>
              ) : (
                <span />
              )}
            </nav>
          ) : null}
        </section>

        <div className="mt-16">
          <NewsletterSignup />
        </div>
      </div>
    </div>
  );
}
