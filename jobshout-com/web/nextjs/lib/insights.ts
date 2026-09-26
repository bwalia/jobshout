/**
 * Insights hub client. Server-side only for anything that carries identity:
 * the signed-in user is forwarded as headers the API trusts (it is not
 * reachable from the internet — ingress routes only to this app).
 */

export type InsightKind = "post" | "article" | "blog" | "podcast" | "video";
export type InsightStatus = "draft" | "pending_review" | "published" | "rejected" | "archived";
export type InsightSource = "staff" | "community" | "agent";

export const INSIGHT_KINDS: InsightKind[] = ["post", "article", "blog", "podcast", "video"];

export const KIND_META: Record<
  InsightKind,
  { label: string; plural: string; blurb: string }
> = {
  post: { label: "Post", plural: "Posts", blurb: "A short update or news item." },
  article: {
    label: "Article",
    plural: "Articles",
    blurb: "Long-form technical or business analysis.",
  },
  blog: { label: "Blog", plural: "Blogs", blurb: "Opinion and first-hand experience." },
  podcast: { label: "Podcast", plural: "Podcasts", blurb: "An episode with show notes." },
  video: { label: "Video", plural: "Videos", blurb: "A talk, demo or explainer." },
};

export const STATUS_LABELS: Record<InsightStatus, string> = {
  draft: "Draft",
  pending_review: "In review",
  published: "Published",
  rejected: "Changes requested",
  archived: "Archived",
};

export interface InsightTopic {
  slug: string;
  name: string;
  description: string;
}

export interface Insight {
  id: string;
  kind: InsightKind;
  slug: string;
  title: string;
  summary: string;
  body_md: string;
  body_html: string;
  cover_image_url: string;
  cover_image_alt: string;
  link_url: string;
  media_url: string;
  embed_provider: "" | "youtube" | "vimeo" | "spotify" | "apple" | "file";
  embed_url: string;
  duration_seconds?: number | null;
  transcript: string;
  author_email?: string;
  author_display_name: string;
  status: InsightStatus;
  source: InsightSource;
  featured: boolean;
  review_note: string;
  reading_minutes: number;
  topics: InsightTopic[];
  published_at?: string | null;
  created_at: string;
  updated_at: string;
}

export type InsightInput = {
  kind: InsightKind;
  title: string;
  summary?: string;
  body_md?: string;
  cover_image_url?: string;
  cover_image_alt?: string;
  link_url?: string;
  media_url?: string;
  duration_seconds?: number | null;
  transcript?: string;
  topics?: string[];
  submit: boolean;
};

export interface InsightDigest {
  week: string;
  starts_on: string;
  items: Insight[];
}

export type Viewer = { email: string; name?: string | null };

export type InsightQuery = {
  kind?: InsightKind | null;
  topic?: string | null;
  q?: string | null;
  featured?: boolean;
  limit?: number;
  offset?: number;
};

export const API_BASE = process.env.JOBSHOUT_COM_API_URL ?? "http://127.0.0.1:8088";

/** Identity headers for the API, signed with the web tier's shared token. */
export function headers(viewer?: Viewer | null): HeadersInit {
  const h: Record<string, string> = { "content-type": "application/json" };
  if (viewer?.email) {
    h["x-jobshout-user-email"] = viewer.email;
    if (viewer.name) h["x-jobshout-user-name"] = viewer.name;
    const token = process.env.JOBSHOUT_INTERNAL_TOKEN;
    if (token) h["x-jobshout-internal-token"] = token;
  }
  return h;
}

export async function failure(res: Response, fallback: string): Promise<Error> {
  const body = (await res.json().catch(() => null)) as {
    error?: { message?: string };
  } | null;
  return new Error(body?.error?.message ?? `${fallback} (${res.status})`);
}

export async function listInsights(
  query: InsightQuery = {},
): Promise<{ data: Insight[]; total: number }> {
  const params = new URLSearchParams();
  if (query.kind) params.set("kind", query.kind);
  if (query.topic) params.set("topic", query.topic);
  if (query.q) params.set("q", query.q);
  if (query.featured) params.set("featured", "true");
  params.set("limit", String(query.limit ?? 12));
  if (query.offset) params.set("offset", String(query.offset));
  const res = await fetch(`${API_BASE}/api/v1/insights?${params}`, {
    next: { revalidate: 30, tags: ["insights"] },
  });
  if (!res.ok) throw await failure(res, "Failed to load insights");
  return (await res.json()) as { data: Insight[]; total: number };
}

export async function listTopics(): Promise<InsightTopic[]> {
  const res = await fetch(`${API_BASE}/api/v1/insights/topics`, { next: { revalidate: 300 } });
  if (!res.ok) throw await failure(res, "Failed to load topics");
  return ((await res.json()) as { data: InsightTopic[] }).data;
}

