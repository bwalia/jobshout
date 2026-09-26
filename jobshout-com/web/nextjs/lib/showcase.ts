/**
 * AI Showcase client. Server-side only for anything that carries identity;
 * shares the signed identity headers with Insights.
 */

import type { Job } from "@/lib/api";
import { API_BASE, failure, headers, type Viewer } from "@/lib/insights";

export type Kind = "app" | "agent" | "team";

export const KINDS: Record<Kind, { label: string; plural: string }> = {
  app: { label: "App", plural: "Apps" },
  agent: { label: "Agent", plural: "Agents" },
  team: { label: "Agent team", plural: "Agent teams" },
};

/** Mirrors AGENT_CAPABILITIES in jobshout-domain; the API rejects anything else. */
export const CAPABILITIES = {
  code_generation: "Code generation",
  code_review: "Code review",
  testing: "Testing",
  debugging: "Debugging",
  security: "Security",
  deployment: "Deployment",
  documentation: "Documentation",
  research: "Research",
  data_analysis: "Data analysis",
  design: "Design",
  planning: "Planning",
  operations: "Operations",
} as const;
export type Capability = keyof typeof CAPABILITIES;

export type AppType =
  | "web_application"
  | "mobile_application"
  | "desktop_application"
  | "api"
  | "saas"
  | "ai_application"
  | "agent_application"
  | "mcp_server"
  | "developer_tool"
  | "infrastructure"
  | "security_tool"
  | "data_platform"
  | "rag_application"
  | "automation"
  | "workflow"
  | "game"
  | "marketplace"
  | "other";
export type Maturity =
  | "idea"
  | "prototype"
  | "alpha"
  | "beta"
  | "release_candidate"
  | "production_ready"
  | "enterprise_ready";
export type BuildMethod = "human" | "human_ai" | "ai_assisted" | "agent_built" | "agent_autonomous";
export type Pricing = "open_source" | "free" | "freemium" | "commercial";
export type Visibility = "public" | "unlisted" | "private";
export type AppStatus = "draft" | "pending_review" | "published" | "rejected" | "archived";
export type Verification =
  | "unverified"
  | "creator_verified"
  | "repository_verified"
  | "build_verified"
  | "security_scanned"
  | "production_verified";
export type Collection = "featured" | "production_ready" | "built_by_agents" | "open_source" | "hiring";
export type SortKey = "new" | "stars" | "updated";

export const APP_TYPES: Record<AppType, string> = {
  web_application: "Web app",
  mobile_application: "Mobile app",
  desktop_application: "Desktop app",
  api: "API",
  saas: "SaaS",
  ai_application: "AI app",
  agent_application: "Agent app",
  mcp_server: "MCP server",
  developer_tool: "Developer tool",
  infrastructure: "Infrastructure",
  security_tool: "Security tool",
  data_platform: "Data platform",
  rag_application: "RAG app",
  automation: "Automation",
  workflow: "Workflow",
  game: "Game",
  marketplace: "Marketplace",
  other: "Other",
};

export const MATURITY: Record<Maturity, { label: string; blurb: string }> = {
  idea: { label: "Idea", blurb: "A concept, maybe a sketch." },
  prototype: { label: "Prototype", blurb: "Works on the happy path." },
  alpha: { label: "Alpha", blurb: "Early users, rough edges expected." },
  beta: { label: "Beta", blurb: "Feature-complete, still hardening." },
  release_candidate: { label: "Release candidate", blurb: "Ready unless something turns up." },
  production_ready: {
    label: "Production ready",
    blurb: "Tested, monitored and documented, with a live deployment.",
  },
  enterprise_ready: {
    label: "Enterprise ready",
    blurb: "Production ready, plus dependency scanning, backups and a release history.",
  },
};

export const BUILD_METHODS: Record<BuildMethod, { label: string; blurb: string }> = {
  human: { label: "Built by humans", blurb: "No AI in the build." },
  ai_assisted: { label: "AI assisted", blurb: "People wrote it, with AI help on parts." },
  human_ai: { label: "Human + AI", blurb: "People and AI built it together." },
  agent_built: { label: "Agent built", blurb: "Agents did most of the building; people steered." },
  agent_autonomous: {
    label: "Agent autonomous",
    blurb: "Agents built it end to end; people approved.",
  },
};

