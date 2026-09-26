import Link from "next/link";
import type { Metadata } from "next";
import { BotIcon, BriefcaseIcon, PlusIcon, RocketIcon, SearchIcon, SparkIcon } from "@/components/icons";
import { AppCard } from "@/components/showcase/AppCard";
import { Badge, EmptyState, ErrorNotice, buttonClass, cx } from "@/components/ui";
import {
  APP_TYPES,
  COLLECTIONS,
  isAppType,
  isCollection,
  isSortKey,
  listApps,
  listTags,
  showcaseHref,
  type Collection,
  type ShowcaseApp,
  type ShowcaseQuery,
  type ShowcaseTag,
  type SortKey,
} from "@/lib/showcase";
import { currentViewer } from "@/lib/session";

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
  const collection = one(searchParams.collection);
  return {
    title: isCollection(collection) ? `${COLLECTIONS[collection].label} · AI Showcase` : "AI Showcase",
    description:
      "Discover applications, agents and production software built by people and AI. See what AI is building on JobShout.",
    alternates: { canonical: "/showcase" },
  };
}

const TAB =
  "inline-flex min-h-[44px] shrink-0 items-center gap-2 rounded-pill px-4 text-sm transition-colors duration-200";

function CollectionTabs({ filters }: { filters: ShowcaseQuery }) {
  const tabs: Array<{ key: Collection | null; label: string }> = [
    { key: null, label: "All" },
    ...(Object.keys(COLLECTIONS) as Collection[]).map((k) => ({ key: k, label: COLLECTIONS[k].label })),
  ];
  return (
    <nav aria-label="Collections" className="-mx-5 overflow-x-auto px-5 sm:mx-0 sm:px-0">
      <ul className="flex w-max gap-1.5 rounded-pill border border-line bg-surface p-1.5">
        {tabs.map((t) => {
          const active = (filters.collection ?? null) === t.key;
          return (
            <li key={t.label}>
              <Link
                href={showcaseHref({ ...filters, collection: t.key })}
                aria-current={active ? "page" : undefined}
                className={cx(
                  TAB,
                  active ? "bg-ink font-semibold text-bg" : "font-medium text-body hover:bg-raised hover:text-ink",
                )}
              >
                {t.label}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

function Search({ filters }: { filters: ShowcaseQuery }) {
  return (
    <form action="/showcase" role="search" className="relative w-full sm:max-w-sm">
      {filters.collection ? <input type="hidden" name="collection" value={filters.collection} /> : null}
      {filters.type ? <input type="hidden" name="type" value={filters.type} /> : null}
      {filters.tech ? <input type="hidden" name="tech" value={filters.tech} /> : null}
      <label htmlFor="showcase-q" className="sr-only">
        Search the showcase
      </label>
      <SearchIcon className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-mute" />
      <input
        id="showcase-q"
        name="q"
        type="search"
        defaultValue={filters.q ?? ""}
        placeholder="Search apps, agents, technologies…"
        className="h-11 w-full rounded-pill border border-line bg-surface pl-10 pr-4 text-sm text-ink placeholder:text-mute/70 transition-colors duration-200 hover:border-edge focus:border-shout focus:outline-none focus:ring-2 focus:ring-shout/25"
      />
    </form>
  );
}

function Chips({
  label,
  items,
}: {
  label: string;
  items: Array<{ key: string; label: string; href: string; active: boolean; title?: string }>;
}) {
  return (
    <nav aria-label={label}>
      <ul className="flex flex-wrap gap-2">
        {items.map((c) => (
          <li key={c.key}>
            <Link
              href={c.href}
              aria-current={c.active ? "true" : undefined}
              title={c.title}
              className={cx(
                "inline-flex min-h-[36px] items-center rounded-pill border px-3.5 text-xs font-medium transition-colors duration-200",
                c.active
                  ? "border-shout/40 bg-shout/10 text-ink"
                  : "border-line bg-surface text-body hover:border-edge hover:text-ink",
              )}
            >
              {c.label}
              {c.active ? (
                <span className="ml-1.5 text-mute" aria-label="remove filter">
                  ×
                </span>
              ) : null}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function Shelf({
  id,
  title,
  lead,
  icon,
  apps,
  href,
}: {
  id: string;
  title: string;
  lead: string;
  icon: React.ReactNode;
  apps: ShowcaseApp[];
  href: string;
}) {
  if (!apps.length) return null;
  return (
    <section aria-labelledby={id} className="mt-12">
      <div className="flex flex-wrap items-end justify-between gap-3 border-b border-line pb-4">
        <div>
          <h2 id={id} className="flex items-center gap-2 font-display text-xl font-semibold text-ink">
            <span className="text-shout">{icon}</span>
            {title}
          </h2>
          <p className="mt-1 text-sm text-mute">{lead}</p>
        </div>
        <Link href={href} className="text-sm font-medium text-ink underline decoration-line underline-offset-4 hover:text-shout">
          See all
        </Link>
      </div>
      <ul className="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
        {apps.map((app) => (
          <li key={app.id}>
            <AppCard app={app} />
          </li>
        ))}
      </ul>
    </section>
  );
}

export default async function ShowcasePage({ searchParams }: { searchParams: SearchParams }) {
  const viewer = await currentViewer();
  const collection = one(searchParams.collection);
  const type = one(searchParams.type);
  const sort = one(searchParams.sort);
  const filters: ShowcaseQuery = {
    collection: isCollection(collection) ? collection : null,
    type: isAppType(type) ? type : null,
    tech: one(searchParams.tech) || null,
    q: one(searchParams.q) || null,
    sort: isSortKey(sort) ? sort : "new",
  };
  const page = Math.max(1, Number.parseInt(one(searchParams.page) || "1", 10) || 1);
  const filtered = Boolean(filters.collection || filters.type || filters.tech || filters.q);
  const front = !filtered && page === 1 && filters.sort === "new";

  let apps: ShowcaseApp[] = [];
  let total = 0;
  let tags: ShowcaseTag[] = [];
  let shelves: Record<"featured" | "agents" | "production" | "hiring", ShowcaseApp[]> = {
    featured: [],
    agents: [],
    production: [],
    hiring: [],
  };
  let error = "";
  try {
    const shelf = (c: Collection) =>
      front ? listApps({ collection: c, sort: "stars", limit: 3 }, viewer).then((r) => r.data) : Promise.resolve([]);
    const [list, tagList, featured, agents, production, hiring] = await Promise.all([
      listApps({ ...filters, limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE }, viewer),
      listTags(16),
      shelf("featured"),
      shelf("built_by_agents"),
      shelf("production_ready"),
      shelf("hiring"),
    ]);
    apps = list.data;
    total = list.total;
    tags = tagList;
    shelves = { featured, agents, production, hiring };
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load the showcase";
  }

  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const heading = filters.q
    ? `Results for “${filters.q}”`
    : filters.collection
      ? COLLECTIONS[filters.collection].label
      : front
        ? "Recently published"
        : "All apps";
  const sortLinks: Array<{ key: SortKey; label: string }> = [
    { key: "new", label: "Newest" },
    { key: "stars", label: "Most starred" },
    { key: "updated", label: "Recently updated" },
  ];

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
                AI Showcase
              </Badge>
              <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-[3.25rem] sm:leading-[1.05]">
                See what AI is building
              </h1>
              <p className="mt-4 text-base leading-relaxed text-mute sm:text-lg">
                Applications, agents and production software built by people and AI — with who built what, the
                stack behind it, and the evidence behind every production claim.
              </p>
            </div>
            <div className="flex flex-wrap gap-2.5">
              <Link href="/showcase/new" className={buttonClass("primary", "md")}>
                <PlusIcon className="h-4 w-4" />
                Showcase your app
              </Link>
              {viewer ? (
                <Link href="/showcase/mine" className={buttonClass("secondary", "md")}>
                  Your apps
                </Link>
              ) : null}
            </div>
          </div>
          <div className="mt-8">
            <Search filters={filters} />
          </div>
        </div>
      </section>

      <div className="mx-auto max-w-board px-5 pb-8 pt-8 sm:px-8">
        {error ? (
          <ErrorNotice title={error} body="The showcase could not reach the marketplace API. Check it is running, then refresh." />
        ) : null}

        <div className="flex flex-col gap-4">
          <CollectionTabs filters={filters} />
          <Chips
            label="Filter by kind of app"
            items={(["ai_application", "agent_application", "mcp_server", "rag_application", "developer_tool", "saas", "infrastructure"] as const).map(
              (t) => ({
                key: t,
                label: APP_TYPES[t],
                href: showcaseHref({ ...filters, type: filters.type === t ? null : t }),
                active: filters.type === t,
              }),
            )}
          />
          {tags.length ? (
            <Chips
              label="Filter by technology"
              items={tags.map((t) => ({
                key: t.name,
                label: t.name,
                title: `${t.count} ${t.count === 1 ? "app" : "apps"}`,
                href: showcaseHref({
                  ...filters,
                  tech: filters.tech?.toLowerCase() === t.name.toLowerCase() ? null : t.name,
                }),
                active: filters.tech?.toLowerCase() === t.name.toLowerCase(),
              }))}
            />
          ) : null}
        </div>

        {front ? (
          <>
            <Shelf
              id="featured-heading"
              title="Featured"
              lead={COLLECTIONS.featured.blurb}
              icon={<SparkIcon className="h-5 w-5" />}
              apps={shelves.featured}
              href={showcaseHref({ collection: "featured" })}
            />
            <Shelf
              id="agents-heading"
              title="Built by AI agents"
              lead={COLLECTIONS.built_by_agents.blurb}
              icon={<BotIcon className="h-5 w-5" />}
              apps={shelves.agents}
              href={showcaseHref({ collection: "built_by_agents" })}
            />
            <Shelf
              id="production-heading"
              title="Production ready"
              lead={COLLECTIONS.production_ready.blurb}
              icon={<RocketIcon className="h-5 w-5" />}
              apps={shelves.production}
              href={showcaseHref({ collection: "production_ready" })}
            />
            <Shelf
              id="hiring-heading"
              title="Hiring now"
              lead={COLLECTIONS.hiring.blurb}
              icon={<BriefcaseIcon className="h-5 w-5" />}
              apps={shelves.hiring}
              href={showcaseHref({ collection: "hiring" })}
            />
          </>
        ) : null}

        <section aria-labelledby="all-heading" className="mt-12">
          <div className="flex flex-wrap items-baseline justify-between gap-3 border-b border-line pb-4">
            <h2 id="all-heading" className="font-display text-xl font-semibold text-ink">
              {heading}
              {filters.type ? <span className="text-mute"> · {APP_TYPES[filters.type]}</span> : null}
              {filters.tech ? <span className="text-mute"> · {filters.tech}</span> : null}
            </h2>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-mute">
              <nav aria-label="Sort" className="flex gap-3">
                {sortLinks.map((s) => (
                  <Link
                    key={s.key}
                    href={showcaseHref({ ...filters, sort: s.key })}
                    aria-current={filters.sort === s.key ? "true" : undefined}
                    className={cx(
                      "min-h-[32px] underline-offset-4 hover:text-ink",
                      filters.sort === s.key ? "font-semibold text-ink underline decoration-shout" : "",
                    )}
                  >
                    {s.label}
                  </Link>
                ))}
              </nav>
              <span>
                {total} {total === 1 ? "app" : "apps"}
                {filtered ? (
                  <>
                    {" · "}
                    <Link href="/showcase" className="font-medium text-ink underline decoration-line underline-offset-4 hover:text-shout">
                      Clear filters
                    </Link>
                  </>
                ) : null}
              </span>
            </div>
          </div>
          {filters.collection === "production_ready" ? (
            <p className="mt-4 text-sm text-mute">
              Production claims are the creator&apos;s own, backed by the evidence they list on each page. JobShout
              verification is coming.
            </p>
          ) : null}

          {apps.length ? (
            <ul className="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
              {apps.map((app) => (
                <li key={app.id}>
                  <AppCard app={app} />
                </li>
              ))}
            </ul>
          ) : !error ? (
            <div className="mt-6">
              {filtered ? (
                <EmptyState
                  icon={<SearchIcon className="h-6 w-6" />}
                  title="Nothing matches that yet"
                  body="Try another collection or technology, or search for something broader."
                  action={
                    <Link href="/showcase" className={buttonClass("primary", "md")}>
                      See everything
                    </Link>
                  }
                />
              ) : (
                <EmptyState
                  icon={<RocketIcon className="h-6 w-6" />}
                  title="No apps published yet"
                  body="Built something with AI? Be the first to show it."
                  action={
                    <Link href="/showcase/new" className={buttonClass("primary", "md")}>
                      Showcase your app
                    </Link>
                  }
                />
              )}
            </div>
          ) : null}

          {pages > 1 ? (
            <nav aria-label="Pagination" className="mt-10 flex items-center justify-between gap-4">
              {page > 1 ? (
                <Link href={showcaseHref({ ...filters, page: page - 1 })} className={buttonClass("secondary", "md")}>
                  Previous
                </Link>
              ) : (
                <span />
              )}
              <span className="text-sm text-mute">
                Page {page} of {pages}
              </span>
              {page < pages ? (
                <Link href={showcaseHref({ ...filters, page: page + 1 })} className={buttonClass("secondary", "md")}>
                  Next
                </Link>
              ) : (
                <span />
              )}
            </nav>
          ) : null}
        </section>
      </div>
    </div>
  );
}
