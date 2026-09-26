import { apiClient } from "@/lib/api/client";
import type { PaginatedResponse, PaginationParams } from "@/lib/types/common";
import type {
  ChatMessage,
  ChatSession,
  ChatTurnResult,
  ChatStreamEvent,
} from "@/lib/types/chat";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export async function listChatSessions(
  params: PaginationParams = {}
): Promise<PaginatedResponse<ChatSession>> {
  const { data } = await apiClient.get<PaginatedResponse<ChatSession>>(
    "/chat/sessions",
    { params }
  );
  return data;
}

export async function createChatSession(): Promise<ChatSession> {
  const { data } = await apiClient.post<ChatSession>("/chat/sessions", {
    source: "web",
  });
  return data;
}

export async function deleteChatSession(id: string): Promise<void> {
  await apiClient.delete(`/chat/sessions/${id}`);
}

export async function getChatMessages(
  sessionId: string,
  limit = 100
): Promise<ChatMessage[]> {
  const { data } = await apiClient.get<ChatMessage[]>(
    `/chat/sessions/${sessionId}/messages`,
    { params: { limit } }
  );
  return data ?? [];
}

export async function sendChatMessage(
  sessionId: string,
  content: string,
  confirmationToken?: string
): Promise<ChatTurnResult> {
  const { data } = await apiClient.post<ChatTurnResult>(
    `/chat/sessions/${sessionId}/messages`,
    { content, source: "web", confirmation_token: confirmationToken || undefined },
    { timeout: 120000 }
  );
  return data;
}

export interface StreamChatOptions {
  confirmationToken?: string;
  signal?: AbortSignal;
  /**
   * What the transcript should show for this message when it differs from
   * content — e.g. a clarify pick sends the option's machine value as content
   * and the clicked label here.
   */
  displayContent?: string;
}

export async function streamChatMessage(
  sessionId: string,
  content: string,
  onEvent: (ev: ChatStreamEvent) => void,
  options: StreamChatOptions = {}
): Promise<void> {
  const res = await postChatStream(sessionId, content, options);
  if (!res.ok || !res.body) {
    throw new Error(`Chat stream failed (${res.status})`);
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    const chunks = buf.split("\n\n");
    buf = chunks.pop() ?? "";
    for (const chunk of chunks) {
      const ev = parseSse(chunk);
      if (ev) onEvent(ev);
    }
  }
  buf += decoder.decode();
  if (buf.trim()) {
    const ev = parseSse(buf);
    if (ev) onEvent(ev);
  }
}

async function postChatStream(
  sessionId: string,
  content: string,
  options: StreamChatOptions,
  retried = false
): Promise<Response> {
  const token =
    typeof window !== "undefined" ? localStorage.getItem("access_token") : null;
  const res = await fetch(
    `${API_BASE_URL}/api/v1/chat/sessions/${sessionId}/messages/stream`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({
        content,
        source: "web",
        confirmation_token: options.confirmationToken || undefined,
        display_content: options.displayContent || undefined,
      }),
      signal: options.signal,
    }
  );
  if (res.status === 401 && !retried) {
    const refreshed = await refreshAccessTokenOnce();
    if (refreshed) {
      return postChatStream(sessionId, content, options, true);
    }
  }
  return res;
}

async function refreshAccessTokenOnce(): Promise<boolean> {
  if (typeof window === "undefined") return false;
  const refreshToken = localStorage.getItem("refresh_token");
  if (!refreshToken) return false;
  try {
    const res = await fetch(`${API_BASE_URL}/api/v1/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });
    if (!res.ok) return false;
    const data = (await res.json()) as {
      access_token?: string;
      refresh_token?: string;
    };
    if (!data.access_token) return false;
    localStorage.setItem("access_token", data.access_token);
    if (data.refresh_token) {
      localStorage.setItem("refresh_token", data.refresh_token);
    }
    return true;
  } catch {
    return false;
  }
}

function parseSse(chunk: string): ChatStreamEvent | null {
  let type = "";
  let data = "";
  for (const line of chunk.split("\n")) {
    if (line.startsWith("event:")) type = line.slice(6).trim();
    if (line.startsWith("data:")) data += line.slice(5).trim();
  }
  if (!data) return null;
  try {
    const parsed = JSON.parse(data) as ChatStreamEvent;
    if (type && !parsed.type) parsed.type = type as ChatStreamEvent["type"];
    return parsed;
  } catch {
    return null;
  }
}
