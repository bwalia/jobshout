import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ArrowLeftIcon, BotIcon, LayersIcon, PlusIcon, RocketIcon } from "@/components/icons";
import { AgentCard, AppCard } from "@/components/showcase/AppCard";
import { Badge, EmptyState, buttonClass, cx } from "@/components/ui";
import {
  CROSS_INDUSTRY,
  directoryHref,
  findIndustry,
  industryHref,
  industryNames,
  listApps,
  listIndustries,
  showcaseHref,
  type IndustryNames,
  type IndustryNode,
  type ShowcaseApp,
} from "@/lib/showcase";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

type Params = { params: { slug: string } };

async function load(slug: string) {
  const tree = await listIndustries().catch(() => [] as IndustryNode[]);
  const found = findIndustry(tree, slug.toLowerCase());
  return { tree, found: found && found.node.slug !== CROSS_INDUSTRY ? found : null };
}

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const { found } = await load(params.slug);
  if (!found) return { title: "Not found" };
  const { node, parent } = found;
  const name = parent ? `${node.name} (${parent.name})` : node.name;
  return {
    title: `AI apps and agents for ${name} · AI Showcase`,
    description: `${node.description} AI applications, agents and agent teams built for ${node.name.toLowerCase()}, on the JobShout AI Showcase.`,
    alternates: { canonical: industryHref(node.slug) },
  };
}

function Shelf({
  id,
  title,
  icon,
  entries,
  href,
  total,
  noun,
  names,
}: {
  id: string;
  title: string;
  icon: React.ReactNode;
  entries: ShowcaseApp[];
  href: string;
  total: number;
  noun: string;
  names: IndustryNames;
}) {
  if (!entries.length) return null;
  return (
    <section aria-labelledby={id} className="mt-12">
      <div className="flex flex-wrap items-end justify-between gap-3 border-b border-line pb-4">
        <h2 id={id} className="flex items-center gap-2 font-display text-xl font-semibold text-ink">
          <span className="text-shout">{icon}</span>
          {title}
          <span className="text-base font-normal text-mute">{total}</span>
        </h2>
        {total > entries.length ? (
          <Link href={href} className="text-sm font-medium text-ink underline decoration-line underline-offset-4 hover:text-shout">
            See all {total} {noun}s
          </Link>
        ) : null}
      </div>
      <ul className="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
        {entries.map((e) => (
          <li key={e.id}>{e.kind === "app" ? <AppCard app={e} industries={names} /> : <AgentCard app={e} industries={names} />}</li>
        ))}
      </ul>
    </section>
  );
}

