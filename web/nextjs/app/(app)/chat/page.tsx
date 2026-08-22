"use client";

import { useEffect, useMemo, useState } from "react";

import { ChatComposer } from "@/components/chat/ChatComposer";
import { ChatMessageList } from "@/components/chat/ChatMessageList";
import { ChatSessionSidebar } from "@/components/chat/ChatSessionSidebar";
import { useAgents } from "@/lib/hooks/useAgents";
import {
  useChatMessages,
  useChatSessions,
  useSendChatMessage,
  useStartChatSession,
} from "@/lib/hooks/useChat";

export default function ChatPage() {
  const { data: sessionsResp } = useChatSessions();
  const sessions = useMemo(() => sessionsResp?.data ?? [], [sessionsResp]);

  const { data: agentsResp } = useAgents({ per_page: 100 });
  const agents = useMemo(() => agentsResp?.data ?? [], [agentsResp]);

  const [activeId, setActiveId] = useState<string | null>(null);
  const [agentId, setAgentId] = useState("");
  const [pendingUser, setPendingUser] = useState<string | null>(null);

  const { data: messages } = useChatMessages(activeId);
  const startSession = useStartChatSession();
  const sendMessage = useSendChatMessage();

  // Default to the most recent session once the list loads.
  useEffect(() => {
    if (!activeId && sessions.length > 0) setActiveId(sessions[0].id);
  }, [sessions, activeId]);

  async function handleNewChat() {
    const session = await startSession.mutateAsync(
      agentId ? { agent_id: agentId } : {}
    );
    setActiveId(session.id);
    setPendingUser(null);
  }

  async function handleSend(content: string) {
    let sessionId = activeId;
    if (!sessionId) {
      const session = await startSession.mutateAsync(
        agentId ? { agent_id: agentId } : {}
      );
      sessionId = session.id;
      setActiveId(sessionId);
    }
    setPendingUser(content);
    try {
      await sendMessage.mutateAsync({ sessionId, content });
    } finally {
      setPendingUser(null);
    }
  }

  return (
    <div className="flex h-[calc(100vh-7rem)] gap-4">
      {/* Session list */}
      <aside className="hidden w-64 shrink-0 md:block">
        <ChatSessionSidebar
          sessions={sessions}
          agents={agents}
          activeId={activeId}
          onSelect={(id) => {
            setActiveId(id);
            setPendingUser(null);
          }}
          onNewChat={handleNewChat}
          creating={startSession.isPending}
        />
      </aside>

      {/* Conversation */}
      <div className="flex min-w-0 flex-1 flex-col rounded-xl border border-border bg-background">
        <div className="min-h-0 flex-1 overflow-y-auto scrollbar-thin">
          <ChatMessageList
            messages={messages ?? []}
            agents={agents}
            pendingUser={pendingUser}
            thinking={sendMessage.isPending}
          />
        </div>
        <div className="border-t border-border p-3">
          <div className="mx-auto max-w-3xl">
            <ChatComposer
              agents={agents}
              agentId={agentId}
              onAgentChange={setAgentId}
              onSend={handleSend}
              sending={sendMessage.isPending}
            />
            <p className="mt-1.5 text-center text-[11px] text-muted-foreground">
              JobShout picks an agent, runs the task, and shows the result. Auto
              lets the router choose.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
