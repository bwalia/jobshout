"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Search } from "lucide-react";
import { cn } from "@/lib/utils/cn";
import { PANELS } from "@/lib/panels";
import {
  useChatSessions,
  useCreateChatSession,
} from "@/lib/hooks/useChat";
import { sessionTitle } from "@/lib/types/chat";
import { useUiStore } from "@/lib/store/ui-store";

type Item =
  | { kind: "panel"; id: string; label: string; href: string }
  | { kind: "chat"; id: string; label: string; href: string }
  | { kind: "action"; id: string; label: string; run: () => void };

export function CommandPalette() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [focusIdx, setFocusIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const sessionsQuery = useChatSessions();
  const createSession = useCreateChatSession();
  const overrides = useUiStore((s) => s.chatTitleOverrides);

  const items = useMemo<Item[]>(() => {
    const q = query.trim().toLowerCase();
    const list: Item[] = [
      {
        kind: "action",
        id: "new-chat",
        label: "New chat",
        run: () => {
          void createSession.mutateAsync().then((s) => {
            router.push(`/chat?session=${s.id}`);
          });
        },
      },
      ...PANELS.map((p) => ({
        kind: "panel" as const,
        id: p.id,
        label: p.label,
        href: p.href,
      })),
      ...(sessionsQuery.data?.data ?? []).slice(0, 20).map((s) => ({
        kind: "chat" as const,
        id: s.id,
        label: overrides[s.id] ?? sessionTitle(s),
        href: `/chat?session=${s.id}`,
      })),
    ];
    if (!q) return list;
    return list.filter((i) => i.label.toLowerCase().includes(q));
  }, [query, sessionsQuery.data, overrides, createSession, router]);

  const close = useCallback(() => {
    setOpen(false);
    setQuery("");
    setFocusIdx(0);
  }, []);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((v) => !v);
      }
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "n") {
        e.preventDefault();
        void createSession.mutateAsync().then((s) => {
          router.push(`/chat?session=${s.id}`);
        });
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [createSession, router]);

  useEffect(() => {
    if (open) {
      setFocusIdx(0);
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        close();
      } else if (e.key === "ArrowDown") {
        e.preventDefault();
        setFocusIdx((i) => Math.min(i + 1, items.length - 1));
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        setFocusIdx((i) => Math.max(i - 1, 0));
      } else if (e.key === "Enter") {
        e.preventDefault();
        const item = items[focusIdx];
        if (!item) return;
        if (item.kind === "action") item.run();
        else router.push(item.href);
        close();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, items, focusIdx, close, router]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]">
      <button
        type="button"
        className="absolute inset-0 bg-black/40"
        aria-label="Close command palette"
        onClick={close}
      />
      <div
        role="dialog"
        aria-label="Command palette"
        className="relative z-10 w-full max-w-lg overflow-hidden rounded-xl border border-border bg-popover shadow-card-hover"
      >
        <div className="flex items-center gap-2 border-b border-border px-3">
          <Search className="h-4 w-4 text-muted-foreground" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setFocusIdx(0);
            }}
            placeholder="Jump to panel or chat…"
            className="h-11 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
          <kbd className="hidden rounded border border-border px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground sm:inline">
            esc
          </kbd>
        </div>
        <ul className="max-h-72 overflow-y-auto scrollbar-thin p-1">
          {items.length === 0 ? (
            <li className="px-3 py-6 text-center text-sm text-muted-foreground">
              No matches
            </li>
          ) : (
            items.map((item, idx) => (
              <li key={`${item.kind}-${item.id}`}>
                <button
                  type="button"
                  onMouseEnter={() => setFocusIdx(idx)}
                  onClick={() => {
                    if (item.kind === "action") item.run();
                    else router.push(item.href);
                    close();
                  }}
                  className={cn(
                    "flex w-full items-center justify-between rounded-md px-3 py-2 text-left text-sm",
                    idx === focusIdx
                      ? "bg-accent text-accent-foreground"
                      : "hover:bg-secondary"
                  )}
                >
                  <span>{item.label}</span>
                  <span className="text-[11px] text-muted-foreground">
                    {item.kind === "panel"
                      ? "Panel"
                      : item.kind === "chat"
                        ? "Chat"
                        : "Action"}
                  </span>
                </button>
              </li>
            ))
          )}
        </ul>
      </div>
    </div>
  );
}