export async function getInsight(slug: string, viewer?: Viewer | null): Promise<Insight | null> {
  const res = await fetch(`${API_BASE}/api/v1/insights/${encodeURIComponent(slug)}`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load insight");
  return (await res.json()) as Insight;
}

export async function relatedInsights(slug: string): Promise<Insight[]> {
  const res = await fetch(`${API_BASE}/api/v1/insights/${encodeURIComponent(slug)}/related`, {
    next: { revalidate: 60, tags: ["insights"] },
  });
  if (!res.ok) return [];
  return ((await res.json()) as { data: Insight[] }).data;
}

export async function myInsights(viewer: Viewer): Promise<Insight[]> {
  const res = await fetch(`${API_BASE}/api/v1/insights/mine`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load your insights");
  return ((await res.json()) as { data: Insight[] }).data;
}

/** Returns null when the viewer is not an editor. */
export async function reviewQueue(viewer: Viewer): Promise<Insight[] | null> {
  const res = await fetch(`${API_BASE}/api/v1/insights/review-queue`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (res.status === 403) return null;
  if (!res.ok) throw await failure(res, "Failed to load the review queue");
  return ((await res.json()) as { data: Insight[] }).data;
}

export async function listDigests(weeks = 12): Promise<InsightDigest[]> {
  const res = await fetch(`${API_BASE}/api/v1/insights/digests?weeks=${weeks}`, {
    next: { revalidate: 300, tags: ["insights"] },
  });
  if (!res.ok) throw await failure(res, "Failed to load the newsletter archive");
  return ((await res.json()) as { data: InsightDigest[] }).data;
}

export async function saveInsight(
  viewer: Viewer,
  input: InsightInput,
  id?: string,
): Promise<Insight> {
  const res = await fetch(`${API_BASE}/api/v1/insights${id ? `/${id}` : ""}`, {
    method: id ? "PATCH" : "POST",
    headers: headers(viewer),
    body: JSON.stringify(input),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to save");
  return (await res.json()) as Insight;
}

export async function deleteInsight(viewer: Viewer, id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/insights/${id}`, {
    method: "DELETE",
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to delete");
}

export type ModerationAction = "approve" | "reject" | "feature" | "unfeature" | "archive";

export async function moderateInsight(
  viewer: Viewer,
  id: string,
  action: ModerationAction,
  note = "",
): Promise<Insight> {
  const res = await fetch(`${API_BASE}/api/v1/insights/${id}/moderate`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify({ action, note }),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to update");
  return (await res.json()) as Insight;
}

export async function previewMarkdown(body_md: string): Promise<string> {
  const res = await fetch(`${API_BASE}/api/v1/insights/preview`, {
    method: "POST",
    headers: headers(),
    body: JSON.stringify({ body_md }),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Preview failed");
  return ((await res.json()) as { html: string }).html;
}

export async function fetchFeed(which: "rss" | "podcast"): Promise<Response> {
  return fetch(`${API_BASE}/api/v1/insights/feed/${which}`, { next: { revalidate: 300 } });
}

async function newsletterCall(path: string, body: unknown): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/newsletter/${path}`, {
    method: "POST",
    headers: headers(),
    body: JSON.stringify(body),
    cache: "no-store",
  });
  if (res.status === 404) throw new Error("That link has expired or was already used.");
  if (!res.ok) throw await failure(res, "Newsletter request failed");
}

export const subscribeNewsletter = (email: string) => newsletterCall("subscribe", { email });
export const confirmNewsletter = (token: string) => newsletterCall("confirm", { token });
export const unsubscribeNewsletter = (token: string) => newsletterCall("unsubscribe", { token });

/* --- Formatting ---------------------------------------------------------- */

export function formatDuration(seconds?: number | null): string | null {
  if (!seconds || seconds <= 0) return null;
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h) return `${h} hr ${m} min`;
  if (m) return `${m} min`;
  return `${seconds} sec`;
}

/** "6 min read", "42 min listen", "3 min watch" — whatever fits the kind. */
export function consumeLabel(item: Insight): string {
  if (item.kind === "podcast" || item.kind === "video") {
    const d = formatDuration(item.duration_seconds);
    return d ? `${d} ${item.kind === "podcast" ? "listen" : "watch"}` : KIND_META[item.kind].label;
  }
  if (item.kind === "post") return "Quick read";
  return `${item.reading_minutes} min read`;
}

export function formatDate(iso?: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
}

export function isInsightKind(v: unknown): v is InsightKind {
  return typeof v === "string" && (INSIGHT_KINDS as string[]).includes(v);
}

export function siteUrl(): string {
  // Read at runtime: NEXT_PUBLIC_* would be frozen into the build.
  return (process.env.SITE_URL || process.env.NEXTAUTH_URL || "https://jobshout.com")
    .replace(/\/$/, "");
}

export async function isEditor(viewer: Viewer | null): Promise<boolean> {
  if (!viewer) return false;
  const res = await fetch(`${API_BASE}/api/v1/insights/whoami`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) return false;
  return ((await res.json()) as { is_staff: boolean }).is_staff;
}
