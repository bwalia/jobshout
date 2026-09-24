import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { JobCard } from "@/components/JobCard";
import { ArrowLeftIcon, ArrowUpRightIcon, PenIcon } from "@/components/icons";
import { InsightCover, InsightRow, KindBadge } from "@/components/insights/InsightCard";
import { MediaPlayer, Transcript } from "@/components/insights/MediaPlayer";
import { NewsletterSignup } from "@/components/insights/NewsletterSignup";
import { ReviewActions } from "@/components/insights/ReviewActions";
import { ShareLinks } from "@/components/insights/ShareLinks";
import { Badge, buttonClass } from "@/components/ui";
import { listJobs, type Job } from "@/lib/api";
import { jobCategory } from "@/lib/format";
import {
  STATUS_LABELS,
  consumeLabel,
  formatDate,
  getInsight,
  isEditor,
  relatedInsights,
  siteUrl,
  type Insight,
} from "@/lib/insights";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

type Params = { params: { slug: string } };

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const item = await getInsight(params.slug, await currentViewer()).catch(() => null);
  if (!item) return { title: "Not found" };
  const url = `/insights/${item.slug}`;
  const images = item.cover_image_url ? [{ url: item.cover_image_url, alt: item.cover_image_alt }] : [];
  return {
    title: item.title,
    description: item.summary || undefined,
    alternates: { canonical: url },
    robots: item.status === "published" ? undefined : { index: false, follow: false },
    openGraph: {
      type: item.kind === "video" ? "video.other" : "article",
      title: item.title,
      description: item.summary || undefined,
      url,
      publishedTime: item.published_at ?? undefined,
      authors: item.author_display_name ? [item.author_display_name] : undefined,
      tags: item.topics.map((t) => t.name),
      images,
    },
    twitter: {
      card: item.cover_image_url ? "summary_large_image" : "summary",
      title: item.title,
      description: item.summary || undefined,
    },
  };
}

function iso8601Duration(seconds?: number | null): string | undefined {
  if (!seconds) return undefined;
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  return `PT${h ? `${h}H` : ""}${m ? `${m}M` : ""}${s || (!h && !m) ? `${s}S` : ""}`;
}

function structuredData(item: Insight, url: string) {
  const base = {
    "@context": "https://schema.org",
    name: item.title,
    headline: item.title,
    description: item.summary || undefined,
    url,
    datePublished: item.published_at ?? undefined,
    dateModified: item.updated_at,
    author: item.author_display_name ? { "@type": "Person", name: item.author_display_name } : undefined,
    publisher: { "@type": "Organization", name: "JobShout", url: siteUrl() },
    image: item.cover_image_url || undefined,
    keywords: item.topics.map((t) => t.name).join(", ") || undefined,
  };
  if (item.kind === "podcast") {
    return {
      ...base,
      "@type": "PodcastEpisode",
      timeRequired: iso8601Duration(item.duration_seconds),
      associatedMedia: item.embed_url
        ? { "@type": "MediaObject", contentUrl: item.embed_provider === "file" ? item.embed_url : undefined, embedUrl: item.embed_provider !== "file" ? item.embed_url : undefined }
        : undefined,
      transcript: item.transcript || undefined,
    };
  }
  if (item.kind === "video") {
    return {
      ...base,
      "@type": "VideoObject",
      uploadDate: item.published_at ?? item.created_at,
      duration: iso8601Duration(item.duration_seconds),
      thumbnailUrl: item.cover_image_url || `${siteUrl()}/insights/podcast-cover.png`,
      contentUrl: item.embed_provider === "file" ? item.embed_url : undefined,
      embedUrl: item.embed_provider !== "file" ? item.embed_url : undefined,
      transcript: item.transcript || undefined,
    };
  }
  return {
    ...base,
    "@type": item.kind === "blog" ? "BlogPosting" : item.kind === "post" ? "SocialMediaPosting" : "Article",
    wordCount: item.body_md.split(/\s+/).filter(Boolean).length,
  };
}

/** Roles to cross-sell: AI and data roles first, then whatever is newest. */
function pickJobs(jobs: Job[]): Job[] {
  const ai = jobs.filter((j) => jobCategory(j) === "Data & AI");
  return [...ai, ...jobs.filter((j) => !ai.includes(j))].slice(0, 2);
}