export const PRICING: Record<Pricing, string> = {
  open_source: "Open source",
  free: "Free",
  freemium: "Freemium",
  commercial: "Commercial",
};

export const VISIBILITY: Record<Visibility, { label: string; blurb: string }> = {
  public: { label: "Public", blurb: "Listed in the showcase and search once approved." },
  unlisted: { label: "Unlisted", blurb: "Anyone with the link, kept out of lists and search." },
  private: { label: "Private", blurb: "Only you and the editors." },
};

export const STATUS_LABELS: Record<AppStatus, string> = {
  draft: "Draft",
  pending_review: "In review",
  published: "Published",
  rejected: "Changes requested",
  archived: "Archived",
};

export const VERIFICATION_LABELS: Record<Verification, string> = {
  unverified: "Unverified",
  creator_verified: "Creator verified",
  repository_verified: "Repository verified",
  build_verified: "Build verified",
  security_scanned: "Security scanned",
  production_verified: "Production verified",
};

export const COLLECTIONS: Record<Collection, { label: string; blurb: string }> = {
  featured: { label: "Featured", blurb: "Picked by the JobShout editors." },
  production_ready: {
    label: "Production ready",
    blurb: "Creators who back the claim with tests, CI, monitoring and a live deployment.",
  },
  built_by_agents: {
    label: "Built by agents",
    blurb: "Apps where AI agents did most of the building.",
  },
  open_source: { label: "Open source", blurb: "Read the code, run it yourself." },
  hiring: { label: "Hiring", blurb: "Projects with open roles on the JobShout board." },
};

export const EVIDENCE: Array<{ key: EvidenceFlag; label: string }> = [
  { key: "automated_tests", label: "Automated tests" },
  { key: "ci_cd", label: "CI/CD" },
  { key: "security_scanning", label: "Security scanning" },
  { key: "dependency_scanning", label: "Dependency scanning" },
  { key: "monitoring", label: "Monitoring" },
  { key: "backups", label: "Backups and recovery" },
  { key: "documentation", label: "Documentation" },
  { key: "release_history", label: "Release history" },
];

export type EvidenceFlag =
  | "automated_tests"
  | "ci_cd"
  | "security_scanning"
  | "dependency_scanning"
  | "monitoring"
  | "backups"
  | "documentation"
  | "release_history";

export type Evidence = Record<EvidenceFlag, boolean> & { status_page_url: string };

export interface ShowcaseAgent {
  name: string;
  role: string;
}

/** A directory agent or team another entry links to. */
export interface ShowcaseLink {
  slug: string;
  kind: Kind;
  name: string;
  tagline: string;
  logo_url: string;
  role: string;
}

export interface LinkInput {
  slug: string;
  role: string;
}

export interface ShowcaseApp {
  id: string;
  kind: Kind;
  slug: string;
  name: string;
  tagline: string;
  description_md: string;
  description_html: string;
  app_type: AppType;
  maturity: Maturity;
  build_method: BuildMethod;
  pricing: Pricing;
  license: string;
  version: string;
  logo_url: string;
  screenshots: string[];
  repo_url: string;
  demo_url: string;
  website_url: string;
  docs_url: string;
  technologies: string[];
  ai_models: string[];
  agents: ShowcaseAgent[];
  human_oversight: string;
  evidence: Evidence;
  team_name: string;
  model_provider: string;
  tools: string[];
  mcp_servers: string[];
  capabilities: string[];
  linked_agents: ShowcaseLink[];
  linked_team: ShowcaseLink | null;
  used_in: number;
  /** Open roles on the board this entry is hiring for (published jobs only). */
  jobs: Job[];
  creator_email?: string;
  creator_display_name: string;
  visibility: Visibility;
  status: AppStatus;
  verification: Verification;
  featured: boolean;
  review_note: string;
  star_count: number;
  starred: boolean;
  published_at?: string | null;
  created_at: string;
  updated_at: string;
}

