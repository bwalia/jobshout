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
  sessionId: string | null
): UseQueryResult<ChatMessage[]> {
  return useQuery({
    queryKey: chatKeys.messages(sessionId ?? ""),
    queryFn: () => getChatMessages(sessionId!),
    enabled: Boolean(sessionId),
  });
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
