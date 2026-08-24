"use client";

import { useCallback, useEffect, useState } from "react";
import { usePathname } from "next/navigation";
import { Sparkles, X } from "lucide-react";
import { ChatPage } from "./ChatPage";
import { cn } from "@/lib/utils/cn";

const STORAGE = "jobshout-chat-dock-open";

export function ChatDock() {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);

  useEffect(() => {
    try {
      setOpen(localStorage.getItem(STORAGE) === "1");
    } catch {
      /* ignore */
    }
  }, []);

  const toggle = useCallback((next: boolean) => {
    setOpen(next);
    try {
      localStorage.setItem(STORAGE, next ? "1" : "0");
    } catch {
      /* ignore */
    }
  }, []);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        toggle(!open);
      }
      if (e.key === "Escape" && open) toggle(false);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, toggle]);

  // The /chat route renders the full-page chat UI; the dock (and its floating
  // launcher) would overlay a second copy of the same surface there.
  if (pathname?.startsWith("/chat")) return null;

  return (
    <>
      <button
        type="button"
        onClick={() => toggle(true)}
        className={cn(
          "fixed bottom-5 right-5 z-40 flex h-12 w-12 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-lg hover:opacity-90",
          open && "hidden"
        )}
        aria-label="Open chat (⌘K)"
      >
        <Sparkles className="h-5 w-5" />
      </button>
      {open ? (
        <div
          className="fixed bottom-5 right-5 z-40 flex h-[min(720px,80vh)] w-[min(920px,calc(100vw-2rem))] flex-col overflow-hidden rounded-xl border border-border bg-background shadow-2xl"
          role="dialog"
          aria-label="JobShout chat"
        >
          <div className="flex items-center justify-between border-b border-border px-3 py-2">
            <p className="text-sm font-medium">Chat</p>
            <button
              type="button"
              onClick={() => toggle(false)}
              className="rounded-md p-1 hover:bg-secondary"
              aria-label="Close chat"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          <div className="min-h-0 flex-1">
            <ChatPage className="h-full rounded-none border-0" />
          </div>
        </div>
      ) : null}
    </>
  );
}
