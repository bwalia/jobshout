"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { AlertTriangle, RotateCcw, X } from "lucide-react";
import {
  useChatMessages,
  useChatSessions,
  useCreateChatSession,
  awaitingAssistant,
  chatKeys,
} from "@/lib/hooks/useChat";
import { streamChatMessage } from "@/lib/api/chat";
import { useQueryClient } from "@tanstack/react-query";
import { sessionTitle, type ChatMessage, type ChatResponse } from "@/lib/types/chat";
import { MessageList } from "./MessageList";
import { Composer } from "./Composer";
import { ChatHeader } from "./ChatHeader";
import { ChatEmptyState } from "./ChatEmptyState";
import { ChatContextRail } from "./ChatContextRail";
import { cn } from "@/lib/utils/cn";

export function ChatPage({ className }: { className?: string }) {
  const qc = useQueryClient();
  const router = useRouter();
  const searchParams = useSearchParams();
  const createSession = useCreateChatSession();
  const sessionFromUrl = searchParams.get("session");
  const [sessionId, setSessionId] = useState<string | null>(sessionFromUrl);
  const [draft, setDraft] = useState("");
  const [failedDraft, setFailedDraft] = useState<string | null>(null);
  const [errorText, setErrorText] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [streaming, setStreaming] = useState("");
  const [runningLabel, setRunningLabel] = useState<string | null>(null);
  const [pending, setPending] = useState<ChatMessage | null>(null);
  const [turnFailed, setTurnFailed] = useState(false);
  /** Model the server said is answering. Sticky between turns so the header
   *  keeps naming it once the reply lands. */
  const [servingModel, setServingModel] = useState<string | null>(null);
  const messagesQuery = useChatMessages(sessionId, { pollForReply: !busy });
  const sessionsQuery = useChatSessions();
  const abortRef = useRef<AbortController | null>(null);
  const turnRef = useRef(0);
  const turnStartedAtRef = useRef(0);
  const sessionIdRef = useRef<string | null>(sessionFromUrl);
  const lastSentRef = useRef<string | null>(null);

  useEffect(() => {
    // New-chat first send sets the id, then the URL — don't treat that as a switch.
    if (sessionFromUrl === sessionIdRef.current) return;
    turnRef.current += 1;
    abortRef.current?.abort();
    abortRef.current = null;
    sessionIdRef.current = sessionFromUrl;
    setSessionId(sessionFromUrl);
    setPending(null);
    setStreaming("");
    setFailedDraft(null);
    setErrorText(null);
    setBusy(false);
    setRunningLabel(null);
    setTurnFailed(false);
    setServingModel(null);
    setDraft("");
  }, [sessionFromUrl]);

  const ensureSession = useCallback(async () => {
    if (sessionId) return sessionId;
    const s = await createSession.mutateAsync();
    sessionIdRef.current = s.id;
    setSessionId(s.id);
    router.replace(`/chat?session=${s.id}`);
    return s.id;
  }, [sessionId, createSession, router]);

  const send = useCallback(
    async (text: string, opts?: { token?: string; display?: string }) => {
      const content = text.trim();
      const token = opts?.token;
      if (!content && !token) return;
      const shown = opts?.display?.trim() || content;
      const turn = ++turnRef.current;
      turnStartedAtRef.current = Date.now();
      abortRef.current?.abort();
      const ac = new AbortController();
      abortRef.current = ac;
      lastSentRef.current = shown;

      setFailedDraft(null);
      setErrorText(null);
      setTurnFailed(false);
      setPending(optimisticUserMessage(shown, sessionId));
      setBusy(true);
      setStreaming("");
      setRunningLabel("Thinking");
      let id = sessionId;
      try {
        id = await ensureSession();
        if (turn !== turnRef.current) return;
        await streamChatMessage(
          id,
          content || "yes",
          (ev) => {
            if (turn !== turnRef.current) return;
            if (ev.type === "token" && ev.token) {
              setRunningLabel(null);
              setStreaming((s) => s + ev.token);
            }
            if (ev.type === "model" && ev.model) {
              setServingModel(ev.model);
            }
            if (ev.type === "tool_call") {
              setRunningLabel(ev.label || humanise(ev.tool) || "Working");
            }
            if (ev.type === "error") {
              setFailedDraft(shown);
              setErrorText(ev.error || "The agent could not finish this turn.");
              setTurnFailed(true);
            }
          },
          {
            confirmationToken: token,
            displayContent: shown !== content ? shown : undefined,
            signal: ac.signal,
          }
        );
        if (turn !== turnRef.current) return;
        await qc.invalidateQueries({ queryKey: chatKeys.messages(id) });
        await qc.invalidateQueries({ queryKey: chatKeys.sessions() });
      } catch (err) {
        if (turn !== turnRef.current) return;
        if (id) {
          void qc.invalidateQueries({ queryKey: chatKeys.messages(id) });
        }
        if (isDisconnectError(err)) {
          return;
        }
        setPending(null);
        setFailedDraft(shown);
        setErrorText(errorMessage(err));
        setTurnFailed(true);
      } finally {
        if (turn !== turnRef.current) return;
        setBusy(false);
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

  /** Abort the in-flight turn. The server keeps whatever it already wrote,
   *  so we refetch rather than dropping the partial reply on the floor. */
  const onStop = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setBusy(false);
    setRunningLabel(null);
    setStreaming("");
    if (sessionId) {
      void qc.invalidateQueries({ queryKey: chatKeys.messages(sessionId) });
    }
  }, [qc, sessionId]);

  const messages: ChatMessage[] = withPending(messagesQuery.data ?? [], pending);
  const waitingOnTurn = !busy && !turnFailed && awaitingAssistant(messages);
  const isEmpty = !sessionId && messages.length === 0 && !busy && !pending;
  const inFlight = busy || waitingOnTurn;

  const title = useMemo(() => {
    const s = (sessionsQuery.data?.data ?? []).find((x) => x.id === sessionId);
    if (s) return sessionTitle(s);
    const firstUser = messages.find((m) => m.role === "user");
    if (firstUser) return truncate(firstUser.content, 60);
    return "New chat";
  }, [sessionsQuery.data, sessionId, messages]);

  // The live event wins; before the first turn of a reloaded session, fall
  // back to whichever model signed the last reply.
  const model = servingModel ?? lastReplyModel(messages);

  const status = turnFailed
    ? "error"
    : streaming
      ? "streaming"
      : inFlight
        ? "working"
        : "idle";

  useEffect(() => {
    if (!pending) return;
    if (pendingSynced(messagesQuery.data ?? [], pending)) {
      setPending(null);
    }
  }, [messagesQuery.data, pending]);

  // Keep the typed reply on screen until the persisted agent message is in
  // the list — clearing on `done` made every turn blink out, then back in.
  useEffect(() => {
    if (!streaming) return;
    const vis = (messagesQuery.data ?? []).filter(
      (m) => m.role === "user" || m.role === "agent"
    );
    const last = vis[vis.length - 1];
    if (last?.role !== "agent") return;
    const ts = Date.parse(last.created_at);
    if (!Number.isNaN(ts) && ts + 2000 < turnStartedAtRef.current) return;
    setStreaming("");
  }, [messagesQuery.data, streaming]);

  const errorBanner = errorText ? (
    <div
      role="alert"
      className="mb-2 flex items-start gap-2.5 rounded-xl border border-destructive/30 bg-destructive/5 px-3 py-2.5"
    >
      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-foreground">
          That turn didn&apos;t finish
        </p>
        <p className="mt-0.5 break-words text-sm leading-6 text-muted-foreground">
          {errorText}
        </p>
      </div>
      {failedDraft ? (
        <button
          type="button"
          onClick={() => {
            const text = failedDraft;
            setFailedDraft(null);
            setErrorText(null);
            void send(text);
          }}
          className="flex h-8 shrink-0 items-center gap-1 rounded-md border border-border bg-card px-2.5 text-xs font-medium transition-colors hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <RotateCcw className="h-3 w-3" />
          Retry
        </button>
      ) : null}
      <button
        type="button"
        onClick={() => setErrorText(null)}
        aria-label="Dismiss error"
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  ) : null;

  /** Prompts ending in a space are a starter to finish, not a command to
   *  fire — put those in the box instead of sending them. */
  const onPick = useCallback(
    (prompt: string) => {
      if (prompt.endsWith(" ")) {
        setDraft(prompt);
        document.getElementById("chat-composer")?.focus();
        return;
      }
      setDraft("");
      void send(prompt);
    },
    [send]
  );

  const composer = (
    <Composer
      value={draft}
      onChange={setDraft}
      onSend={onSend}
      onStop={onStop}
      streaming={inFlight}
      disabled={inFlight}
      variant={isEmpty ? "hero" : "docked"}
    />
  );

  return (
    <div
      className={cn("flex h-[calc(100dvh-3rem)] bg-background lg:h-screen", className)}
      data-chat-layout={isEmpty ? "hero" : "docked"}
    >
      {/* The transcript takes the whole column rather than a centred strip;
          the rail beside it is what used to be empty margin. */}
      <div className="flex min-w-0 flex-1 flex-col">
        {isEmpty ? (
          <div className="scrollbar-thin flex flex-1 flex-col justify-center overflow-y-auto px-6 py-8">
            <div className="mx-auto flex w-full max-w-5xl flex-col">
              <ChatEmptyState onPick={onPick} />
              <div className="mt-5">
                {errorBanner}
                {composer}
              </div>
            </div>
          </div>
        ) : (
          <div className="flex min-h-0 w-full flex-1 flex-col px-5 sm:px-6">
            <ChatHeader
              title={title}
              status={status}
              model={model}
              turns={
                messages.filter((m) => m.role === "user" || m.role === "agent").length
              }
              onNewChat={() => router.push("/chat")}
            />
            <MessageList
              key={sessionId ?? "new"}
              messages={messages}
              streamingText={streaming}
              runningLabel={runningLabel ?? (waitingOnTurn ? "Working" : null)}
              model={model}
              busy={inFlight}
              stickToBottom={busy}
              onConfirm={(token) => void send("yes", { token })}
              onCancel={() => void send("cancel")}
              onClarify={(value, label) => void send(value, { display: label })}
              onRetry={() => {
                const text = lastSentRef.current;
                if (text) void send(text);
              }}
            />
            <div className="pb-4">
              {errorBanner}
              {composer}
            </div>
          </div>
        )}
      </div>

      <ChatContextRail
        messages={messages}
        onPick={onPick}
        liveModel={model}
        showSpecialists={!isEmpty}
        className="hidden xl:flex"
      />
    </div>
  );
}

/** Model named by the most recent agent reply in this session, if any. */
function lastReplyModel(messages: ChatMessage[]): string | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const envelope = messages[i].metadata?.response as ChatResponse | undefined;
    const name = envelope?.usage?.model;
    if (name) return name;
  }
  return null;
}

