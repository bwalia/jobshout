import { apiClient } from "@/lib/api/client";
import { getAccessToken } from "@/lib/auth/auth";
import type { PaginatedResponse, PaginationParams } from "@/lib/types/common";
import type {
  ChatMessage,
  ChatSession,
  SendChatMessageResponse,
  StartChatSessionRequest,
  SendChatMessageRequest,
} from "@/lib/types/chat";

// Wraps the EXISTING /chat backend (ChatService + the 12-stage ChatRouterService).
// All paths are relative to the apiClient baseURL (/api/v1).

/** Start a new chat session (thread). */
export async function startChatSession(
  payload: StartChatSessionRequest = {}
): Promise<ChatSession> {
  const { data } = await apiClient.post<ChatSession>("/chat/sessions", {
    source: "web",
    ...payload,
  });
  return data;
}

/** List the current user's chat sessions, newest first. */
export async function getChatSessions(
  params: PaginationParams = {}
): Promise<PaginatedResponse<ChatSession>> {
  const { data } = await apiClient.get<PaginatedResponse<ChatSession>>(
    "/chat/sessions",
    { params }
  );
  return data;
}

/** Fetch a session's message history. */
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

/**
 * Send a user turn. The backend persists it, routes it through the intent
 * router (which may run an agent or workflow), and returns both the stored user
 * message and the assistant reply — the reply's metadata references any
 * execution/workflow it launched.
 */
export async function sendChatMessage(
  sessionId: string,
  payload: SendChatMessageRequest
): Promise<SendChatMessageResponse> {
  const { data } = await apiClient.post<SendChatMessageResponse>(
    `/chat/sessions/${sessionId}/messages`,
    { source: "web", ...payload }
  );
  return data;
}

const API_BASE =
  (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080") + "/api/v1";

export interface ChatStreamHandlers {
  onStatus?: (state: string, data: Record<string, unknown>) => void;
  onTool?: (data: Record<string, unknown>) => void;
  onMessage?: (data: Record<string, unknown>) => void;
  onError?: (message: string) => void;
}

/**
 * Send a message and consume the server's SSE progress stream. The endpoint is
 * a POST with a body, so this uses fetch + ReadableStream (EventSource is
 * GET-only). Handlers fire as frames arrive; the promise resolves when the
 * stream ends. Pass an AbortSignal to stop generation.
 */
export async function streamChatMessage(
  sessionId: string,
  content: string,
  handlers: ChatStreamHandlers,
  signal?: AbortSignal
): Promise<void> {
  const token = getAccessToken();
  const res = await fetch(
    `${API_BASE}/chat/sessions/${sessionId}/messages/stream`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({ content, source: "web" }),
      signal,
    }
  );
  if (!res.ok || !res.body) {
    handlers.onError?.(`stream failed (HTTP ${res.status})`);
    return;
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";

  const dispatch = (frame: string) => {
    let event = "message";
    let dataStr = "";
    for (const line of frame.split("\n")) {
      if (line.startsWith("event:")) event = line.slice(6).trim();
      else if (line.startsWith("data:")) dataStr += line.slice(5).trim();
    }
    if (event === "done") return;
    let parsed: { type?: string; data?: Record<string, unknown> } = {};
    try {
      parsed = dataStr ? JSON.parse(dataStr) : {};
    } catch {
      return;
    }
    const type = parsed.type ?? event;
    const inner = parsed.data ?? {};
    if (type === "status") handlers.onStatus?.(String(inner.state ?? ""), inner);
    else if (type === "tool") handlers.onTool?.(inner);
    else if (type === "message") handlers.onMessage?.(inner);
    else if (type === "error")
      handlers.onError?.(String(inner.message ?? "error"));
  };

  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let idx;
    while ((idx = buf.indexOf("\n\n")) !== -1) {
      const frame = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      if (frame.trim()) dispatch(frame);
    }
  }
}
