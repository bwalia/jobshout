"use client";

import { useEffect, useMemo, useState } from "react";
import { Menu, RefreshCw, X } from "lucide-react";

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
  const [drawerOpen, setDrawerOpen] = useState(false);

  const { data: messages } = useChatMessages(activeId);
  const startSession = useStartChatSession();
  const sendMessage = useSendChatMessage();

  useEffect(() => {
    if (!activeId && sessions.length > 0) setActiveId(sessions[0].id);
  }, [sessions, activeId]);

  const title = useMemo(() => {
    const firstUser = (messages ?? []).find((m) => m.role === "user");
    if (!firstUser) return "New chat";
    const t = firstUser.content.replace(/\n/g, " ").trim();
    return t.length > 70 ? t.slice(0, 70) + "…" : t;
  }, [messages]);

  const lastUserContent = useMemo(() => {
    const msgs = messages ?? [];
    for (let i = msgs.length - 1; i >= 0; i--) {
      if (msgs[i].role === "user") return msgs[i].content;
    }
    return null;
  }, [messages]);

  const canRegenerate =
    !sendMessage.isPending &&
    !pendingUser &&
    (messages?.length ?? 0) > 0 &&
    messages?.[messages.length - 1].role === "agent" &&
    Boolean(lastUserContent);

  async function ensureSession(): Promise<string> {
    if (activeId) return activeId;
    const session = await startSession.mutateAsync(
      agentId ? { agent_id: agentId } : {}
    );
    setActiveId(session.id);
    return session.id;
  }

  async function handleNewChat() {
    const session = await startSession.mutateAsync(
      agentId ? { agent_id: agentId } : {}
    );
    setActiveId(session.id);
    setPendingUser(null);
    setDrawerOpen(false);
  }

  async function runTurn(sessionId: string, content: string) {
    setPendingUser(content);
    try {
      await sendMessage.mutateAsync({ sessionId, content });
    } finally {
      setPendingUser(null);
    }
  }

  async function handleSend(content: string) {
    const sessionId = await ensureSession();
    await runTurn(sessionId, content);
  }

  async function handleRegenerate() {
    if (activeId && lastUserContent) await runTurn(activeId, lastUserContent);
  }

  const sidebar = (
    <ChatSessionSidebar
      sessions={sessions}
      agents={agents}
      activeId={activeId}
      onSelect={(id) => {
        setActiveId(id);
        setPendingUser(null);
        setDrawerOpen(false);
      }}
      onNewChat={handleNewChat}
      creating={startSession.isPending}
    />
  );

  return (
    <div className="flex h-[calc(100vh-7rem)] gap-4">
      {/* Session list — desktop column */}
      <aside className="hidden w-64 shrink-0 md:block">{sidebar}</aside>

      {/* Session list — mobile drawer */}
      {drawerOpen && (
        <div className="fixed inset-0 z-40 md:hidden" onClick={() => setDrawerOpen(false)}>
          <div className="absolute inset-0 bg-black/50" />
          <div
            className="absolute left-0 top-0 h-full w-72 border-r border-border bg-background p-4"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-3 flex items-center justify-between">
              <span className="font-display text-sm font-semibold">Chats</span>
              <button
                onClick={() => setDrawerOpen(false)}
                className="rounded-md p-1 text-muted-foreground hover:bg-accent"
                aria-label="Close"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            {sidebar}
          </div>
        </div>
      )}

      {/* Conversation */}
      <div className="flex min-w-0 flex-1 flex-col rounded-xl border border-border bg-background">
        {/* Header */}
        <div className="flex items-center gap-2 border-b border-border px-3 py-2.5">
          <button
            onClick={() => setDrawerOpen(true)}
            className="rounded-md p-1 text-muted-foreground hover:bg-accent md:hidden"
            aria-label="Open chats"
          >
            <Menu className="h-5 w-5" />
          </button>
          <h1 className="min-w-0 flex-1 truncate text-sm font-medium">{title}</h1>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto scrollbar-thin">
          <ChatMessageList
            messages={messages ?? []}
            agents={agents}
            pendingUser={pendingUser}
            thinking={sendMessage.isPending}
          />
          {canRegenerate && (
            <div className="mx-auto flex max-w-3xl justify-center px-4 pb-2">
              <button
                onClick={handleRegenerate}
                className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card px-3 py-1.5 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
              >
                <RefreshCw className="h-3.5 w-3.5" /> Regenerate
              </button>
            </div>
          )}
        </div>

        {/* Composer */}
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
