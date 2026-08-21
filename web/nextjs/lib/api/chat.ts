import { apiClient } from "@/lib/api/client";
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