export type ShowcaseAppInput = Omit<
  ShowcaseApp,
  | "id"
  | "slug"
  | "description_html"
  | "creator_email"
  | "creator_display_name"
  | "status"
  | "verification"
  | "featured"
  | "review_note"
  | "star_count"
  | "starred"
  | "published_at"
  | "created_at"
  | "updated_at"
  | "linked_agents"
  | "linked_team"
  | "used_in"
  | "jobs"
> & { submit: boolean; agent_links: LinkInput[]; team_slug: string; job_ids: string[] };

export interface ShowcaseTag {
  name: string;
  count: number;
}

export type ShowcaseQuery = {
  kind?: Kind | null;
  capability?: Capability | null;
  q?: string | null;
  type?: AppType | null;
  collection?: Collection | null;
  tech?: string | null;
  sort?: SortKey | null;
  limit?: number;
  offset?: number;
};

const is =
  <T extends string>(values: Record<T, unknown>) =>
  (v: unknown): v is T =>
    typeof v === "string" && Object.prototype.hasOwnProperty.call(values, v);

export const isAppType = is(APP_TYPES);
export const isMaturity = is(MATURITY);
export const isBuildMethod = is(BUILD_METHODS);
export const isPricing = is(PRICING);
export const isVisibility = is(VISIBILITY);
export const isCollection = is(COLLECTIONS);
export const isKind = is(KINDS);
export const isCapability = is(CAPABILITIES);
export const isSortKey = (v: unknown): v is SortKey => v === "new" || v === "stars" || v === "updated";

export async function listApps(
  query: ShowcaseQuery = {},
  viewer?: Viewer | null,
): Promise<{ data: ShowcaseApp[]; total: number }> {
  const params = new URLSearchParams();
  if (query.kind) params.set("kind", query.kind);
  if (query.capability) params.set("capability", query.capability);
  if (query.q) params.set("q", query.q);
  if (query.type) params.set("type", query.type);
  if (query.collection) params.set("collection", query.collection);
  if (query.tech) params.set("tech", query.tech);
  if (query.sort) params.set("sort", query.sort);
  params.set("limit", String(query.limit ?? 12));
  if (query.offset) params.set("offset", String(query.offset));
  const res = await fetch(`${API_BASE}/api/v1/showcase/apps?${params}`, {
    headers: headers(viewer),
    // Per-viewer star state makes signed-in lists uncacheable.
    ...(viewer ? { cache: "no-store" as const } : { next: { revalidate: 30, tags: ["showcase"] } }),
  });
  if (!res.ok) throw await failure(res, "Failed to load the showcase");
  return (await res.json()) as { data: ShowcaseApp[]; total: number };
}

export async function listTags(limit = 24, kind: Kind = "app"): Promise<ShowcaseTag[]> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/tags?limit=${limit}&kind=${kind}`, {
    next: { revalidate: 300, tags: ["showcase"] },
  });
  if (!res.ok) throw await failure(res, "Failed to load technologies");
  return ((await res.json()) as { data: ShowcaseTag[] }).data;
}

export async function getApp(slug: string, viewer?: Viewer | null): Promise<ShowcaseApp | null> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/apps/${encodeURIComponent(slug)}`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load the app");
  return (await res.json()) as ShowcaseApp;
}

export async function relatedApps(slug: string): Promise<ShowcaseApp[]> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/apps/${encodeURIComponent(slug)}/related`, {
    next: { revalidate: 60, tags: ["showcase"] },
  });
  if (!res.ok) return [];
  return ((await res.json()) as { data: ShowcaseApp[] }).data;
}

/** Public apps (and, for an agent, teams) that link to an agent or team. */
export async function usedIn(
  slug: string,
  viewer?: Viewer | null,
): Promise<{ apps: ShowcaseApp[]; teams: ShowcaseApp[] }> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/apps/${encodeURIComponent(slug)}/used-in`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) return { apps: [], teams: [] };
  return (await res.json()) as { apps: ShowcaseApp[]; teams: ShowcaseApp[] };
}

