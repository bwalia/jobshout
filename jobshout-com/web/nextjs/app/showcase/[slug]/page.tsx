import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { JobCard } from "@/components/JobCard";
import {
  ArrowLeftIcon,
  ArrowUpRightIcon,
  BookIcon,
  BotIcon,
  CheckIcon,
  CodeIcon,
  GlobeIcon,
  PenIcon,
  PlayIcon,
  ShieldIcon,
  UserIcon,
  XIcon,
} from "@/components/icons";
import { ShareLinks } from "@/components/insights/ShareLinks";
import { AppLogo, AppRow, BuildBadge, MaturityBadge } from "@/components/showcase/AppCard";
import { ShowcaseReviewActions } from "@/components/showcase/ShowcaseReviewActions";
import { StarButton } from "@/components/showcase/StarButton";
import { Badge, buttonClass, cx } from "@/components/ui";
import { listJobs, type Job } from "@/lib/api";
import { jobCategory } from "@/lib/format";
import { formatDate, isEditor, siteUrl } from "@/lib/insights";
import {
  APP_TYPES,
  BUILD_METHODS,
  EVIDENCE,
  MATURITY,
  PRICING,
  STATUS_LABELS,
  VERIFICATION_LABELS,
  VISIBILITY,
  displayUrl,
  getApp,
  relatedApps,
  showcaseHref,
  type ShowcaseApp,
} from "@/lib/showcase";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

type Params = { params: { slug: string } };

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const app = await getApp(params.slug, await currentViewer()).catch(() => null);
  if (!app) return { title: "Not found" };
  const url = `/showcase/${app.slug}`;
  const indexable = app.status === "published" && app.visibility === "public";
  return {
    title: `${app.name} · AI Showcase`,
    description: app.tagline || undefined,
    alternates: { canonical: url },
    robots: indexable ? undefined : { index: false, follow: false },
    openGraph: {
      type: "website",
      title: app.name,
      description: app.tagline || undefined,
      url,
      images: app.screenshots[0] ? [{ url: app.screenshots[0] }] : app.logo_url ? [{ url: app.logo_url }] : [],
    },
    twitter: {
      card: app.screenshots[0] ? "summary_large_image" : "summary",
      title: app.name,
      description: app.tagline || undefined,
    },
  };
}

function structuredData(app: ShowcaseApp, url: string) {
  return {
    "@context": "https://schema.org",
    "@type": "SoftwareApplication",
    name: app.name,
    description: app.tagline || undefined,
    url,
    applicationCategory: APP_TYPES[app.app_type],
    softwareVersion: app.version || undefined,
    license: app.license || undefined,
    codeRepository: app.repo_url || undefined,
    image: app.logo_url || undefined,
    screenshot: app.screenshots.length ? app.screenshots : undefined,
    keywords: app.technologies.join(", ") || undefined,
    author: { "@type": app.team_name ? "Organization" : "Person", name: app.team_name || app.creator_display_name },
    datePublished: app.published_at ?? undefined,
    dateModified: app.updated_at,
    isAccessibleForFree: app.pricing !== "commercial",
  };
}

/** Roles that mention the app's stack; AI roles when nothing matches. */
function pickJobs(jobs: Job[], technologies: string[]): Job[] {
  const words = technologies.map((t) => t.toLowerCase()).filter((t) => t.length > 1);
  const matches = (j: Job) => {
    const text = [j.title, j.summary, ...j.requirements].join(" ").toLowerCase();
    return words.some((w) => new RegExp(`(^|[^a-z0-9])${w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}([^a-z0-9]|$)`).test(text));
  };
  const stack = jobs.filter(matches);
  const ai = jobs.filter((j) => !stack.includes(j) && jobCategory(j) === "Data & AI");
  return [...stack, ...ai].slice(0, 2);
}

function LinkButton({
  href,
  icon,
  label,
  primary,
}: {
  href: string;
  icon: React.ReactNode;
  label: string;
  primary?: boolean;
}) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer nofollow ugc"
      className={buttonClass(primary ? "primary" : "secondary", "md")}
    >
      {icon}
      {label}
      <ArrowUpRightIcon className="h-3.5 w-3.5 opacity-60" />
    </a>
  );
}

