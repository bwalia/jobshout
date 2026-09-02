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
  stickToBottom,
}: {
  messages: ChatMessage[];
  streamingText?: string;
  runningLabel?: string | null;
  onConfirm?: (token: string) => void;
  onCancel?: () => void;
  onClarify?: (value: string, label: string) => void;
  busy?: boolean;
  stickToBottom?: boolean;
}) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const scrollerRef = useRef<HTMLDivElement>(null);
  const [locked, setLocked] = useState(false);

  useEffect(() => {
    if (stickToBottom) setLocked(false);
  }, [stickToBottom]);

  useEffect(() => {
    if (!locked) {
      bottomRef.current?.scrollIntoView({ behavior: "auto" });
    }
  }, [messages, streamingText, runningLabel, locked]);

  const vis = messages.filter((m) => m.role === "user" || m.role === "agent");
  const last = vis[vis.length - 1];
  const liveAgentId = last?.role === "agent" ? last.id : null;

  return (
    <div className="relative min-h-0 flex-1">
      <div
        ref={scrollerRef}
        className="h-full space-y-3 overflow-y-auto px-1 py-3 scrollbar-thin"
        aria-live="polite"
        onScroll={() => {
          const el = scrollerRef.current;
          if (!el) return;
          const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
          setLocked(!atBottom);
        }}
      >
        {vis.map((m, i) => (
          <MessageBubble
            key={m.id}
            message={m}
            isLatest={m.id === liveAgentId}
            answeredAs={vis[i + 1]?.role === "user" ? vis[i + 1].content : undefined}
            onConfirm={onConfirm}
            onCancel={onCancel}
            onClarify={onClarify}
            busy={busy}
          />
        ))}
        {streamingText ? (
          <div className="flex justify-start">
            <div className="max-w-[85%] min-w-0 break-words rounded-2xl bg-secondary px-3.5 py-2.5 text-sm">
              {streamingText}
            </div>
          </div>
        ) : null}
        {runningLabel ? <ToolCallChip running label={runningLabel} /> : null}
        <div ref={bottomRef} />
      </div>
      {locked ? (
        <button
          type="button"
          onClick={() => setLocked(false)}
          className="absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full border border-border bg-card px-3 py-1 text-xs shadow-sm hover:bg-secondary"
        >
          Jump to latest
        </button>
      ) : null}
    </div>
  );
}
