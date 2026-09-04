"use client";

import { useEffect, useRef, useState } from "react";
import { ArrowDown } from "lucide-react";
import { cn } from "@/lib/utils/cn";
import type { ChatMessage } from "@/lib/types/chat";
import { MessageBubble } from "./MessageBubble";
import { Markdown } from "./Markdown";
import { ThinkingIndicator } from "./ThinkingIndicator";
import { dayLabel, sameDay } from "./time";

export function MessageList({
  messages,
  streamingText,
  runningLabel,
  model,
  onConfirm,
  onCancel,
  onClarify,
  onRetry,
  busy,
  stickToBottom,
}: {
  messages: ChatMessage[];
  streamingText?: string;
  runningLabel?: string | null;
  model?: string | null;
  onConfirm?: (token: string) => void;
  onCancel?: () => void;
  onClarify?: (value: string, label: string) => void;
  onRetry?: () => void;
  busy?: boolean;
  stickToBottom?: boolean;
}) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const scrollerRef = useRef<HTMLDivElement>(null);
  const [locked, setLocked] = useState(false);
  const [missed, setMissed] = useState(0);

  useEffect(() => {
    if (stickToBottom) setLocked(false);
  }, [stickToBottom]);

  useEffect(() => {
    if (!locked) {
      bottomRef.current?.scrollIntoView({ behavior: "auto" });
      setMissed(0);
    } else {
      setMissed((n) => n + 1);
    }
    // `locked` deliberately excluded: unlocking should not count as a miss.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messages, streamingText, runningLabel]);

  const vis = messages.filter((m) => m.role === "user" || m.role === "agent");
  const last = vis[vis.length - 1];
  const liveAgentId = last?.role === "agent" ? last.id : null;

  return (
    <div className="relative min-h-0 flex-1">
      <div
        ref={scrollerRef}
        className="scrollbar-thin h-full space-y-4 overflow-y-auto px-1 py-4"
        onScroll={() => {
          const el = scrollerRef.current;
          if (!el) return;
          const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
          setLocked(!atBottom);
          if (atBottom) setMissed(0);
        }}
      >
        {vis.map((m, i) => {
          const prev = vis[i - 1];
          const newDay = !prev || !sameDay(prev.created_at, m.created_at);
          // Consecutive agent turns drop the repeated avatar and name so a
          // multi-part answer reads as one block rather than three strangers.
          const grouped =
            !newDay &&
            prev?.role === "agent" &&
            m.role === "agent" &&
            withinGroupWindow(prev.created_at, m.created_at);

          return (
            <div key={m.id} className="space-y-4">
              {newDay ? <DaySeparator iso={m.created_at} /> : null}
              <MessageBubble
                message={m}
                isLatest={m.id === liveAgentId}
                showAvatar={!grouped}
                answeredAs={vis[i + 1]?.role === "user" ? vis[i + 1].content : undefined}
                onConfirm={onConfirm}
                onCancel={onCancel}
                onClarify={onClarify}
                onRetry={onRetry}
                busy={busy}
              />
            </div>
          );
        })}

        {/* The streamed reply is rendered with the same markdown pipeline as
            the persisted one, so text does not reflow when the turn lands. */}
        {streamingText ? (
          <div className="flex gap-3">
            <div className="w-7 shrink-0" />
            <div className="min-w-0 flex-1">
              <Markdown className="chat-caret">{streamingText}</Markdown>
            </div>
          </div>
        ) : null}

        {runningLabel && !streamingText ? (
          <ThinkingIndicator label={runningLabel} model={model} />
        ) : null}

        <div ref={bottomRef} />
      </div>

      {/* Screen readers get the finished reply once, rather than every token
          of the stream re-announced by a live region on the whole scroller. */}
      <p className="sr-only" aria-live="polite" aria-atomic="true">
        {busy ? runningLabel || "Working" : last?.role === "agent" ? last.content : ""}
      </p>

      <div className="pointer-events-none absolute inset-x-0 bottom-0 flex justify-center pb-3">
        <button
          type="button"
          onClick={() => {
            setLocked(false);
            bottomRef.current?.scrollIntoView({ behavior: "smooth" });
          }}
          className={cn(
            "pointer-events-auto flex h-9 items-center gap-1.5 rounded-full border border-border bg-card px-3 text-xs font-medium shadow-card-hover transition-all duration-200",
            "hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            locked
              ? "translate-y-0 opacity-100"
              : "pointer-events-none translate-y-2 opacity-0"
          )}
          aria-hidden={!locked}
          tabIndex={locked ? 0 : -1}
        >
          <ArrowDown className="h-3.5 w-3.5" />
          {missed > 0 ? `${missed} new` : "Jump to latest"}
        </button>
      </div>
    </div>
  );
}

function DaySeparator({ iso }: { iso: string }) {
  const label = dayLabel(iso);
  if (!label) return null;
  return (
    <div className="flex items-center gap-3 py-1">
      <span className="h-px flex-1 bg-border" />
      <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
      <span className="h-px flex-1 bg-border" />
    </div>
  );
}

function withinGroupWindow(a: string, b: string): boolean {
  const ta = Date.parse(a);
  const tb = Date.parse(b);
  if (Number.isNaN(ta) || Number.isNaN(tb)) return false;
  return Math.abs(tb - ta) < 5 * 60 * 1000;
}
