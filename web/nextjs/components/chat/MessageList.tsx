"use client";

import { useEffect, useRef, useState } from "react";
import type { ChatMessage } from "@/lib/types/chat";
import { MessageBubble } from "./MessageBubble";
import { ToolCallChip } from "./ToolCallChip";

export function MessageList({
  messages,
  streamingText,
  runningLabel,
  onConfirm,
  onCancel,
  onClarify,
  busy,
}: {
  messages: ChatMessage[];
  streamingText?: string;
  runningLabel?: string | null;
  onConfirm?: (token: string) => void;
  onCancel?: () => void;
  onClarify?: (value: string) => void;
  busy?: boolean;
}) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const scrollerRef = useRef<HTMLDivElement>(null);
  const [locked, setLocked] = useState(false);

  useEffect(() => {
    if (!locked) {
      bottomRef.current?.scrollIntoView({ behavior: "auto" });
    }
  }, [messages, streamingText, runningLabel, locked]);

  return (
    <div
      ref={scrollerRef}
      className="flex-1 space-y-3 overflow-y-auto px-1 py-3 scrollbar-thin"
      aria-live="polite"
      onScroll={() => {
        const el = scrollerRef.current;
        if (!el) return;
        const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        setLocked(!atBottom);
      }}
    >
      {messages
        .filter((m) => m.role === "user" || m.role === "agent")
        .map((m) => (
          <MessageBubble
            key={m.id}
            message={m}
            onConfirm={onConfirm}
            onCancel={onCancel}
            onClarify={onClarify}
            busy={busy}
          />
        ))}
      {runningLabel ? <ToolCallChip running label={runningLabel} /> : null}
      {streamingText ? (
        <div className="flex justify-start">
          <div className="max-w-[85%] rounded-2xl bg-secondary px-3.5 py-2.5 text-sm">
            {streamingText}
          </div>
        </div>
      ) : null}
      <div ref={bottomRef} />
    </div>
  );
}
