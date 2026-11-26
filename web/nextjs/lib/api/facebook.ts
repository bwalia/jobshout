import { apiClient } from "@/lib/api/client";
import type {
  FacebookAnalytics,
  FacebookComment,
  FacebookPage,
  FacebookPost,
  FacebookStatus,
} from "@/lib/types/facebook";

// Drafting starts through POST /tasks/launch (New task, Run task, chat), so
// the launch fields live in one schema on the server. This module covers the
// Facebook connection, Pages, and reviewing, scheduling and publishing posts.

const post = (id: string, suffix = "") => `/facebook/posts/${encodeURIComponent(id)}${suffix}`;
const page = (id: string, suffix = "") => `/facebook/pages/${encodeURIComponent(id)}${suffix}`;

export async function getFacebookStatus(): Promise<FacebookStatus> {
  const { data } = await apiClient.get<FacebookStatus>("/facebook/status");
  return data;
}

/** Returns Facebook's consent URL; the browser goes there to connect. */
export async function startFacebookConnect(): Promise<string> {
  const { data } = await apiClient.post<{ url: string }>("/facebook/connection/oauth/start");
  return data.url;
}

export async function disconnectFacebook(): Promise<void> {
  await apiClient.delete("/facebook/connection");
}

export async function saveFacebookProfile(
  pageId: string,
  body: { brief: string; voice: string; timezone: string }
): Promise<FacebookPage> {
  const { data } = await apiClient.patch<FacebookPage>(page(pageId, "/profile"), body);
  return data;
}

export async function setFacebookAutomation(pageId: string, paused: boolean): Promise<FacebookPage> {
  const { data } = await apiClient.post<FacebookPage>(page(pageId, "/automation"), { paused });
  return data;
}

export async function refreshFacebookPage(pageId: string): Promise<FacebookPage> {
  const { data } = await apiClient.post<FacebookPage>(page(pageId, "/refresh"));
  return data;
}

export async function updateFacebookPageInfo(
  pageId: string,
  body: { about?: string; description?: string; website?: string }
): Promise<FacebookPage> {
  const { data } = await apiClient.patch<FacebookPage>(page(pageId, "/info"), body);
  return data;
}

export async function getFacebookAnalytics(pageId: string): Promise<FacebookAnalytics> {
  const { data } = await apiClient.get<FacebookAnalytics>(page(pageId, "/analytics"));
  return data;
}

export async function listFacebookPosts(pageId?: string): Promise<FacebookPost[]> {
  const { data } = await apiClient.get<{ data: FacebookPost[] }>("/facebook/posts", {
    params: pageId ? { page: pageId } : undefined,
  });
  return Array.isArray(data.data) ? data.data : [];
}

export async function updateFacebookPost(
  id: string,
  body: { message: string; scheduled_for?: string; clear_schedule?: boolean }
): Promise<FacebookPost> {
  const { data } = await apiClient.patch<FacebookPost>(post(id), body);
  return data;
}

export async function editPublishedFacebookPost(id: string, message: string): Promise<FacebookPost> {
  const { data } = await apiClient.patch<FacebookPost>(post(id, "/published"), { message });
  return data;
}

/** Schedules a draft; a time in the past publishes it now. */
export async function approveFacebookPost(id: string, scheduledFor?: string): Promise<FacebookPost> {
  const { data } = await apiClient.post<FacebookPost>(post(id, "/approve"), {
    scheduled_for: scheduledFor,
  });
  return data;
}

export async function publishFacebookPost(id: string): Promise<FacebookPost> {
  const { data } = await apiClient.post<FacebookPost>(post(id, "/publish"));
  return data;
}

export async function unscheduleFacebookPost(id: string): Promise<FacebookPost> {
  const { data } = await apiClient.post<FacebookPost>(post(id, "/unschedule"));
  return data;
}

export async function redraftFacebookPost(id: string): Promise<FacebookPost> {
  const { data } = await apiClient.post<FacebookPost>(post(id, "/redraft"));
  return data;
}

export async function generateFacebookImage(id: string): Promise<FacebookPost> {
  const { data } = await apiClient.post<FacebookPost>(post(id, "/image"));
  return data;
}

export async function deleteFacebookPost(id: string): Promise<void> {
  await apiClient.delete(post(id));
}

export async function listFacebookComments(id: string): Promise<FacebookComment[]> {
  const { data } = await apiClient.get<{ data: FacebookComment[] }>(post(id, "/comments"));
  return Array.isArray(data.data) ? data.data : [];
}

export async function replyToFacebookComment(
  id: string,
  commentId: string,
  message: string
): Promise<void> {
  await apiClient.post(post(id, "/comments/reply"), { comment_id: commentId, message });
}