export default async function InsightPage({ params }: Params) {
  const viewer = await currentViewer();
  const item = await getInsight(params.slug, viewer);
  if (!item) notFound();

  const [editor, related, jobs] = await Promise.all([
    isEditor(viewer),
    item.status === "published" ? relatedInsights(item.slug) : Promise.resolve([]),
    listJobs(30).catch(() => [] as Job[]),
  ]);
  const owner = Boolean(viewer && item.author_email?.toLowerCase() === viewer.email.toLowerCase());
  const canEdit = editor || (owner && item.status !== "published" && item.status !== "archived");
  const url = `${siteUrl()}/insights/${item.slug}`;
  const media = item.kind === "podcast" || item.kind === "video";
  const roles = pickJobs(jobs);

  return (
    <article className="mx-auto max-w-board px-5 pb-8 pt-8 sm:px-8 sm:pt-12">
      {item.status === "published" ? (
        <script
          type="application/ld+json"
          // JSON.stringify output with "<" escaped cannot break out of the script tag.
          dangerouslySetInnerHTML={{
            __html: JSON.stringify(structuredData(item, url)).replace(/</g, "\\u003c"),
          }}
        />
      ) : null}

      <Link
        href="/insights"
        className="inline-flex min-h-[44px] items-center gap-2 text-sm font-medium text-mute transition-colors duration-200 hover:text-ink"
      >
        <ArrowLeftIcon className="h-4 w-4" />
        Insights
      </Link>

      {item.status !== "published" || editor ? (
        <div className="mt-4 surface-card flex flex-col gap-4 border-warn/30 bg-warn/[0.06] p-4 sm:flex-row sm:items-start sm:justify-between sm:p-5">
          <div className="min-w-0">
            <p className="text-sm font-semibold text-ink">
              {STATUS_LABELS[item.status]}
              {item.featured ? " · Featured" : ""}
              {item.status !== "published" ? (
                <span className="font-normal text-mute"> — only you{editor ? " and other editors" : " and the editors"} can see this.</span>
              ) : null}
            </p>
            {item.review_note ? (
              <p className="mt-1.5 text-sm leading-relaxed text-body">
                <span className="font-semibold">Editor’s note:</span> {item.review_note}
              </p>
            ) : null}
          </div>
          <div className="flex shrink-0 flex-col gap-3 sm:items-end">
            {canEdit ? (
              <Link href={`/insights/${item.slug}/edit`} className={buttonClass("secondary", "sm")}>
                <PenIcon className="h-4 w-4" />
                Edit
              </Link>
            ) : null}
            {editor ? <ReviewActions id={item.id} status={item.status} featured={item.featured} /> : null}
          </div>
        </div>
      ) : null}

      <div className="mt-6 grid gap-12 lg:grid-cols-[minmax(0,1fr)_19rem] lg:gap-14">
        <div className="min-w-0">
          <header className="max-w-prose">
            <div className="flex flex-wrap items-center gap-2">
              <KindBadge item={item} />
              {item.topics.map((t) => (
                <Link key={t.slug} href={`/insights?topic=${t.slug}`} className="rounded-pill focus-visible:outline-offset-2">
                  <Badge>{t.name}</Badge>
                </Link>
              ))}
            </div>
            <h1 className="mt-5 break-words font-display text-3xl font-semibold leading-[1.1] tracking-[-0.025em] text-ink sm:text-[2.75rem]">
              {item.title}
            </h1>
            {item.summary ? (
              <p className="mt-4 text-lg leading-relaxed text-mute">{item.summary}</p>
            ) : null}
            <div className="mt-6 flex flex-wrap items-center justify-between gap-4 border-y border-line py-4">
              <p className="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-sm text-mute">
                {item.author_display_name ? (
                  <span className="font-semibold text-ink">{item.author_display_name}</span>
                ) : null}
                {item.published_at ? (
                  <>
                    <span aria-hidden className="h-1 w-1 rounded-full bg-edge" />
                    <time dateTime={item.published_at}>{formatDate(item.published_at)}</time>
                  </>
                ) : null}
                <span aria-hidden className="h-1 w-1 rounded-full bg-edge" />
                <span>{consumeLabel(item)}</span>
              </p>
              {item.status === "published" ? <ShareLinks url={url} title={item.title} /> : null}
            </div>
          </header>

          <div className="mt-8">
            {media ? (
              <MediaPlayer item={item} />
            ) : item.cover_image_url ? (
              <figure className="aspect-[16/9] overflow-hidden rounded-card border border-line">
                <InsightCover item={item} />
              </figure>
            ) : null}
          </div>

          <div className="max-w-prose">
            {item.body_html ? (
              <div
                className="insight-prose mt-8"
                // Sanitised by the API (ammonia) when the item was saved.
                dangerouslySetInnerHTML={{ __html: item.body_html }}
              />
            ) : null}

            {item.link_url ? (
              <a
                href={item.link_url}
                target="_blank"
                rel="noopener noreferrer nofollow ugc"
                className="mt-8 flex items-center justify-between gap-3 rounded-card border border-line bg-surface p-4 text-sm transition-colors duration-200 hover:border-edge"
              >
                <span className="min-w-0">
                  <span className="block font-semibold text-ink">{item.kind === "post" ? "Read the story" : "Primary source"}</span>
                  <span className="block truncate text-mute">{item.link_url.replace(/^https?:\/\//, "")}</span>
                </span>
                <ArrowUpRightIcon className="h-5 w-5 shrink-0 text-mute" />
              </a>
            ) : null}

            <Transcript text={item.transcript} />
          </div>
        </div>

        <aside className="space-y-8 lg:sticky lg:top-24 lg:self-start">
          {related.length ? (
            <section aria-labelledby="related-heading">
              <h2 id="related-heading" className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
                Related insights
              </h2>
              <div className="mt-2 divide-y divide-line">
                {related.map((r) => (
                  <InsightRow key={r.id} item={r} />
                ))}
              </div>
            </section>
          ) : null}
          <NewsletterSignup compact />
        </aside>
      </div>

      {roles.length ? (
        <section aria-labelledby="roles-heading" className="mt-16 border-t border-line pt-10">
          <div className="flex flex-wrap items-end justify-between gap-4">
            <div>
              <h2 id="roles-heading" className="font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
                Put it to work
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
