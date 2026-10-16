/**
 * OpsAPI CMS posts for the Insights hub. Server-side only.
 *
 * Reads the public, published-only endpoints of one OpsAPI workspace
 * (/api/v2/public/cms/{namespace}/...), so no key is involved. Disabled — every
 * call returns empty — unless OPSAPI_URL and OPSAPI_NAMESPACE are both set.
 * Posts are cached under the "opsapi-posts" tag; /hooks/opsapi revalidates it
 * when someone presses publish / "Website update" in OpsAPI.
 */

import sanitizeHtml from "sanitize-html";
import type { Insight, InsightTopic } from "@/lib/insights";

export const OPSAPI_TAG = "opsapi-posts";
/** Insight ids for OpsAPI posts carry this prefix, so pages can tell them apart. */
export const OPSAPI_ID_PREFIX = "opsapi:";

const REVALIDATE_SECONDS = 300;

type OpsapiTerm = { uuid: string; slug: string; name: string };

type OpsapiPost = {
  uuid: string;
  slug: string;
  title: string;
  excerpt?: string;
  content_html?: string;
  featured_image_url?: string;
  author_name?: string;
  published_at?: string;
  reading_minutes?: number;
  is_featured?: boolean;
  categories?: OpsapiTerm[];
  tags?: OpsapiTerm[];
  seo_title?: string;
  seo_description?: string;
};

function config(): { base: string; namespace: string } | null {
  // Read at runtime, like siteUrl(): NEXT_PUBLIC_* would be frozen into the build.
  const base = process.env.OPSAPI_URL?.trim().replace(/\/$/, "");
  const namespace = process.env.OPSAPI_NAMESPACE?.trim();
  return base && namespace ? { base, namespace } : null;
}

export function opsapiEnabled(): boolean {
  return config() !== null;
}

/** OpsAPI sends Postgres "YYYY-MM-DD HH:MM:SS.ffffff" in UTC, without a zone. */
function isoDate(value?: string): string | null {
  if (!value) return null;
  const iso = /[zZ]|[+-]\d\d:?\d\d$/.test(value) ? value : `${value.replace(" ", "T")}Z`;
  return Number.isNaN(Date.parse(iso)) ? null : iso;
}

/** Workspace members write this HTML; it renders on our origin, so strip anything active. */
function cleanHtml(html: string): string {
  return sanitizeHtml(html, {
    allowedTags: sanitizeHtml.defaults.allowedTags.concat(["img", "figure", "figcaption"]),
    allowedAttributes: {
      ...sanitizeHtml.defaults.allowedAttributes,
      img: ["src", "alt", "title", "width", "height", "loading"],
      code: ["class"],
    },
    allowedSchemes: ["http", "https", "mailto"],
    transformTags: {
      a: sanitizeHtml.simpleTransform("a", { rel: "noopener noreferrer nofollow" }),
    },
  });
}

function toInsight(post: OpsapiPost): Insight {
  const published = isoDate(post.published_at);
  const topics: InsightTopic[] = [...(post.categories ?? []), ...(post.tags ?? [])].map((t) => ({
    slug: t.slug,
    name: t.name,
    description: "",
  }));
  return {
    id: `${OPSAPI_ID_PREFIX}${post.uuid}`,
    kind: "blog",
    slug: post.slug,
    title: post.title,
    summary: post.seo_description || post.excerpt || "",
    body_md: "",
    body_html: post.content_html ? cleanHtml(post.content_html) : "",
    cover_image_url: post.featured_image_url ?? "",
    cover_image_alt: "",
    link_url: "",
    media_url: "",
    embed_provider: "",
    embed_url: "",
    transcript: "",
    author_display_name: post.author_name ?? "",
    author_handle: null,
    status: "published",
    source: "staff",
    featured: Boolean(post.is_featured),
    review_note: "",
    reading_minutes: post.reading_minutes ?? 1,
    topics,
    published_at: published,
    created_at: published ?? "",
    updated_at: published ?? "",
  };
}

/** Newest published OpsAPI posts. Never throws: OpsAPI being down must not break Insights. */
export async function listOpsapiPosts(limit = 6): Promise<Insight[]> {
  const cfg = config();
  if (!cfg) return [];
  try {
    const res = await fetch(
      `${cfg.base}/api/v2/public/cms/${encodeURIComponent(cfg.namespace)}/posts?page=1&perPage=${limit}`,
      { next: { revalidate: REVALIDATE_SECONDS, tags: [OPSAPI_TAG] } },
    );
    if (!res.ok) return [];
    const body = (await res.json()) as { data?: OpsapiPost[] };
    return (body.data ?? []).map(toInsight);
  } catch {
    return [];
  }
}

/** One published OpsAPI post by slug, or null (not found, disabled, or OpsAPI unreachable). */
export async function getOpsapiPost(slug: string): Promise<Insight | null> {
  const cfg = config();
  if (!cfg) return null;
  try {
    const res = await fetch(
      `${cfg.base}/api/v2/public/cms/${encodeURIComponent(cfg.namespace)}/posts/${encodeURIComponent(slug)}`,
      { next: { revalidate: REVALIDATE_SECONDS, tags: [OPSAPI_TAG] } },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { data?: OpsapiPost };
    return body.data ? toInsight(body.data) : null;
  } catch {
    return null;
  }
}
