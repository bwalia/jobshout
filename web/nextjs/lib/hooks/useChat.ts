import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryResult,
  type UseMutationResult,
} from "@tanstack/react-query";
import { toast } from "sonner";
import {
  listChatSessions,
  createChatSession,
  deleteChatSession,
  getChatMessages,
  sendChatMessage,
} from "@/lib/api/chat";
import type { ChatMessage, ChatSession, ChatTurnResult } from "@/lib/types/chat";
import type { PaginatedResponse } from "@/lib/types/common";

export const chatKeys = {
  all: ["chat"] as const,
  sessions: () => [...chatKeys.all, "sessions"] as const,
  messages: (id: string) => [...chatKeys.all, "messages", id] as const,
};

export function useChatSessions(): UseQueryResult<PaginatedResponse<ChatSession>> {
  return useQuery({
    queryKey: chatKeys.sessions(),
    queryFn: () => listChatSessions({ page: 1, per_page: 50 }),
  });
}

export function useChatMessages(
  sessionId: string | null,
  opts?: { pollForReply?: boolean }
): UseQueryResult<ChatMessage[]> {
  return useQuery({
    queryKey: chatKeys.messages(sessionId ?? ""),
    queryFn: () => getChatMessages(sessionId!),
    enabled: Boolean(sessionId),
    refetchInterval: (query) => {
      if (!opts?.pollForReply) return false;
      const msgs = query.state.data;
      return awaitingAssistant(msgs ?? []) ? 1500 : false;
    },
  });
}

/** True when the last visible message is the user and the reply may still be in flight. */
export function awaitingAssistant(messages: ChatMessage[]): boolean {
  const vis = messages.filter((m) => m.role === "user" || m.role === "agent");
  const last = vis[vis.length - 1];
  if (!last || last.role !== "user") return false;
  const t = Date.parse(last.created_at);
  if (Number.isNaN(t)) return true;
  return Date.now() - t < 3 * 60 * 1000;
}

export function useCreateChatSession(): UseMutationResult<ChatSession, Error, void> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createChatSession,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: chatKeys.sessions() });
    },
  });
}

export function useDeleteChatSession(): UseMutationResult<void, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteChatSession,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: chatKeys.sessions() });
      toast.success("Chat deleted");
    },
  });
}

export function useSendChatMessage(
  sessionId: string | null
): UseMutationResult<
  ChatTurnResult,
  Error,
  { content: string; confirmationToken?: string }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ content, confirmationToken }) => {
      if (!sessionId) throw new Error("No chat session");
      return sendChatMessage(sessionId, content, confirmationToken);
    },
    onSuccess: () => {
      if (sessionId) {
        qc.invalidateQueries({ queryKey: chatKeys.messages(sessionId) });
      }
      qc.invalidateQueries({ queryKey: chatKeys.sessions() });
    },
    onError: (err) => {
      toast.error(err.message || "Couldn't send that message");
    },
  });
}