function BuiltBy({ app }: { app: ShowcaseApp }) {
  const human = app.build_method !== "agent_autonomous" || app.human_oversight;
  const steps: Array<{ name: string; role: string; agent: boolean }> = [
    ...(human
      ? [{ name: app.team_name || app.creator_display_name || "Creator", role: app.build_method === "human" ? "Built it" : "Steered and approved", agent: false }]
      : []),
    ...app.agents.map((a) => ({ name: a.name, role: a.role, agent: true })),
  ];
  return (
    <section aria-labelledby="built-heading" className="surface-card p-5 sm:p-6">
      <h2 id="built-heading" className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
        Built by
      </h2>
      <p className="mt-2 font-display text-lg font-semibold text-ink">{BUILD_METHODS[app.build_method].label}</p>
      <p className="text-sm text-mute">{BUILD_METHODS[app.build_method].blurb}</p>
      <ol className="mt-4">
        {steps.map((s, i) => (
          <li key={`${s.name}-${i}`} className="relative flex gap-3 pb-4 last:pb-0">
            {i < steps.length - 1 ? (
              <span aria-hidden className="absolute left-[15px] top-8 h-[calc(100%-2rem)] w-px bg-line" />
            ) : null}
            <span
              className={cx(
                "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border",
                s.agent ? "border-signal/30 bg-signal/10 text-signal" : "border-shout/30 bg-shout/10 text-ink",
              )}
            >
              {s.agent ? <BotIcon className="h-4 w-4" /> : <UserIcon className="h-4 w-4" />}
            </span>
            <div className="min-w-0 pt-1">
              <p className="break-words text-sm font-semibold text-ink">{s.name}</p>
              {s.role ? <p className="text-xs text-mute">{s.role}</p> : null}
            </div>
          </li>
        ))}
      </ol>
      {app.ai_models.length ? (
        <p className="mt-4 border-t border-line pt-3 text-xs text-mute">
          <span className="font-semibold text-body">Models:</span> {app.ai_models.join(" · ")}
        </p>
      ) : null}
      {app.human_oversight ? (
        <p className="mt-3 text-xs leading-relaxed text-mute">
          <span className="font-semibold text-body">Human oversight:</span> {app.human_oversight}
        </p>
      ) : null}
    </section>
  );
}

function Evidence({ app }: { app: ShowcaseApp }) {
  const any = EVIDENCE.some(({ key }) => app.evidence[key]);
  if (!any && !app.evidence.status_page_url) return null;
  return (
    <section aria-labelledby="evidence-heading" className="surface-card p-5 sm:p-6">
      <div className="flex items-start justify-between gap-3">
        <h2 id="evidence-heading" className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
          Production evidence
        </h2>
        <Badge className="shrink-0 whitespace-nowrap">Self-declared</Badge>
      </div>
      <ul className="mt-3 space-y-1.5">
        {EVIDENCE.map(({ key, label }) => (
          <li key={key} className={cx("flex items-center gap-2.5 text-sm", app.evidence[key] ? "text-body" : "text-mute/70")}>
            {app.evidence[key] ? (
              <CheckIcon className="h-4 w-4 text-good" />
            ) : (
              <XIcon className="h-4 w-4" />
            )}
            <span>
              {label}
              {app.evidence[key] ? null : <span className="sr-only"> (not declared)</span>}
            </span>
          </li>
        ))}
      </ul>
      {app.evidence.status_page_url ? (
        <a
          href={app.evidence.status_page_url}
          target="_blank"
          rel="noopener noreferrer nofollow ugc"
          className="mt-4 inline-flex items-center gap-1.5 text-sm font-medium text-ink underline decoration-line underline-offset-4 hover:text-shout"
        >
          Status page <ArrowUpRightIcon className="h-3.5 w-3.5" />
        </a>
      ) : null}
    </section>
  );
}

