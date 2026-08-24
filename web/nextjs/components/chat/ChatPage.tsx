"use client";

import { useCallback, useEffect, useState } from "react";
import { Sparkles } from "lucide-react";
import {
  useChatMessages,
  useChatSessions,
  useCreateChatSession,
  useDeleteChatSession,
  awaitingAssistant,
} from "@/lib/hooks/useChat";
import { streamChatMessage } from "@/lib/api/chat";
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
  const [draft, setDraft] = useState("");
  const [failedDraft, setFailedDraft] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [streaming, setStreaming] = useState("");
  const [runningLabel, setRunningLabel] = useState<string | null>(null);
  const [pending, setPending] = useState<ChatMessage | null>(null);
  const messagesQuery = useChatMessages(sessionId, { pollForReply: !busy });

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
      setPending(optimisticUserMessage(content, sessionId));
      setBusy(true);
      setStreaming("");
      setRunningLabel("Working…");
      let id = sessionId;
      try {
        id = await ensureSession();
        await streamChatMessage(
          id,
          content || "yes",
          (ev) => {
            if (ev.type === "token" && ev.token) {
              setRunningLabel(null);
              setStreaming((s) => s + ev.token);
            }
            if (ev.type === "tool_call") {
              setRunningLabel(ev.label || ev.tool || "Working…");
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
      } catch (err) {
        if (id) {
          void qc.invalidateQueries({ queryKey: chatKeys.messages(id) });
        }
        if (isDisconnectError(err)) {
          // Reload aborted the POST; the server finishes the turn and we catch
          // up by polling messages.
          return;
        }
        setPending(null);
        setFailedDraft(content);
      } finally {
        setBusy(false);
        setStreaming("");
        setRunningLabel(null);
      }
    },
    [ensureSession, qc, sessionId]
  );

  const onSend = () => {
    const text = draft;
    setDraft("");
    void send(text);
  };

  const messages: ChatMessage[] = withPending(messagesQuery.data ?? [], pending);
  const waitingOnTurn = !busy && awaitingAssistant(messages);

  useEffect(() => {
    if (!pending) return;
    const synced = (messagesQuery.data ?? []).some(
      (m) => m.role === "user" && m.content === pending.content
    );
    if (synced) setPending(null);
  }, [messagesQuery.data, pending]);

  return (
    <div className={cn("flex h-[calc(100vh-8rem)] overflow-hidden rounded-xl border border-border bg-card", className)}>
      <SessionSidebar
        sessions={sessions}
        activeId={sessionId}
        onSelect={(id) => {
          setPending(null);
          setSessionId(id);
        }}
        onNew={() => {
          setPending(null);
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
        {messages.length === 0 && !busy && !pending ? (
          <EmptyState onPick={(p) => { setDraft(""); void send(p); }} />
        ) : (
          <MessageList
            messages={messages}
            streamingText={streaming}
            runningLabel={runningLabel ?? (waitingOnTurn ? "Working…" : null)}
            busy={busy || waitingOnTurn}
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
          disabled={busy || waitingOnTurn}
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

function optimisticUserMessage(content: string, sessionId: string | null): ChatMessage {
  return {
    id: `pending-${Date.now()}`,
    session_id: sessionId ?? "pending",
    org_id: "",
    role: "user",
    source: "web",
    content,
    metadata: {},
    created_at: new Date().toISOString(),
  };
}

function withPending(messages: ChatMessage[], pending: ChatMessage | null): ChatMessage[] {
  if (!pending) return messages;
  const last = [...messages].reverse().find((m) => m.role === "user" || m.role === "agent");
  if (last?.role === "user" && last.content === pending.content) return messages;
  return [...messages, pending];
}

function isDisconnectError(err: unknown): boolean {
  if (!err || typeof err !== "object") return false;
  const e = err as { name?: string; message?: string };
  if (e.name === "AbortError") return true;
  const msg = (e.message ?? "").toLowerCase();
  return msg.includes("abort") || msg.includes("body stream") || msg.includes("networkerror");
}
