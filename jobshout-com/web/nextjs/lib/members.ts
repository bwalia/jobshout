/**
 * Members and follows client. Server-side only for anything that carries
 * identity; shares the signed identity headers with Insights.
 */

import { API_BASE, failure, headers, type Insight, type InsightKind, type Viewer } from "@/lib/insights";
import type { Kind, ShowcaseApp } from "@/lib/showcase";

/** Mirrors HEADLINE_MAX in jobshout-domain. */
export const HEADLINE_MAX = 160;

export interface Member {
  id: string;
  handle: string;
  display_name: string;
  headline: string;
  /** A platform agent (e.g. the Article Writer), not a person. */
  is_agent: boolean;
  follower_count: number;
  following_count: number;
  /** Whether the viewer follows this member. */
  following: boolean;
  entry_count: number;
  insight_count: number;
  created_at: string;
}

export interface MemberProfile {
  member: Member;
  is_self: boolean;
  entries: ShowcaseApp[];
  insights: Insight[];
}

export interface FollowState {
  following: boolean;
  follower_count: number;
}

export type FeedItemKind = "new_entry" | "new_insight" | "entry_updated" | "used_in";

export interface FeedEntry {
  slug: string;
  kind: Kind;
  name: string;
  tagline: string;
  logo_url: string;
  version: string;
}

export interface FeedItem {
  kind: FeedItemKind;
  at: string;
  actor?: { handle: string; display_name: string; is_agent: boolean };
  entry?: FeedEntry;
  /** For used_in: the followed agent or team it uses. */
  uses?: FeedEntry;
  insight?: { slug: string; kind: InsightKind; title: string; summary: string };
}

export interface Following {
  members: Member[];
  entries: Array<FeedEntry & { follower_count: number }>;
}

export function memberHref(handle: string): string {
  return `/members/${encodeURIComponent(handle)}`;
}

export async function listMembers(
  q: string | null,
  page: number,
  pageSize: number,
  viewer?: Viewer | null,
): Promise<{ data: Member[]; total: number }> {
  const params = new URLSearchParams({ limit: String(pageSize), offset: String((page - 1) * pageSize) });
  if (q) params.set("q", q);
  const res = await fetch(`${API_BASE}/api/v1/members?${params}`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load members");
  return (await res.json()) as { data: Member[]; total: number };
}

export async function getMember(handle: string, viewer?: Viewer | null): Promise<MemberProfile | null> {
  const res = await fetch(`${API_BASE}/api/v1/members/${encodeURIComponent(handle)}`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load the member");
  return (await res.json()) as MemberProfile;
}

/** The viewer's own member, created on first call. */
export async function getMe(viewer: Viewer): Promise<Member> {
  const res = await fetch(`${API_BASE}/api/v1/members/me`, { headers: headers(viewer), cache: "no-store" });
  if (!res.ok) throw await failure(res, "Failed to load your profile");
  return (await res.json()) as Member;
}

export async function saveHeadline(viewer: Viewer, headline: string): Promise<Member> {
  const res = await fetch(`${API_BASE}/api/v1/members/me`, {
    method: "PATCH",
    headers: headers(viewer),
    body: JSON.stringify({ headline }),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to save your headline");
  return (await res.json()) as Member;
}

export type FollowTarget = { type: "member"; handle: string } | { type: "entry"; slug: string };

export async function setFollow(viewer: Viewer, target: FollowTarget, on: boolean): Promise<FollowState> {
  const path =
    target.type === "member"
      ? `/api/v1/members/${encodeURIComponent(target.handle)}/follow`
      : `/api/v1/showcase/apps/${encodeURIComponent(target.slug)}/follow`;
  const res = await fetch(`${API_BASE}${path}`, {
    method: on ? "PUT" : "DELETE",
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to update the follow");
  return (await res.json()) as FollowState;
}

export async function getFollowing(viewer: Viewer): Promise<Following> {
  const res = await fetch(`${API_BASE}/api/v1/following`, { headers: headers(viewer), cache: "no-store" });
  if (!res.ok) throw await failure(res, "Failed to load who you follow");
  return (await res.json()) as Following;
}

export async function getFeed(viewer: Viewer, before?: string | null): Promise<FeedItem[]> {
  const params = new URLSearchParams({ limit: "30" });
  if (before) params.set("before", before);
  const res = await fetch(`${API_BASE}/api/v1/following/feed?${params}`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load your feed");
  return ((await res.json()) as { data: FeedItem[] }).data;
}
