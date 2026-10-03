import { apiClient } from "@/lib/api/client";
import type { LinkedInPost, LinkedInStatus } from "@/lib/types/linkedin";

// Drafting starts through POST /tasks/launch (New task, Run task, chat), so
// the launch fields live in one schema on the server. This module covers the
// LinkedIn connection and reviewing, editing and posting drafts.

export async function getLinkedInStatus(): Promise<LinkedInStatus> {
  const { data } = await apiClient.get<LinkedInStatus>("/linkedin/connection");
  return data;
}

/** Returns LinkedIn's consent URL; the browser goes there to connect. */
export async function startLinkedInConnect(): Promise<string> {
  const { data } = await apiClient.post<{ url: string }>("/linkedin/connection/oauth/start");
  return data.url;
}

export async function disconnectLinkedIn(): Promise<void> {
  await apiClient.delete("/linkedin/connection");
}

export async function listLinkedInPosts(): Promise<LinkedInPost[]> {
  const { data } = await apiClient.get<{ data: LinkedInPost[] }>("/linkedin/posts");
  return Array.isArray(data.data) ? data.data : [];
}

export async function updateLinkedInPost(id: string, commentary: string): Promise<LinkedInPost> {
  const { data } = await apiClient.patch<LinkedInPost>(`/linkedin/posts/${encodeURIComponent(id)}`, {
    commentary,
  });
  return data;
}

export async function redraftLinkedInPost(id: string): Promise<LinkedInPost> {
  const { data } = await apiClient.post<LinkedInPost>(
    `/linkedin/posts/${encodeURIComponent(id)}/redraft`
  );
  return data;
}

export async function publishLinkedInPost(id: string): Promise<LinkedInPost> {
  const { data } = await apiClient.post<LinkedInPost>(
    `/linkedin/posts/${encodeURIComponent(id)}/publish`
  );
  return data;
}
