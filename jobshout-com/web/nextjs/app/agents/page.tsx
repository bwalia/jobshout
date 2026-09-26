import Link from "next/link";
import type { Metadata } from "next";
import { BotIcon, LayersIcon, PlusIcon, SearchIcon } from "@/components/icons";
import { AgentCard } from "@/components/showcase/AppCard";
import { Badge, EmptyState, ErrorNotice, buttonClass, cx } from "@/components/ui";
import {
  CAPABILITIES,
  directoryHref,
  isCapability,
  isSortKey,
  listApps,
  listTags,
  type Capability,
  type ShowcaseApp,
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

export async function generateMetadata({ searchParams }: { searchParams: SearchParams }): Promise<Metadata> {
  const teams = one(searchParams.kind) === "team";
  return {
    title: teams ? "Agent teams · AI Showcase" : "Agent directory · AI Showcase",
    description:
      "AI agents and agent teams: what they run on, what they can do, and the apps they have built. Part of the JobShout AI Showcase.",
    alternates: { canonical: teams ? "/agents?kind=team" : "/agents" },
  };
}

const TAB =
  "inline-flex min-h-[44px] shrink-0 items-center gap-2 rounded-pill px-4 text-sm transition-colors duration-200";

export default async function AgentsPage({ searchParams }: { searchParams: SearchParams }) {
  const viewer = await currentViewer();
  const kind = one(searchParams.kind) === "team" ? "team" : "agent";
  const cap = one(searchParams.capability);
  const sort = one(searchParams.sort);
  const filters = {
    kind,
    capability: kind === "agent" && isCapability(cap) ? (cap as Capability) : null,
    tech: one(searchParams.tech) || null,
    q: one(searchParams.q) || null,
    sort: (isSortKey(sort) ? sort : "new") as SortKey,
  } as const;
  const page = Math.max(1, Number.parseInt(one(searchParams.page) || "1", 10) || 1);
  const filtered = Boolean(filters.capability || filters.tech || filters.q);

  let entries: ShowcaseApp[] = [];
  let total = 0;
  let tags: ShowcaseTag[] = [];
  let error = "";
  try {
    const [list, tagList] = await Promise.all([
      listApps({ ...filters, limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE }, viewer),
      kind === "agent" ? listTags(14, "agent") : Promise.resolve([]),
    ]);
    entries = list.data;
    total = list.total;
    tags = tagList;
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load the directory";
  }
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const noun = kind === "team" ? "team" : "agent";
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
              <Badge tone="signal">
                <BotIcon className="h-3.5 w-3.5" />
                Agent directory
              </Badge>
              <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-[3.25rem] sm:leading-[1.05]">
                The agents behind the apps
              </h1>
              <p className="mt-4 text-base leading-relaxed text-mute sm:text-lg">
                AI agents and agent teams: what they run on, what they can do, and what they have built in the{" "}
                <Link href="/showcase" className="font-medium text-ink underline decoration-shout/50 underline-offset-4 hover:text-shout">
                  AI Showcase
                </Link>
                .
              </p>
            </div>
            <div className="flex flex-wrap gap-2.5">
              <Link href="/showcase/new?kind=agent" className={buttonClass("primary", "md")}>
                <PlusIcon className="h-4 w-4" />
                Add an agent
              </Link>
              <Link href="/showcase/new?kind=team" className={buttonClass("secondary", "md")}>
                <LayersIcon className="h-4 w-4" />
                Add a team
              </Link>
            </div>
          </div>
          <form action="/agents" role="search" className="relative mt-8 w-full sm:max-w-sm">
            {kind === "team" ? <input type="hidden" name="kind" value="team" /> : null}
            {filters.capability ? <input type="hidden" name="capability" value={filters.capability} /> : null}
            <label htmlFor="agents-q" className="sr-only">
              Search the directory
            </label>
            <SearchIcon className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-mute" />
            <input
              id="agents-q"
              name="q"
              type="search"
              defaultValue={filters.q ?? ""}
              placeholder={kind === "team" ? "Search teams…" : "Search agents, skills, tools…"}
              className="h-11 w-full rounded-pill border border-line bg-surface pl-10 pr-4 text-sm text-ink placeholder:text-mute/70 transition-colors duration-200 hover:border-edge focus:border-shout focus:outline-none focus:ring-2 focus:ring-shout/25"
            />
          </form>
        </div>
      </section>

      <div className="mx-auto max-w-board px-5 pb-8 pt-8 sm:px-8">
        {error ? <ErrorNotice title={error} body="The directory could not reach the marketplace API. Refresh in a moment." /> : null}

        <div className="flex flex-col gap-4">
          <nav aria-label="Agents or teams">
            <ul className="flex w-max gap-1.5 rounded-pill border border-line bg-surface p-1.5">
              {(["agent", "team"] as const).map((k) => (
                <li key={k}>
                  <Link
                    href={directoryHref({ kind: k })}
                    aria-current={kind === k ? "page" : undefined}
                    className={cx(TAB, kind === k ? "bg-ink font-semibold text-bg" : "font-medium text-body hover:bg-raised hover:text-ink")}
                  >
                    {k === "agent" ? <BotIcon className="h-4 w-4" /> : <LayersIcon className="h-4 w-4" />}
                    {k === "agent" ? "Agents" : "Teams"}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>
          {kind === "agent" ? (
            <nav aria-label="Filter by capability">
              <ul className="flex flex-wrap gap-2">
                {(Object.keys(CAPABILITIES) as Capability[]).map((c) => {
                  const active = filters.capability === c;
                  return (
                    <li key={c}>
                      <Link
                        href={directoryHref({ ...filters, capability: active ? null : c })}
                        aria-current={active ? "true" : undefined}
                        className={cx(
                          "inline-flex min-h-[36px] items-center rounded-pill border px-3.5 text-xs font-medium transition-colors duration-200",
                          active ? "border-shout/40 bg-shout/10 text-ink" : "border-line bg-surface text-body hover:border-edge hover:text-ink",
                        )}
                      >
                        {CAPABILITIES[c]}
                        {active ? <span className="ml-1.5 text-mute" aria-label="remove filter">×</span> : null}
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </nav>
          ) : null}
          {tags.length ? (
            <nav aria-label="Filter by skill">
              <ul className="flex flex-wrap gap-2">
                {tags.map((t) => {
                  const active = filters.tech?.toLowerCase() === t.name.toLowerCase();
                  return (
                    <li key={t.name}>
                      <Link
                        href={directoryHref({ ...filters, tech: active ? null : t.name })}
                        aria-current={active ? "true" : undefined}
                        title={`${t.count} ${t.count === 1 ? "agent" : "agents"}`}
                        className={cx(
                          "inline-flex min-h-[32px] items-center rounded-pill border px-3 text-xs transition-colors duration-200",
                          active ? "border-shout/40 bg-shout/10 font-medium text-ink" : "border-dashed border-line text-mute hover:border-edge hover:text-ink",
                        )}
                      >
                        {t.name}
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </nav>
          ) : null}
        </div>

        <section aria-labelledby="dir-heading" className="mt-10">
          <div className="flex flex-wrap items-baseline justify-between gap-3 border-b border-line pb-4">
            <h2 id="dir-heading" className="font-display text-xl font-semibold text-ink">
              {filters.q ? `Results for “${filters.q}”` : kind === "team" ? "Agent teams" : "Agents"}
              {filters.capability ? <span className="text-mute"> · {CAPABILITIES[filters.capability]}</span> : null}
              {filters.tech ? <span className="text-mute"> · {filters.tech}</span> : null}
            </h2>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-mute">
              <nav aria-label="Sort" className="flex gap-3">
                {sortLinks.map((s) => (
                  <Link
                    key={s.key}
                    href={directoryHref({ ...filters, sort: s.key })}
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
                {total} {total === 1 ? noun : `${noun}s`}
                {filtered ? (
                  <>
                    {" · "}
                    <Link href={directoryHref({ kind })} className="font-medium text-ink underline decoration-line underline-offset-4 hover:text-shout">
                      Clear filters
                    </Link>
                  </>
                ) : null}
              </span>
            </div>
          </div>

          {entries.length ? (
            <ul className="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
              {entries.map((e) => (
                <li key={e.id}>
                  <AgentCard app={e} />
                </li>
              ))}
            </ul>
          ) : !error ? (
            <div className="mt-6">
              <EmptyState
                icon={filtered ? <SearchIcon className="h-6 w-6" /> : <BotIcon className="h-6 w-6" />}
                title={filtered ? "Nothing matches that yet" : `No ${noun}s listed yet`}
                body={
                  filtered
                    ? "Try another capability or skill, or search for something broader."
                    : kind === "team"
                      ? "Put agents from the directory together and show how the work flows between them."
                      : "Built an agent? List what it runs on and what it can do, then link it from the apps it built."
                }
                action={
                  <Link
                    href={filtered ? directoryHref({ kind }) : `/showcase/new?kind=${kind}`}
                    className={buttonClass("primary", "md")}
                  >
                    {filtered ? "See everything" : kind === "team" ? "Add a team" : "Add an agent"}
                  </Link>
                }
              />
            </div>
          ) : null}

          {pages > 1 ? (
            <nav aria-label="Pagination" className="mt-10 flex items-center justify-between gap-4">
              {page > 1 ? (
                <Link href={directoryHref({ ...filters, page: page - 1 })} className={buttonClass("secondary", "md")}>
                  Previous
                </Link>
              ) : (
                <span />
              )}
              <span className="text-sm text-mute">
                Page {page} of {pages}
              </span>
              {page < pages ? (
                <Link href={directoryHref({ ...filters, page: page + 1 })} className={buttonClass("secondary", "md")}>
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
