import Link from "next/link";
import type { Metadata } from "next";
import { notFound, redirect } from "next/navigation";
import {
  ArrowLeftIcon,
  ArrowUpRightIcon,
  BookIcon,
  BotIcon,
  CodeIcon,
  GlobeIcon,
  LayersIcon,
  PenIcon,
  ShieldIcon,
} from "@/components/icons";
import { JobCard } from "@/components/JobCard";
import { ShareLinks } from "@/components/insights/ShareLinks";
import { AgentCard, AppCard, AppLogo } from "@/components/showcase/AppCard";
import { ShowcaseReviewActions } from "@/components/showcase/ShowcaseReviewActions";
import { StarButton } from "@/components/showcase/StarButton";
import { Badge, buttonClass, cx } from "@/components/ui";
import { formatDate, isEditor, siteUrl } from "@/lib/insights";
import {
  CAPABILITIES,
  KINDS,
  PRICING,
  STATUS_LABELS,
  VERIFICATION_LABELS,
  VISIBILITY,
  directoryHref,
  getApp,
  isCapability,
  usedIn,
  type ShowcaseApp,
} from "@/lib/showcase";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

type Params = { params: { slug: string } };

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const e = await getApp(params.slug, await currentViewer()).catch(() => null);
  if (!e) return { title: "Not found" };
  const indexable = e.status === "published" && e.visibility === "public";
  return {
    title: `${e.name} · ${e.kind === "team" ? "Agent team" : "AI agent"}`,
    description: e.tagline || undefined,
    alternates: { canonical: `/agents/${e.slug}` },
    robots: indexable ? undefined : { index: false, follow: false },
    openGraph: {
      type: "website",
      title: e.name,
      description: e.tagline || undefined,
      url: `/agents/${e.slug}`,
      images: e.logo_url ? [{ url: e.logo_url }] : [],
    },
  };
}

