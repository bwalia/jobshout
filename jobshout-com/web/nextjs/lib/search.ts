/**
 * Cross-entity typeahead search. Fans out to existing board APIs and returns
 * a flat, href-ready hit list. Groups are omitted when GROUPS_ENABLED is dark.
 */

import { GroupsDarkError, searchGroups } from "@/lib/groups";
import { API_BASE, listInsights } from "@/lib/insights";
import { listMembers } from "@/lib/members";
import {
  type SearchHit,
  type UnifiedSearchResponse,
} from "@/lib/search-shared";
import { entryHref, listApps } from "@/lib/showcase";

export type { SearchHit, SearchKind, UnifiedSearchResponse } from "@/lib/search-shared";
export { boardPathForKind, searchKindLabel } from "@/lib/search-shared";

const PER_KIND = 3;

async function searchJobs(q: string): Promise<SearchHit[]> {
  const qs = new URLSearchParams({ q, limit: String(PER_KIND), offset: "0" });
  const res = await fetch(`${API_BASE}/api/v1/jobs?${qs}`, {
    next: { revalidate: 15 },
  });
  if (!res.ok) return [];
  const body = (await res.json()) as {
    data: Array<{
      id: string;
      title: string;
      summary?: string;
      location?: {
        city?: string | null;
        region?: string | null;
        country?: string;
        remote?: boolean;
      };
    }>;
  };
  return (body.data ?? []).slice(0, PER_KIND).map((job) => {
    const loc = job.location;
    const place = loc?.remote
      ? "Remote"
      : [loc?.city, loc?.region, loc?.country].filter(Boolean).join(", ");
    return {
      kind: "job" as const,
      id: job.id,
      title: job.title,
      subtitle: place || job.summary?.slice(0, 80) || "Open role",
      href: `/jobs/${job.id}`,
    };
  });
}

async function searchAgents(q: string): Promise<SearchHit[]> {
  const { data } = await listApps({ kind: "agent", q, limit: PER_KIND });
  return data.slice(0, PER_KIND).map((app) => ({
    kind: "agent" as const,
    id: app.id,
    title: app.name,
    subtitle: app.tagline || "AI agent",
    href: entryHref(app),
  }));
}

async function searchTalents(q: string): Promise<SearchHit[]> {
  const { data } = await listMembers(q, 1, PER_KIND);
  return data.slice(0, PER_KIND).map((m) => ({
    kind: "talent" as const,
    id: m.id,
    title: m.display_name,
    subtitle: m.headline || `@${m.handle}`,
    href: `/members/${encodeURIComponent(m.handle)}`,
  }));
}

async function searchGroupHits(q: string): Promise<SearchHit[]> {
  try {
    const rows = await searchGroups(q, PER_KIND);
    return rows.slice(0, PER_KIND).map((g) => ({
      kind: "group" as const,
      id: g.id,
      title: g.name,
      subtitle: g.description?.slice(0, 80) || "Group",
      href: `/groups/${encodeURIComponent(g.slug)}`,
    }));
  } catch (e) {
    if (e instanceof GroupsDarkError) return [];
    return [];
  }
}

async function searchInsightHits(q: string): Promise<SearchHit[]> {
  const { data } = await listInsights({ q, limit: PER_KIND });
  return data.slice(0, PER_KIND).map((item) => ({
    kind: "insight" as const,
    id: item.id,
    title: item.title,
    subtitle: item.summary?.slice(0, 80) || "Insight",
    href: `/insights/${encodeURIComponent(item.slug)}`,
  }));
}

/**
 * Fan-out typeahead across jobs, agents, talents, groups, and insights.
 * Soft-fails per lane so one cold API cannot blank the whole dropdown.
 */
export async function unifiedSearch(raw: string): Promise<UnifiedSearchResponse> {
  const query = raw.trim();
  if (query.length < 2) {
    return { query, results: [] };
  }

  const settled = await Promise.allSettled([
    searchJobs(query),
    searchAgents(query),
    searchTalents(query),
    searchGroupHits(query),
    searchInsightHits(query),
  ]);

  const results: SearchHit[] = [];
  for (const outcome of settled) {
    if (outcome.status === "fulfilled") results.push(...outcome.value);
  }
  return { query, results };
}