/** Agents or teams the viewer may link: public ones plus their own. */
export async function linkCandidates(
  viewer: Viewer,
  kind: "agent" | "team",
  q: string,
): Promise<ShowcaseApp[]> {
  const params = new URLSearchParams({ kind });
  if (q.trim()) params.set("q", q.trim());
  const res = await fetch(`${API_BASE}/api/v1/showcase/link-candidates?${params}`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Could not search the directory");
  return ((await res.json()) as { data: ShowcaseApp[] }).data;
}

/** Open jobs the viewer may link: the ones they posted (editors: any). */
export async function linkableJobs(viewer: Viewer): Promise<Job[]> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/linkable-jobs`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) return [];
  return ((await res.json()) as { data: Job[] }).data;
}

/** Public showcase entries (any kind) hiring for this job. */
export async function entriesForJob(jobId: string): Promise<ShowcaseApp[]> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/by-job/${encodeURIComponent(jobId)}`, {
    next: { revalidate: 60, tags: ["showcase"] },
  });
  if (!res.ok) return [];
  return ((await res.json()) as { data: ShowcaseApp[] }).data;
}

export async function myApps(viewer: Viewer): Promise<ShowcaseApp[]> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/apps/mine`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load your apps");
  return ((await res.json()) as { data: ShowcaseApp[] }).data;
}

/** Returns null when the viewer is not an editor. */
export async function showcaseReviewQueue(viewer: Viewer): Promise<ShowcaseApp[] | null> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/review-queue`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (res.status === 403) return null;
  if (!res.ok) throw await failure(res, "Failed to load the review queue");
  return ((await res.json()) as { data: ShowcaseApp[] }).data;
}

export async function saveApp(
  viewer: Viewer,
  input: ShowcaseAppInput,
  id?: string,
): Promise<ShowcaseApp> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/apps${id ? `/${id}` : ""}`, {
    method: id ? "PATCH" : "POST",
    headers: headers(viewer),
    body: JSON.stringify(input),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to save");
  return (await res.json()) as ShowcaseApp;
}

export async function deleteApp(viewer: Viewer, id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/apps/${id}`, {
    method: "DELETE",
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to delete");
}

export type ShowcaseModeration = "approve" | "reject" | "feature" | "unfeature" | "archive";

export async function moderateApp(
  viewer: Viewer,
  id: string,
  action: ShowcaseModeration,
  note = "",
): Promise<ShowcaseApp> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/apps/${id}/moderate`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify({ action, note }),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to update");
  return (await res.json()) as ShowcaseApp;
}

export async function setStar(
  viewer: Viewer,
  slug: string,
  on: boolean,
): Promise<{ starred: boolean; star_count: number }> {
  const res = await fetch(`${API_BASE}/api/v1/showcase/apps/${encodeURIComponent(slug)}/star`, {
    method: on ? "PUT" : "DELETE",
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to update the star");
  return (await res.json()) as { starred: boolean; star_count: number };
}

/** Where an entry lives: apps in the showcase, agents and teams in the directory. */
export function entryHref(e: { kind: Kind; slug: string }): string {
  return e.kind === "app" ? `/showcase/${e.slug}` : `/agents/${e.slug}`;
}

export function directoryHref(f: { kind?: Kind | null; capability?: Capability | null; q?: string | null; tech?: string | null; sort?: SortKey | null; page?: number }): string {
  const p = new URLSearchParams();
  if (f.kind === "team") p.set("kind", "team");
  if (f.capability) p.set("capability", f.capability);
  if (f.tech) p.set("tech", f.tech);
  if (f.q) p.set("q", f.q);
  if (f.sort && f.sort !== "new") p.set("sort", f.sort);
  if (f.page && f.page > 1) p.set("page", String(f.page));
  const s = p.toString();
  return s ? `/agents?${s}` : "/agents";
}

/** "github.com/acme/app" — a link's host and path, for display. */
export function displayUrl(url: string): string {
  return url.replace(/^https?:\/\/(www\.)?/, "").replace(/\/$/, "");
}

export function showcaseHref(f: ShowcaseQuery & { page?: number }): string {
  const p = new URLSearchParams();
  if (f.collection) p.set("collection", f.collection);
  if (f.type) p.set("type", f.type);
  if (f.tech) p.set("tech", f.tech);
  if (f.q) p.set("q", f.q);
  if (f.sort && f.sort !== "new") p.set("sort", f.sort);
  if (f.page && f.page > 1) p.set("page", String(f.page));
  const s = p.toString();
  return s ? `/showcase?${s}` : "/showcase";
}
