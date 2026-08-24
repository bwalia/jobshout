"use client";

import { Plus, Trash2 } from "lucide-react";
import { cn } from "@/lib/utils/cn";
import type { ChatSession } from "@/lib/types/chat";
import { sessionTitle } from "@/lib/types/chat";

export function SessionSidebar({
  sessions,
  activeId,
  onSelect,
  onNew,
  onDelete,
}: {
  sessions: ChatSession[];
  activeId: string | null;
  onSelect: (id: string) => void;
  onNew: () => void;
  onDelete: (id: string) => void;
}) {
  return (
    <aside className="flex w-56 shrink-0 flex-col border-r border-border">
      <div className="flex items-center justify-between px-3 py-2">
        <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Chats
        </p>
        <button
          type="button"
          onClick={onNew}
          className="rounded-md p-1 hover:bg-secondary"
          aria-label="New chat"
        >
          <Plus className="h-4 w-4" />
        </button>
      </div>
      <ul className="flex-1 overflow-y-auto scrollbar-thin px-1 pb-2">
        {sessions.map((s) => (
          <li key={s.id} className="group relative">
            <button
              type="button"
              onClick={() => onSelect(s.id)}
              className={cn(
                "w-full truncate rounded-md px-2 py-1.5 text-left text-sm",
                s.id === activeId
                  ? "bg-primary/10 text-foreground"
                  : "text-muted-foreground hover:bg-secondary"
              )}
            >
              {sessionTitle(s)}
            </button>
            <button
              type="button"
              aria-label="Delete chat"
              onClick={() => onDelete(s.id)}
              className="absolute right-1 top-1 hidden rounded p-1 text-muted-foreground hover:text-destructive group-hover:block"
            >
              <Trash2 className="h-3 w-3" />
            </button>
          </li>
        ))}
      </ul>
    </aside>
  );
}