export default async function IndustryPage({ params }: Params) {
  const { tree, found } = await load(params.slug);
  if (!found) notFound();
  const { node, parent } = found;
  const top = parent ?? node;
  const viewer = await currentViewer();
  const scope = { industry: node.slug, cross: false };
  const empty = { data: [] as ShowcaseApp[], total: 0 };
  const [apps, agents, teams] = await Promise.all([
    listApps({ ...scope, kind: "app", sort: "stars", limit: 6 }, viewer).catch(() => empty),
    listApps({ ...scope, kind: "agent", sort: "stars", limit: 6 }, viewer).catch(() => empty),
    listApps({ ...scope, kind: "team", sort: "stars", limit: 3 }, viewer).catch(() => empty),
  ]);
  const names = industryNames(tree);
  const nothing = !apps.total && !agents.total && !teams.total;

  return (
    <div>
      <section className="relative overflow-hidden border-b border-line">
        <div aria-hidden className="pointer-events-none absolute inset-0 halo" />
        <div aria-hidden className="pointer-events-none absolute inset-0 grid-field" />
        <div className="relative mx-auto max-w-board px-5 pb-10 pt-8 sm:px-8 sm:pb-12 sm:pt-12">
          <nav aria-label="Breadcrumb" className="text-sm text-mute">
            <ol className="flex flex-wrap items-center gap-1.5">
              <li>
                <Link href="/showcase" className="inline-flex min-h-[32px] items-center gap-1.5 hover:text-ink">
                  <ArrowLeftIcon className="h-3.5 w-3.5" />
                  AI Showcase
                </Link>
              </li>
              {parent ? (
                <>
                  <li aria-hidden>/</li>
                  <li>
                    <Link href={industryHref(parent.slug)} className="hover:text-ink">
                      {parent.name}
                    </Link>
                  </li>
                </>
              ) : null}
            </ol>
          </nav>
          <div className="mt-5 max-w-2xl">
            <Badge tone="brand">{parent ? `${parent.name} specialism` : "Industry"}</Badge>
            <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-[3.25rem] sm:leading-[1.05]">
              AI apps and agents for {node.name}
            </h1>
            <p className="mt-4 text-base leading-relaxed text-mute sm:text-lg">{node.description}</p>
          </div>
          {top.verticals.length ? (
            <nav aria-label={`Specialisms in ${top.name}`} className="mt-8">
              <ul className="flex flex-wrap gap-2">
                {[top, ...top.verticals].map((v) => {
                  const active = v.slug === node.slug;
                  return (
                    <li key={v.slug}>
                      <Link
                        href={industryHref(v.slug)}
                        aria-current={active ? "page" : undefined}
                        className={cx(
                          "inline-flex min-h-[36px] items-center gap-1.5 rounded-pill border px-3.5 text-xs font-medium transition-colors duration-200",
                          active ? "border-shout/40 bg-shout/10 text-ink" : "border-line bg-surface text-body hover:border-edge hover:text-ink",
                        )}
                      >
                        {v === top ? `All of ${top.name}` : v.name}
                        <span className="tabular-nums text-mute">{v.count}</span>
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </nav>
          ) : null}
        </div>
      </section>

      <div className="mx-auto max-w-board px-5 pb-16 pt-2 sm:px-8">
        <Shelf
          id="apps-heading"
          title="Apps"
          icon={<RocketIcon className="h-5 w-5" />}
          entries={apps.data}
          total={apps.total}
          noun="app"
          names={names}
          href={showcaseHref({ industry: node.slug, cross: false })}
        />
        <Shelf
          id="agents-heading"
          title="Agents"
          icon={<BotIcon className="h-5 w-5" />}
          entries={agents.data}
          total={agents.total}
          noun="agent"
          names={names}
          href={directoryHref({ industry: node.slug, cross: false })}
        />
        <Shelf
          id="teams-heading"
          title="Agent teams"
          icon={<LayersIcon className="h-5 w-5" />}
          entries={teams.data}
          total={teams.total}
          noun="team"
          names={names}
          href={directoryHref({ kind: "team", industry: node.slug, cross: false })}
        />

        {nothing ? (
          <div className="mt-12">
            <EmptyState
              icon={<RocketIcon className="h-6 w-6" />}
              title={`Nothing built for ${node.name} yet`}
              body="Built an app or agent for this sector? Be the first to show it. General-purpose tools that work here are in the main showcase."
              action={
                <div className="flex flex-wrap justify-center gap-2.5">
                  <Link href="/showcase/new" className={buttonClass("primary", "md")}>
                    <PlusIcon className="h-4 w-4" />
                    Showcase your app
                  </Link>
                  <Link href={showcaseHref({ industry: node.slug })} className={buttonClass("secondary", "md")}>
                    Include cross-industry tools
                  </Link>
                </div>
              }
            />
          </div>
        ) : (
          <p className="mt-12 text-sm text-mute">
            Looking for general-purpose tools too?{" "}
            <Link href={showcaseHref({ industry: node.slug })} className="font-medium text-ink underline decoration-line underline-offset-4 hover:text-shout">
              Browse {node.name} with cross-industry tools
            </Link>
            .
          </p>
        )}
      </div>
    </div>
  );
}
