import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from "@tanstack/react-query";
import { toast } from "sonner";

import { apiErrorMessage } from "@/lib/api/client";
import {
  getChatMessages,
  getChatSessions,
  sendChatMessage,
  startChatSession,
} from "@/lib/api/chat";
import type { PaginatedResponse } from "@/lib/types/common";
import type {
  ChatMessage,
  ChatSession,
  SendChatMessageResponse,
  StartChatSessionRequest,
} from "@/lib/types/chat";

export const chatKeys = {
  all: ["chat"] as const,
  sessions: () => [...chatKeys.all, "sessions"] as const,
  messages: (sessionId: string) =>
    [...chatKeys.all, "messages", sessionId] as const,
};

/** The current user's chat sessions, newest first. */
export function useChatSessions(): UseQueryResult<
  PaginatedResponse<ChatSession>
> {
  return useQuery({
    queryKey: chatKeys.sessions(),
    queryFn: () => getChatSessions({ per_page: 50 }),
  });
}

/** A session's message history. */
export function useChatMessages(
  sessionId: string | null
): UseQueryResult<ChatMessage[]> {
  return useQuery({
    queryKey: chatKeys.messages(sessionId ?? ""),
    queryFn: () => getChatMessages(sessionId as string),
    enabled: Boolean(sessionId),
  });
}

/** Start a new session, then refresh the sidebar list. */
export function useStartChatSession(): UseMutationResult<
  ChatSession,
  Error,
  StartChatSessionRequest | void
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload) => startChatSession(payload ?? {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: chatKeys.sessions() });
    },
    onError: (error: Error) => {
      toast.error(apiErrorMessage(error, "Failed to start chat"));
    },
  });
}

/**
 * Send a message in a session. On success it refreshes that session's messages
 * and the session list (recency). The caller can also read the returned
 * assistant reply directly to append optimistically.
 */
export function useSendChatMessage(): UseMutationResult<
  SendChatMessageResponse,
  Error,
  { sessionId: string; content: string }
> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ sessionId, content }) =>
      sendChatMessage(sessionId, { content }),
    onSuccess: (_data, { sessionId }) => {
      queryClient.invalidateQueries({
        queryKey: chatKeys.messages(sessionId),
      });
      queryClient.invalidateQueries({ queryKey: chatKeys.sessions() });
    },
    onError: (error: Error) => {
      toast.error(apiErrorMessage(error, "Failed to send message"));
    },
  });
}
