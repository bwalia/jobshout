"use client";

import { useEffect, useRef } from "react";
import { Loader2, Sparkles } from "lucide-react";

import { ChatMessageItem } from "@/components/chat/ChatMessageItem";
import type { ChatMessage } from "@/lib/types/chat";

interface ChatMessageListProps {
  messages: ChatMessage[];
  /** An in-flight optimistic user message not yet persisted. */
  pendingUser?: string | null;
  thinking?: boolean;
}

const SUGGESTIONS = [
  "List my agents",
  "Create a task to investigate the checkout-service restarts",
  "Run the Kubernetes investigation workflow",
  "Show me the status of my tasks",
];

export function ChatMessageList({
  messages,
  pendingUser,
  thinking,
}: ChatMessageListProps) {
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages.length, pendingUser, thinking]);

  const empty = messages.length === 0 && !pendingUser;

  if (empty) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-6 px-6 text-center">
        <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10 text-primary">
          <Sparkles className="h-7 w-7" />
        </div>
        <div>
          <h2 className="font-display text-xl font-semibold">Ask JobShout anything</h2>
          <p className="mt-1 max-w-md text-sm text-muted-foreground">
            Describe what you want done in plain language. JobShout picks the
            right agent, runs the task, and streams the result back here.
          </p>
        </div>
        <div className="grid w-full max-w-lg grid-cols-1 gap-2 sm:grid-cols-2">
          {SUGGESTIONS.map((s) => (
            <div
              key={s}
              className="rounded-lg border border-border bg-card px-3 py-2 text-left text-sm text-muted-foreground"
            >
              {s}
            </div>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-5 px-4 py-6">
      {messages.map((m) => (
        <ChatMessageItem key={m.id} message={m} />
      ))}
      {pendingUser && (
        <ChatMessageItem
          message={{
            id: "pending-user",
            session_id: "",
            org_id: "",
            role: "user",
            source: "web",
            content: pendingUser,
            metadata: {},
            created_at: new Date().toISOString(),
          }}
        />
      )}
      {thinking && (
        <div className="flex items-center gap-2 pl-11 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Thinking…
        </div>
      )}
      <div ref={bottomRef} />
    </div>
  );
}
