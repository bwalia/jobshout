"use client";

import { useCallback, useEffect, useState } from "react";
import { Sparkles } from "lucide-react";
import {
  useChatMessages,
  useChatSessions,
  useCreateChatSession,
  useDeleteChatSession,
} from "@/lib/hooks/useChat";
import { streamChatMessage, sendChatMessage } from "@/lib/api/chat";
import { chatKeys } from "@/lib/hooks/useChat";
import { useQueryClient } from "@tanstack/react-query";
import type { ChatMessage } from "@/lib/types/chat";
import { MessageList } from "./MessageList";
import { Composer } from "./Composer";
import { SessionSidebar } from "./SessionSidebar";
import { cn } from "@/lib/utils/cn";

const EXAMPLES = [
  "List my agents",
  "Create a task to fix the login timeout",
  "Who is working on what right now?",
  "Run the release check workflow",
  "What's my usage this month?",
  "What am I allowed to do?",
];

export function ChatPage({ className }: { className?: string }) {
  const qc = useQueryClient();
  const sessionsQuery = useChatSessions();
  const createSession = useCreateChatSession();
  const deleteSession = useDeleteChatSession();
  const [sessionId, setSessionId] = useState<string | null>(null);
  const messagesQuery = useChatMessages(sessionId);
  const [draft, setDraft] = useState("");
  const [failedDraft, setFailedDraft] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [streaming, setStreaming] = useState("");
  const [runningLabel, setRunningLabel] = useState<string | null>(null);

  const sessions = sessionsQuery.data?.data ?? [];

  useEffect(() => {
    if (!sessionId && sessions[0]) setSessionId(sessions[0].id);
  }, [sessionId, sessions]);

  const ensureSession = useCallback(async () => {
    if (sessionId) return sessionId;
    const s = await createSession.mutateAsync();
    setSessionId(s.id);
    return s.id;
  }, [sessionId, createSession]);

  const send = useCallback(
    async (text: string, token?: string) => {
      const content = text.trim();
      if (!content && !token) return;
      setFailedDraft(null);
      setBusy(true);
      setStreaming("");
      setRunningLabel(null);
      try {
        const id = await ensureSession();
        await streamChatMessage(
          id,
          content || "yes",
          (ev) => {
            if (ev.type === "token" && ev.token) {
              setStreaming((s) => s + ev.token);
            }
            if (ev.type === "tool_call") {
              setRunningLabel(ev.label || ev.tool || "Working…");
            }
            if (ev.type === "tool_result") {
              setRunningLabel(null);
            }
            if (ev.type === "done") {
              setStreaming("");
            }
            if (ev.type === "error") {
              setFailedDraft(content);
            }
          },
          token
        );
        await qc.invalidateQueries({ queryKey: chatKeys.messages(id) });
        await qc.invalidateQueries({ queryKey: chatKeys.sessions() });
      } catch {
        setFailedDraft(content);
        try {
          const id = await ensureSession();
          await sendChatMessage(id, content || "yes", token);
          await qc.invalidateQueries({ queryKey: chatKeys.messages(id) });
        } catch {
          /* keep failed draft */
        }
      } finally {
        setBusy(false);
        setStreaming("");
        setRunningLabel(null);
      }
    },
    [ensureSession, qc]
  );

  const onSend = () => {
    const text = draft;
    setDraft("");
    void send(text);
  };

  const messages: ChatMessage[] = messagesQuery.data ?? [];

  return (
    <div className={cn("flex h-[calc(100vh-8rem)] overflow-hidden rounded-xl border border-border bg-card", className)}>
      <SessionSidebar
        sessions={sessions}
        activeId={sessionId}
        onSelect={setSessionId}
        onNew={() => {
          createSession.mutate(undefined, {
            onSuccess: (s) => setSessionId(s.id),
          });
        }}
        onDelete={(id) => {
          deleteSession.mutate(id, {
            onSuccess: () => {
              if (sessionId === id) setSessionId(null);
            },
          });
        }}
      />
      <div className="flex min-w-0 flex-1 flex-col p-4">
        {messages.length === 0 && !busy ? (
          <EmptyState onPick={(p) => { setDraft(""); void send(p); }} />
        ) : (
          <MessageList
            messages={messages}
            streamingText={streaming}
            runningLabel={runningLabel}
            busy={busy}
            onConfirm={(token) => void send("yes", token)}
            onCancel={() => void send("cancel")}
            onClarify={(v) => void send(v)}
          />
        )}
        {failedDraft ? (
          <button
            type="button"
            className="mb-2 self-start text-xs text-destructive underline"
            onClick={() => {
              setDraft(failedDraft);
              setFailedDraft(null);
            }}
          >
            Send failed — restore draft
          </button>
        ) : null}
        <Composer
          value={draft}
          onChange={setDraft}
          onSend={onSend}
          disabled={busy}
        />
      </div>
    </div>
  );
}

function EmptyState({ onPick }: { onPick: (prompt: string) => void }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-4 text-center">
      <Sparkles className="h-8 w-8 text-primary" />
      <div>
        <h2 className="font-display text-lg font-semibold">JobShout AI</h2>
        <p className="mt-1 max-w-md text-sm text-muted-foreground">
          Drive agents, tasks, workflows and the rest of the platform in plain language.
          This chat is separate from Telegram — each keeps its own history.
        </p>
      </div>
      <div className="flex max-w-lg flex-wrap justify-center gap-2">
        {EXAMPLES.map((p) => (
          <button
            key={p}
            type="button"
            onClick={() => onPick(p)}
            className="rounded-full border border-border px-3 py-1.5 text-xs hover:border-primary/40 hover:bg-primary/10"
          >
            {p}
          </button>
        ))}
      </div>
    </div>
  );
}