export default async function ShowcaseAppPage({ params }: Params) {
  const viewer = await currentViewer();
  const app = await getApp(params.slug, viewer);
  if (!app) notFound();

  const live = app.status === "published";
  const [editor, related, jobs] = await Promise.all([
    isEditor(viewer),
    live ? relatedApps(app.slug) : Promise.resolve([]),
    listJobs(30).catch(() => [] as Job[]),
  ]);
  const owner = Boolean(viewer && app.creator_email?.toLowerCase() === viewer.email.toLowerCase());
  const canEdit = editor || (owner && app.status !== "archived");
  const url = `${siteUrl()}/showcase/${app.slug}`;
  const roles = pickJobs(jobs, app.technologies);
  const details: Array<[string, string]> = (
    [
      ["Kind", APP_TYPES[app.app_type]],
      ["Maturity", MATURITY[app.maturity].label],
      ["Pricing", PRICING[app.pricing]],
      ["Licence", app.license],
      ["Version", app.version],
      ["Updated", formatDate(app.updated_at)],
      ["Published", formatDate(app.published_at)],
    ] as Array<[string, string]>
  ).filter(([, v]) => v);

  return (
    <article className="mx-auto max-w-board px-5 pb-8 pt-8 sm:px-8 sm:pt-12">
      {live && app.visibility === "public" ? (
        <script
          type="application/ld+json"
          // JSON.stringify output with "<" escaped cannot break out of the script tag.
          dangerouslySetInnerHTML={{ __html: JSON.stringify(structuredData(app, url)).replace(/</g, "\\u003c") }}
        />
      ) : null}

      <Link
        href="/showcase"
        className="inline-flex min-h-[44px] items-center gap-2 text-sm font-medium text-mute transition-colors duration-200 hover:text-ink"
      >
        <ArrowLeftIcon className="h-4 w-4" />
        AI Showcase
      </Link>

      {!live || editor || owner ? (
        <div className="mt-4 surface-card flex flex-col gap-4 border-warn/30 bg-warn/[0.06] p-4 sm:flex-row sm:items-start sm:justify-between sm:p-5">
          <div className="min-w-0">
            <p className="text-sm font-semibold text-ink">
              {STATUS_LABELS[app.status]}
              {app.featured ? " · Featured" : ""}
              {" · "}
              {VISIBILITY[app.visibility].label}
              {!live ? (
                <span className="font-normal text-mute">
                  {" "}
                  — only you{editor ? " and other editors" : " and the editors"} can see this.
                </span>
              ) : null}
            </p>
            {app.review_note ? (
              <p className="mt-1.5 text-sm leading-relaxed text-body">
                <span className="font-semibold">Editor’s note:</span> {app.review_note}
              </p>
            ) : null}
          </div>
          <div className="flex shrink-0 flex-col gap-3 sm:items-end">
            {canEdit ? (
              <Link href={`/showcase/${app.slug}/edit`} className={buttonClass("secondary", "sm")}>
                <PenIcon className="h-4 w-4" />
                Edit
              </Link>
            ) : null}
            {editor ? <ShowcaseReviewActions id={app.id} status={app.status} featured={app.featured} /> : null}
          </div>
        </div>
      ) : null}

      <header className="mt-6 flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 gap-4 sm:gap-5">
          <AppLogo app={app} size="lg" />
          <div className="min-w-0">
            <h1 className="break-words font-display text-3xl font-semibold leading-[1.1] tracking-[-0.025em] text-ink sm:text-[2.75rem]">
              {app.name}
            </h1>
            {app.tagline ? <p className="mt-2 text-lg leading-relaxed text-mute">{app.tagline}</p> : null}
            <div className="mt-4 flex flex-wrap items-center gap-2">
              <MaturityBadge app={app} />
              <BuildBadge app={app} />
              <Badge>{APP_TYPES[app.app_type]}</Badge>
              {app.pricing === "open_source" ? <Badge>{PRICING.open_source}</Badge> : null}
              <Badge tone={app.verification === "unverified" ? "neutral" : "good"}>
                <ShieldIcon className="h-3 w-3" />
                {VERIFICATION_LABELS[app.verification]}
              </Badge>
            </div>
            <p className="mt-3 text-sm text-mute">
              by <span className="font-semibold text-ink">{app.creator_display_name}</span>
              {app.team_name ? <> · {app.team_name}</> : null}
            </p>
          </div>
        </div>
        {live ? (
          <div className="flex shrink-0 flex-wrap items-start gap-2.5">
            <StarButton slug={app.slug} starred={app.starred} count={app.star_count} signedIn={Boolean(viewer)} />
          </div>
        ) : null}
      </header>

      <div className="mt-6 flex flex-wrap gap-2.5">
        {app.demo_url ? <LinkButton href={app.demo_url} icon={<PlayIcon className="h-4 w-4" />} label="Try the demo" primary /> : null}
        {app.website_url ? (
          <LinkButton href={app.website_url} icon={<GlobeIcon className="h-4 w-4" />} label="Website" primary={!app.demo_url} />
        ) : null}
        {app.repo_url ? <LinkButton href={app.repo_url} icon={<CodeIcon className="h-4 w-4" />} label="Source" /> : null}
        {app.docs_url ? <LinkButton href={app.docs_url} icon={<BookIcon className="h-4 w-4" />} label="Docs" /> : null}
      </div>

      {app.screenshots.length ? (
        <section aria-label="Screenshots" className="-mx-5 mt-8 overflow-x-auto px-5 sm:mx-0 sm:px-0">
          <ul className="flex w-max gap-4">
            {app.screenshots.map((src, i) => (
              <li key={src} className="aspect-[16/10] w-[min(34rem,80vw)] overflow-hidden rounded-card border border-line bg-raised">
                <a href={src} target="_blank" rel="noopener noreferrer nofollow ugc">
                  {/* Screenshots come from arbitrary hosts; see AppLogo. */}
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={src} alt={`${app.name} screenshot ${i + 1}`} loading="lazy" className="h-full w-full object-cover" />
                </a>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <div className="mt-10 grid grid-cols-[minmax(0,1fr)] gap-12 lg:grid-cols-[minmax(0,1fr)_21rem] lg:gap-14">
        <div className="min-w-0 max-w-prose">
          {app.description_html ? (
            <div
              className="insight-prose"
              // Sanitised by the API (ammonia) when the app was saved.
              dangerouslySetInnerHTML={{ __html: app.description_html }}
            />
          ) : (
            <p className="text-mute">No description yet.</p>
          )}

          {app.technologies.length ? (
            <section aria-labelledby="stack-heading" className="mt-10">
              <h2 id="stack-heading" className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
                Built with
              </h2>
              <ul className="mt-3 flex flex-wrap gap-2">
                {app.technologies.map((t) => (
                  <li key={t}>
                    <Link
                      href={showcaseHref({ tech: t })}
                      className="inline-flex min-h-[36px] items-center rounded-pill border border-line bg-surface px-3.5 text-xs font-medium text-body transition-colors duration-200 hover:border-edge hover:text-ink"
                    >
                      {t}
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}

          {live ? (
            <div className="mt-10 border-t border-line pt-5">
              <ShareLinks url={url} title={`${app.name} on the JobShout AI Showcase`} />
            </div>
          ) : null}
        </div>

        <aside className="space-y-6 lg:sticky lg:top-24 lg:self-start">
          <BuiltBy app={app} />
          <Evidence app={app} />
          <section aria-labelledby="details-heading" className="surface-card p-5 sm:p-6">
            <h2 id="details-heading" className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
              Details
            </h2>
            <dl className="mt-3 divide-y divide-line text-sm">
              {details.map(([k, v]) => (
                <div key={k} className="flex justify-between gap-4 py-2">
                  <dt className="text-mute">{k}</dt>
                  <dd className="text-right font-medium text-ink">{v}</dd>
                </div>
              ))}
              {app.repo_url ? (
                <div className="flex justify-between gap-4 py-2">
                  <dt className="text-mute">Repository</dt>
                  <dd className="min-w-0 truncate text-right font-medium text-ink">{displayUrl(app.repo_url)}</dd>
                </div>
              ) : null}
            </dl>
            <p className="mt-3 text-xs leading-relaxed text-mute">
              Everything on this page is the creator&apos;s own account. {VERIFICATION_LABELS[app.verification]} means
              JobShout has not checked it yet.
            </p>
          </section>
          {related.length ? (
            <section aria-labelledby="related-heading">
              <h2 id="related-heading" className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
                More like this
              </h2>
              <div className="mt-2 divide-y divide-line">
                {related.map((r) => (
                  <AppRow key={r.id} app={r} />
                ))}
              </div>
            </section>
          ) : null}
        </aside>
      </div>

      {roles.length ? (
        <section aria-labelledby="roles-heading" className="mt-16 border-t border-line pt-10">
          <div className="flex flex-wrap items-end justify-between gap-4">
            <div>
              <h2 id="roles-heading" className="font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
                Work with this stack
              </h2>
              <p className="mt-1.5 text-sm text-mute">Open roles on the JobShout board right now.</p>
            </div>
            <Link href="/jobs" className={buttonClass("secondary", "md")}>
              See all jobs
            </Link>
          </div>
          <ul className="mt-6 grid gap-4 md:grid-cols-2">
            {roles.map((job) => (
              <li key={job.id}>
                <JobCard job={job} />
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </article>
  );
}