function truncate(s: string, n: number): string {
  const t = s.trim().replace(/\s+/g, " ");
  return t.length > n ? `${t.slice(0, n - 1)}…` : t;
}

function humanise(tool?: string): string {
  return tool ? tool.replace(/_/g, " ") : "";
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "Something went wrong sending that message.";
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

function pendingSynced(messages: ChatMessage[], pending: ChatMessage): boolean {
  const lastUser = [...messages].reverse().find((m) => m.role === "user");
  if (!lastUser || lastUser.content !== pending.content) return false;
  const lastTs = Date.parse(lastUser.created_at);
  const pendingTs = Date.parse(pending.created_at);
  if (Number.isNaN(lastTs) || Number.isNaN(pendingTs)) return true;
  return lastTs >= pendingTs - 2000;
}

function withPending(
  messages: ChatMessage[],
  pending: ChatMessage | null
): ChatMessage[] {
  if (!pending) return messages;
  if (pendingSynced(messages, pending)) return messages;
  return [...messages, pending];
}

function isDisconnectError(err: unknown): boolean {
  if (!err || typeof err !== "object") return false;
  const e = err as { name?: string; message?: string };
  if (e.name === "AbortError") return true;
  const msg = (e.message ?? "").toLowerCase();
  return (
    msg.includes("abort") || msg.includes("body stream") || msg.includes("networkerror")
  );
}
