import { API_BASE, failure, headers, type Viewer } from "@/lib/insights";

export type CommunityGroup = {
  id: string;
  slug: string;
  name: string;
  description: string;
  kind: string;
  visibility: string;
  join_policy: string;
  created_at: string;
  member_count?: number | null;
  is_member?: boolean | null;
  my_role?: string | null;
};

export type GroupPost = {
  id: string;
  group_id: string;
  author_member_id: string;
  author_handle: string;
  author_display_name: string;
  kind: string;
  title: string;
  body: string;
  pinned: boolean;
  locked: boolean;
  comment_count: number;
  reaction_count: number;
  liked_by_me: boolean;
  created_at: string;
  updated_at: string;
};

export type GroupComment = {
  id: string;
  post_id: string;
  author_member_id: string;
  author_handle: string;
  author_display_name: string;
  body: string;
  created_at: string;
};

export type CreateGroupInput = {
  name: string;
  slug: string;
  description?: string;
  kind?: string;
  visibility?: string;
  join_policy?: string;
};

export type CreatePostInput = {
  title?: string;
  body: string;
  kind?: string;
};

/** Distinguish flag-off (404) from other failures. */
export class GroupsDarkError extends Error {
  constructor() {
    super("GROUPS_ENABLED is dark");
    this.name = "GroupsDarkError";
  }
}

async function parseList(res: Response, fallback: string): Promise<CommunityGroup[]> {
  if (res.status === 404) throw new GroupsDarkError();
  if (!res.ok) throw await failure(res, fallback);
  const body = (await res.json()) as { data: CommunityGroup[] };
  return body.data ?? [];
}

export async function listGroups(limit = 40): Promise<CommunityGroup[]> {
  const res = await fetch(`${API_BASE}/api/v1/groups?limit=${limit}`, { cache: "no-store" });
  return parseList(res, "Failed to load groups");
}

export async function searchGroups(q: string, limit = 40): Promise<CommunityGroup[]> {
  const params = new URLSearchParams({ q, limit: String(limit) });
  const res = await fetch(`${API_BASE}/api/v1/groups/search?${params}`, { cache: "no-store" });
  return parseList(res, "Failed to search groups");
}

export async function getGroup(
  slug: string,
  viewer?: Viewer | null,
): Promise<CommunityGroup | null> {
  const res = await fetch(`${API_BASE}/api/v1/groups/${encodeURIComponent(slug)}`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load group");
  return ((await res.json()) as { data: CommunityGroup }).data;
}

export async function createGroup(
  viewer: Viewer,
  input: CreateGroupInput,
): Promise<CommunityGroup> {
  const res = await fetch(`${API_BASE}/api/v1/groups`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify(input),
  });
  if (!res.ok) throw await failure(res, "Failed to create group");
  return ((await res.json()) as { data: CommunityGroup }).data;
}

export async function joinGroup(viewer: Viewer, slug: string): Promise<{ role: string }> {
  const res = await fetch(`${API_BASE}/api/v1/groups/${encodeURIComponent(slug)}/join`, {
    method: "POST",
    headers: headers(viewer),
    body: "{}",
  });
  if (!res.ok) throw await failure(res, "Failed to join group");
  return ((await res.json()) as { data: { role: string } }).data;
}

export async function listPosts(
  slug: string,
  viewer?: Viewer | null,
  limit = 40,
): Promise<GroupPost[]> {
  const res = await fetch(
    `${API_BASE}/api/v1/groups/${encodeURIComponent(slug)}/posts?limit=${limit}`,
    { headers: headers(viewer), cache: "no-store" },
  );
  if (res.status === 404) return [];
  if (!res.ok) throw await failure(res, "Failed to load posts");
  return ((await res.json()) as { data: GroupPost[] }).data ?? [];
}

export async function createPost(
  viewer: Viewer,
  slug: string,
  input: CreatePostInput,
): Promise<GroupPost> {
  const res = await fetch(`${API_BASE}/api/v1/groups/${encodeURIComponent(slug)}/posts`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify(input),
  });
  if (!res.ok) throw await failure(res, "Failed to create post");
  return ((await res.json()) as { data: GroupPost }).data;
}

export async function listComments(
  slug: string,
  postId: string,
  viewer?: Viewer | null,
  limit = 100,
): Promise<GroupComment[]> {
  const res = await fetch(
    `${API_BASE}/api/v1/groups/${encodeURIComponent(slug)}/posts/${encodeURIComponent(postId)}/comments?limit=${limit}`,
    { headers: headers(viewer), cache: "no-store" },
  );
  if (res.status === 404) return [];
  if (!res.ok) throw await failure(res, "Failed to load comments");
  return ((await res.json()) as { data: GroupComment[] }).data ?? [];
}

export async function addComment(
  viewer: Viewer,
  slug: string,
  postId: string,
  body: string,
): Promise<GroupComment> {
  const res = await fetch(
    `${API_BASE}/api/v1/groups/${encodeURIComponent(slug)}/posts/${encodeURIComponent(postId)}/comments`,
    {
      method: "POST",
      headers: headers(viewer),
      body: JSON.stringify({ body }),
    },
  );
  if (!res.ok) throw await failure(res, "Failed to add comment");
  return ((await res.json()) as { data: GroupComment }).data;
}

export async function reactToPost(
  viewer: Viewer,
  slug: string,
  postId: string,
): Promise<{ reaction_count: number; liked_by_me: boolean }> {
  const res = await fetch(
    `${API_BASE}/api/v1/groups/${encodeURIComponent(slug)}/posts/${encodeURIComponent(postId)}/reactions`,
    {
      method: "POST",
      headers: headers(viewer),
      body: JSON.stringify({ reaction: "like" }),
    },
  );
  if (!res.ok) throw await failure(res, "Failed to react");
  return ((await res.json()) as { data: { reaction_count: number; liked_by_me: boolean } }).data;
}

export function kindLabel(kind: string): string {
  switch (kind) {
    case "skill":
      return "Skill";
    case "industry":
      return "Industry";
    case "location":
      return "Location";
    case "company":
      return "Company";
    case "ai_agent":
      return "AI agent";
    default:
      return kind;
  }
}

export function slugify(name: string): string {
  return name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 64);
}
