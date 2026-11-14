import { API_BASE, failure, headers, type Viewer } from "@/lib/insights";
import type { Member } from "@/lib/members";

export type Conversation = {
  id: string;
  kind: string;
  subject: string;
  updated_at: string;
  unread: number;
  peer_handle?: string | null;
  peer_display_name?: string | null;
  last_body?: string | null;
};

export type Message = {
  id: string;
  conversation_id: string;
  sender_id: string;
  sender_handle: string;
  body: string;
  created_at: string;
};

/** null when messaging is switched off on this ring (the API answers 404). */
export async function listConversations(viewer: Viewer): Promise<Conversation[] | null> {
  const res = await fetch(`${API_BASE}/api/v1/messages`, { headers: headers(viewer), cache: "no-store" });
  if (res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load messages");
  return ((await res.json()) as { data: Conversation[] }).data;
}

/** Messages newest first, as the API returns them. Reading marks the thread read. */
export async function listMessages(viewer: Viewer, conversationId: string): Promise<Message[]> {
  const res = await fetch(`${API_BASE}/api/v1/messages/${conversationId}?limit=100`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load the conversation");
  return ((await res.json()) as { data: Message[] }).data;
}

export async function openConversation(viewer: Viewer, handle: string): Promise<Conversation> {
  const res = await fetch(`${API_BASE}/api/v1/messages`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify({ handle }),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Could not start the conversation");
  return (await res.json()) as Conversation;
}

export async function sendMessage(viewer: Viewer, conversationId: string, body: string): Promise<Message> {
  const res = await fetch(`${API_BASE}/api/v1/messages/${conversationId}`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify({ body }),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Message not sent");
  return (await res.json()) as Message;
}

export async function setBlocked(viewer: Viewer, handle: string, blocked: boolean): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/members/${encodeURIComponent(handle)}/block`, {
    method: blocked ? "POST" : "DELETE",
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, blocked ? "Could not block" : "Could not unblock");
}

export type Connections = { incoming: Member[]; outgoing: Member[]; accepted: Member[] };

/** null when rich member features are off on this ring. */
export async function getConnections(viewer: Viewer): Promise<Connections | null> {
  const res = await fetch(`${API_BASE}/api/v1/members/me/connections`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load connections");
  return (await res.json()) as Connections;
}

export async function requestConnection(viewer: Viewer, handle: string): Promise<string> {
  const res = await fetch(`${API_BASE}/api/v1/members/${encodeURIComponent(handle)}/connect`, {
    method: "PUT",
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Could not send the request");
  return ((await res.json()) as { status: string }).status;
}

export async function respondConnection(viewer: Viewer, handle: string, accept: boolean): Promise<string> {
  const res = await fetch(`${API_BASE}/api/v1/members/${encodeURIComponent(handle)}/connect`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify({ accept }),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Could not answer the request");
  return ((await res.json()) as { status: string }).status;
}
