"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import {
  useChatMessages,
  useCreateChatSession,
  awaitingAssistant,
  chatKeys,
} from "@/lib/hooks/useChat";
import { streamChatMessage } from "@/lib/api/chat";
import { useQueryClient } from "@tanstack/react-query";
import type { ChatMessage } from "@/lib/types/chat";
import { MessageList } from "./MessageList";
import { Composer } from "./Composer";
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
  const router = useRouter();
  const searchParams = useSearchParams();
  const createSession = useCreateChatSession();
  const sessionFromUrl = searchParams.get("session");
  const [sessionId, setSessionId] = useState<string | null>(sessionFromUrl);
  const [draft, setDraft] = useState("");
  const [failedDraft, setFailedDraft] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [streaming, setStreaming] = useState("");
  const [runningLabel, setRunningLabel] = useState<string | null>(null);
  const [pending, setPending] = useState<ChatMessage | null>(null);
  const messagesQuery = useChatMessages(sessionId, { pollForReply: !busy });

  useEffect(() => {
    if (sessionFromUrl) {
      setSessionId(sessionFromUrl);
      return;
    }
    setSessionId(null);
    setPending(null);
    setStreaming("");
    setFailedDraft(null);
    setBusy(false);
    setRunningLabel(null);
  }, [sessionFromUrl]);

  const ensureSession = useCallback(async () => {
    if (sessionId) return sessionId;
    const s = await createSession.mutateAsync();
    setSessionId(s.id);
    router.replace(`/chat?session=${s.id}`);
    return s.id;
  }, [sessionId, createSession, router]);

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
  const isEmpty =
    !sessionId && messages.length === 0 && !busy && !pending;

  useEffect(() => {
    if (!pending) return;
    const synced = (messagesQuery.data ?? []).some(
      (m) => m.role === "user" && m.content === pending.content
    );
    if (synced) setPending(null);
  }, [messagesQuery.data, pending]);

  const failedRestore = failedDraft ? (
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
  ) : null;

  const composer = (
    <Composer
      value={draft}
      onChange={setDraft}
      onSend={onSend}
      disabled={busy || waitingOnTurn}
      variant={isEmpty ? "hero" : "docked"}
    />
  );

  return (
    <div
      className={cn(
        "flex h-[calc(100dvh-3rem)] flex-col bg-background lg:h-screen",
        className
      )}
      data-chat-layout={isEmpty ? "hero" : "docked"}
    >
      {isEmpty ? (
        <div className="flex flex-1 flex-col items-center justify-center px-4">
          <div className="flex w-full max-w-2xl flex-col items-center">
            {failedRestore}
            {composer}
            <div className="mt-4 flex max-w-lg flex-wrap justify-center gap-2">
              {EXAMPLES.map((p) => (
                <button
                  key={p}
                  type="button"
                  onClick={() => {
                    setDraft("");
                    void send(p);
                  }}
                  className="rounded-full border border-border px-3 py-1.5 text-xs text-muted-foreground hover:border-primary/40 hover:bg-primary/10 hover:text-foreground"
                >
                  {p}
                </button>
              ))}
            </div>
          </div>
        </div>
      ) : (
        <div className="mx-auto flex min-h-0 w-full max-w-3xl flex-1 flex-col px-4 py-4">
          <MessageList
            messages={messages}
            streamingText={streaming}
            runningLabel={runningLabel ?? (waitingOnTurn ? "Working…" : null)}
            busy={busy || waitingOnTurn}
            onConfirm={(token) => void send("yes", token)}
            onCancel={() => void send("cancel")}
            onClarify={(v) => void send(v)}
          />
          {failedRestore}
          {composer}
        </div>
      )}
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
