"use client";

import { Plus, MessagesSquare } from "lucide-react";

import type { Agent } from "@/lib/types/agent";
import type { ChatSession } from "@/lib/types/chat";

interface ChatSessionSidebarProps {
  sessions: ChatSession[];
  agents: Agent[];
  activeId: string | null;
  onSelect: (id: string) => void;
  onNewChat: () => void;
  creating?: boolean;
}

function sessionLabel(s: ChatSession, agents: Agent[]): string {
  const agent = s.agent_id ? agents.find((a) => a.id === s.agent_id) : null;
  if (agent) return `Chat · ${agent.name}`;
  return "New chat";
}

function when(iso: string): string {
  const d = new Date(iso);
  const diff = Math.max(0, Date.now() - d.getTime());
  const mins = Math.round(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return d.toLocaleDateString();
}

export function ChatSessionSidebar({
  sessions,
  agents,
  activeId,
  onSelect,
  onNewChat,
  creating,
}: ChatSessionSidebarProps) {
  return (
    <div className="flex h-full flex-col gap-3">
      <button
        onClick={onNewChat}
        disabled={creating}
        className="inline-flex h-10 items-center justify-center gap-2 rounded-lg border border-border bg-card text-sm font-medium hover:bg-accent disabled:opacity-50"
      >
        <Plus className="h-4 w-4" /> New Chat
      </button>

      <div className="px-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
        Recent
      </div>

      <div className="flex-1 space-y-1 overflow-y-auto scrollbar-thin">
        {sessions.length === 0 ? (
          <p className="px-2 py-4 text-center text-xs text-muted-foreground">
            No conversations yet.
          </p>
        ) : (
          sessions.map((s) => {
            const active = s.id === activeId;
            return (
              <button
                key={s.id}
                onClick={() => onSelect(s.id)}
                className={
                  "flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-sm transition-colors " +
                  (active
                    ? "bg-primary/10 text-foreground"
                    : "text-muted-foreground hover:bg-accent hover:text-foreground")
                }
              >
                <MessagesSquare className="h-4 w-4 shrink-0" />
                <span className="min-w-0 flex-1 truncate">
                  {sessionLabel(s, agents)}
                </span>
                <span className="shrink-0 font-mono text-[10px] text-muted-foreground">
                  {when(s.updated_at)}
                </span>
              </button>
            );
          })
        )}
      </div>
    </div>
  );
}