function Chips({ title, items, href }: { title: string; items: string[]; href?: (item: string) => string }) {
  if (!items.length) return null;
  return (
    <section className="mt-8">
      <h2 className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">{title}</h2>
      <ul className="mt-3 flex flex-wrap gap-2">
        {items.map((t) => (
          <li key={t}>
            {href ? (
              <Link
                href={href(t)}
                className="inline-flex min-h-[36px] items-center rounded-pill border border-line bg-surface px-3.5 text-xs font-medium text-body transition-colors duration-200 hover:border-edge hover:text-ink"
              >
                {t}
              </Link>
            ) : (
              <span className="inline-flex min-h-[36px] items-center rounded-pill border border-line bg-surface px-3.5 text-xs font-medium text-body">
                {t}
              </span>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

/** A team's members as the flow of work, first to last. */
function Workflow({ team }: { team: ShowcaseApp }) {
  if (!team.linked_agents.length) {
    return <p className="mt-8 text-sm text-mute">No members listed yet.</p>;
  }
  return (
    <section aria-labelledby="flow-heading" className="mt-10">
      <h2 id="flow-heading" className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
        How the work flows
      </h2>
      <ol className="mt-4">
        {team.linked_agents.map((a, i) => (
          <li key={a.slug} className="relative flex gap-4 pb-5 last:pb-0">
            {i < team.linked_agents.length - 1 ? (
              <span aria-hidden className="absolute left-[23px] top-12 h-[calc(100%-3rem)] w-px bg-line" />
            ) : null}
            <AppLogo app={a} />
            <div className="min-w-0 flex-1 rounded-card border border-line bg-surface p-4 transition-colors duration-200 hover:border-edge">
              <p className="text-xs font-semibold uppercase tracking-[0.12em] text-mute">Step {i + 1}</p>
              <h3 className="mt-1 break-words font-display text-base font-semibold text-ink">
                <Link href={`/agents/${a.slug}`} className="hover:text-shout">
                  {a.name}
                </Link>
              </h3>
              {a.role ? <p className="text-sm text-body">{a.role}</p> : null}
              {a.tagline ? <p className="mt-1 line-clamp-2 text-xs text-mute">{a.tagline}</p> : null}
            </div>
          </li>
        ))}
      </ol>
    </section>
  );
}

export default async function AgentPage({ params }: Params) {
  const viewer = await currentViewer();
  const e = await getApp(params.slug, viewer);
  if (!e) notFound();
  if (e.kind === "app") redirect(`/showcase/${e.slug}`);

  const live = e.status === "published";
  const [editor, used] = await Promise.all([
    isEditor(viewer),
    live ? usedIn(e.slug, viewer) : Promise.resolve({ apps: [], teams: [] }),
  ]);
  const owner = Boolean(viewer && e.creator_email?.toLowerCase() === viewer.email.toLowerCase());
  const canEdit = editor || (owner && e.status !== "archived");
  const url = `${siteUrl()}/agents/${e.slug}`;
  const team = e.kind === "team";
  const caps = e.capabilities.filter(isCapability);
  const details = (
    [
      ["Model", e.ai_models.join(", ")],
      ["Provider", e.model_provider],
      ["Pricing", PRICING[e.pricing]],
      ["Licence", e.license],
      ["Version", e.version],
      ["Updated", formatDate(e.updated_at)],
    ] as Array<[string, string]>
  ).filter(([, v]) => v);

  return (
    <article className="mx-auto max-w-board px-5 pb-16 pt-8 sm:px-8 sm:pt-12">
      <Link
        href={directoryHref({ kind: e.kind })}
        className="inline-flex min-h-[44px] items-center gap-2 text-sm font-medium text-mute transition-colors duration-200 hover:text-ink"
      >
        <ArrowLeftIcon className="h-4 w-4" />
        {team ? "Agent teams" : "Agent directory"}
      </Link>

      {!live || editor || owner ? (
        <div className="mt-4 surface-card flex flex-col gap-4 border-warn/30 bg-warn/[0.06] p-4 sm:flex-row sm:items-start sm:justify-between sm:p-5">
          <div className="min-w-0">
            <p className="text-sm font-semibold text-ink">
              {STATUS_LABELS[e.status]}
              {e.featured ? " · Featured" : ""} · {VISIBILITY[e.visibility].label}
              {!live ? (
                <span className="font-normal text-mute"> — only you{editor ? " and other editors" : " and the editors"} can see this.</span>
              ) : null}
            </p>
            {e.review_note ? (
              <p className="mt-1.5 text-sm leading-relaxed text-body">
                <span className="font-semibold">Editor’s note:</span> {e.review_note}
              </p>
            ) : null}
          </div>
          <div className="flex shrink-0 flex-col gap-3 sm:items-end">
            {canEdit ? (
              <Link href={`/showcase/${e.slug}/edit`} className={buttonClass("secondary", "sm")}>
                <PenIcon className="h-4 w-4" />
                Edit
              </Link>
            ) : null}
            {editor ? <ShowcaseReviewActions id={e.id} status={e.status} featured={e.featured} /> : null}
          </div>
        </div>
      ) : null}

      <header className="mt-6 flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 gap-4 sm:gap-5">
          <AppLogo app={e} size="lg" />
          <div className="min-w-0">
            <h1 className="break-words font-display text-3xl font-semibold leading-[1.1] tracking-[-0.025em] text-ink sm:text-[2.75rem]">
              {e.name}
            </h1>
            {e.tagline ? <p className="mt-2 text-lg leading-relaxed text-mute">{e.tagline}</p> : null}
            <div className="mt-4 flex flex-wrap items-center gap-2">
              <Badge tone="signal">
                {team ? <LayersIcon className="h-3 w-3" /> : <BotIcon className="h-3 w-3" />}
                {KINDS[e.kind].label}
              </Badge>
              {team ? <Badge>{e.linked_agents.length} agents</Badge> : null}
              {e.pricing === "open_source" ? <Badge>{PRICING.open_source}</Badge> : null}
              <Badge tone={e.verification === "unverified" ? "neutral" : "good"}>
                <ShieldIcon className="h-3 w-3" />
                {VERIFICATION_LABELS[e.verification]}
              </Badge>
            </div>
            <p className="mt-3 text-sm text-mute">
              by <span className="font-semibold text-ink">{e.creator_display_name}</span>
              {e.team_name ? <> · {e.team_name}</> : null}
              {" · "}used in {e.used_in} {e.used_in === 1 ? "app" : "apps"}
            </p>
          </div>
        </div>
        {live ? (
          <div className="flex shrink-0 flex-wrap items-start gap-2.5">
            <StarButton slug={e.slug} starred={e.starred} count={e.star_count} signedIn={Boolean(viewer)} />
          </div>
        ) : null}
      </header>

      <div className="mt-6 flex flex-wrap gap-2.5">
        {(
          [
            [e.repo_url, <CodeIcon key="c" className="h-4 w-4" />, "Source"],
            [e.docs_url, <BookIcon key="b" className="h-4 w-4" />, "Docs"],
            [e.website_url, <GlobeIcon key="g" className="h-4 w-4" />, "Website"],
            [e.demo_url, <ArrowUpRightIcon key="a" className="h-4 w-4" />, "Try it"],
          ] as const
        )
          .filter(([href]) => href)
          .map(([href, icon, label], i) => (
            <a
              key={label}
              href={href}
              target="_blank"
              rel="noopener noreferrer nofollow ugc"
              className={buttonClass(i === 0 ? "primary" : "secondary", "md")}
            >
              {icon}
              {label}
              <ArrowUpRightIcon className="h-3.5 w-3.5 opacity-60" />
            </a>
          ))}
      </div>

      <div className="mt-10 grid grid-cols-[minmax(0,1fr)] gap-12 lg:grid-cols-[minmax(0,1fr)_21rem] lg:gap-14">
        <div className="min-w-0 max-w-prose">
          {e.description_html ? (
            <div
              className="insight-prose"
              // Sanitised by the API (ammonia) when the entry was saved.
              dangerouslySetInnerHTML={{ __html: e.description_html }}
            />
          ) : (
            <p className="text-mute">No description yet.</p>
          )}

          {team ? (
            <Workflow team={e} />
          ) : (
            <>
              {caps.length ? (
                <section className="mt-10">
                  <h2 className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">Capabilities</h2>
                  <ul className="mt-3 flex flex-wrap gap-2">
                    {caps.map((c) => (
                      <li key={c}>
                        <Link href={directoryHref({ capability: c })} className="rounded-pill">
                          <Badge tone="signal" className="min-h-[32px] px-3.5">
                            {CAPABILITIES[c]}
                          </Badge>
                        </Link>
                      </li>
                    ))}
                  </ul>
                </section>
              ) : null}
              <Chips title="Skills" items={e.technologies} href={(t) => directoryHref({ tech: t })} />
              <Chips title="Tools" items={e.tools} />
              <Chips title="MCP servers" items={e.mcp_servers} />
            </>
          )}

          {live ? (
            <div className="mt-10 border-t border-line pt-5">
              <ShareLinks url={url} title={`${e.name} in the JobShout agent directory`} />
            </div>
          ) : null}
        </div>

        <aside className="space-y-6 lg:sticky lg:top-24 lg:self-start">
          <section aria-labelledby="details-heading" className="surface-card p-5 sm:p-6">
            <h2 id="details-heading" className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
              Details
            </h2>
            <dl className="mt-3 divide-y divide-line text-sm">
              {details.map(([k, v]) => (
                <div key={k} className="flex justify-between gap-4 py-2">
                  <dt className="text-mute">{k}</dt>
                  <dd className="min-w-0 break-words text-right font-medium text-ink">{v}</dd>
                </div>
              ))}
            </dl>
            {e.human_oversight ? (
              <p className="mt-4 border-t border-line pt-3 text-xs leading-relaxed text-mute">
                <span className="font-semibold text-body">Human oversight:</span> {e.human_oversight}
              </p>
            ) : null}
            <p className="mt-3 text-xs leading-relaxed text-mute">
              Everything here is the creator&apos;s own account. {VERIFICATION_LABELS[e.verification]} means JobShout
              has not checked it yet.
            </p>
          </section>
        </aside>
      </div>

      {e.jobs.length ? (
        <section aria-labelledby="roles-heading" className="mt-16 border-t border-line pt-10">
          <h2 id="roles-heading" className="font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
            Open roles
          </h2>
          <p className="mt-1.5 text-sm text-mute">
            {e.team_name || e.creator_display_name} is hiring to work with this {team ? "team" : "agent"}.
          </p>
          <ul className="mt-6 grid gap-4 md:grid-cols-2">
            {e.jobs.map((job) => (
              <li key={job.id}>
                <JobCard job={job} />
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {used.apps.length ? (
        <section aria-labelledby="used-heading" className="mt-16 border-t border-line pt-10">
          <h2 id="used-heading" className="font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
            Used in {used.apps.length} {used.apps.length === 1 ? "app" : "apps"}
          </h2>
          <p className="mt-1.5 text-sm text-mute">
            {team ? "Apps this team built." : "Apps that list this agent, directly or through a team."}
          </p>
          <ul className="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {used.apps.map((a) => (
              <li key={a.id}>
                <AppCard app={a} />
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {used.teams.length ? (
        <section aria-labelledby="teams-heading" className={cx("border-t border-line pt-10", used.apps.length ? "mt-12" : "mt-16")}>
          <h2 id="teams-heading" className="font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
            Member of {used.teams.length} {used.teams.length === 1 ? "team" : "teams"}
          </h2>
          <ul className="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {used.teams.map((t) => (
              <li key={t.id}>
                <AgentCard app={t} />
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </article>
  );
}
