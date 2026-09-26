"use client";

import Link from "next/link";
import { Cpu, LayoutDashboard, MessageSquarePlus, Sparkles } from "lucide-react";
import { cn } from "@/lib/utils/cn";

type Status = "idle" | "working" | "streaming" | "error";

const STATUS: Record<Status, { label: string; dot: string }> = {
  idle: { label: "Ready", dot: "bg-status-idle" },
  working: { label: "Working", dot: "bg-status-progress animate-chat-pulse" },
  streaming: { label: "Replying", dot: "bg-signal-live animate-chat-pulse" },
  error: { label: "Failed", dot: "bg-signal-error" },
};

/**
 * The transcript had no chrome at all — no title, no state, no way back to a
 * new chat without the sidebar. This strip carries the session identity and,
 * more usefully, whether anything is actually happening right now.
 */
export function ChatHeader({
  title,
  status,
  turns,
  model,
  onNewChat,
}: {
  title: string;
  status: Status;
  turns: number;
  /** Model answering this conversation, as reported by the server. */
  model?: string | null;
  onNewChat: () => void;
}) {
  const s = STATUS[status];

  return (
    <header className="flex h-12 shrink-0 items-center gap-3 border-b border-border px-1">
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-border bg-card">
        <Sparkles className="h-3.5 w-3.5 text-primary" />
      </span>

      <div className="flex min-w-0 flex-1 flex-col">
        <h1 className="truncate text-base font-semibold leading-tight text-foreground">
          {title}
        </h1>
        <p className="flex items-center gap-1.5 text-xs leading-tight text-muted-foreground">
          <span className={cn("h-1.5 w-1.5 rounded-full", s.dot)} aria-hidden="true" />
          {s.label}
          {turns > 0 ? (
            <>
              <span className="opacity-40">·</span>
              {turns} message{turns === 1 ? "" : "s"}
            </>
          ) : null}
        </p>
      </div>

      {model ? (
        <span
          title={`Answered by ${model}`}
          className="hidden shrink-0 items-center gap-1.5 rounded-full border border-border bg-card px-2.5 py-1 text-xs text-muted-foreground sm:flex"
        >
          <Cpu className="h-3.5 w-3.5" />
          <span className="font-mono">{model}</span>
        </span>
      ) : null}

      <Link
        href="/panel/task-board"
        title="Open task board"
        className="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-sm:h-11 max-sm:w-11"
      >
        <LayoutDashboard className="h-4 w-4" />
        <span className="sr-only">Open task board</span>
      </Link>
      <button
        type="button"
        onClick={onNewChat}
        title="New chat"
        className="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-sm:h-11 max-sm:w-11"
      >
        <MessageSquarePlus className="h-4 w-4" />
        <span className="sr-only">New chat</span>
      </button>
    </header>
  );
}
