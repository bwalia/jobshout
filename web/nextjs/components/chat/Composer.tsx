"use client";

import { useEffect, useRef, useState } from "react";
import { ArrowUp, CornerDownLeft, Slash, Square } from "lucide-react";
import { cn } from "@/lib/utils/cn";

const SLASH = [
  { cmd: "/help", hint: "What can you do?" },
  { cmd: "/agents", hint: "List my agents" },
  { cmd: "/tasks", hint: "Show recent tasks" },
  { cmd: "/board", hint: "Who is working on what?" },
];

export function Composer({
  value,
  onChange,
  onSend,
  onStop,
  disabled,
  streaming,
  placeholder,
  variant = "docked",
}: {
  value: string;
  onChange: (v: string) => void;
  onSend: () => void;
  onStop?: () => void;
  disabled?: boolean;
  /** A turn is in flight — the send button becomes Stop. */
  streaming?: boolean;
  placeholder?: string;
  variant?: "hero" | "docked";
}) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const matches =
    value.startsWith("/") && !value.includes(" ")
      ? SLASH.filter((s) => s.cmd.startsWith(value))
      : [];
  const showSlash = matches.length > 0;
  const [highlight, setHighlight] = useState(0);
  const hero = variant === "hero";
  const canSend = Boolean(value.trim()) && !disabled;

  useEffect(() => {
    setHighlight(0);
  }, [value]);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, hero ? 220 : 180)}px`;
  }, [value, hero]);

  // "/" from anywhere on the page focuses the composer, the way every other
  // command surface in this app behaves.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== "/" || e.metaKey || e.ctrlKey || e.altKey) return;
      const t = e.target as HTMLElement | null;
      if (t && /^(INPUT|TEXTAREA)$/.test(t.tagName)) return;
      if (t?.isContentEditable) return;
      e.preventDefault();
      ref.current?.focus();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  function pickSlash(i: number) {
    const item = matches[i];
    if (item) {
      onChange(item.hint);
      ref.current?.focus();
    }
  }

  return (
    <div className="relative w-full">
      {showSlash ? (
        <ul
          role="listbox"
          aria-label="Slash commands"
          className="absolute bottom-full mb-2 w-full overflow-hidden rounded-xl border border-border bg-popover p-1 shadow-card-hover"
        >
          {matches.map((s, i) => (
            <li key={s.cmd} role="option" aria-selected={i === highlight}>
              <button
                type="button"
                className={cn(
                  "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm transition-colors",
                  i === highlight ? "bg-secondary" : "hover:bg-secondary/60"
                )}
                onMouseEnter={() => setHighlight(i)}
                onClick={() => pickSlash(i)}
              >
                <Slash className="h-3.5 w-3.5 shrink-0 text-primary" />
                <span className="font-mono text-xs font-medium">{s.cmd}</span>
                <span className="ml-auto truncate text-xs text-muted-foreground">
                  {s.hint}
                </span>
              </button>
            </li>
          ))}
        </ul>
      ) : null}

      <div
        className={cn(
          "rounded-2xl border border-border bg-card transition-shadow duration-200",
          "focus-within:border-primary/50 focus-within:shadow-card-hover",
          hero ? "px-3.5 pb-2 pt-3 shadow-card" : "px-3 pb-2 pt-2.5"
        )}
      >
        <label htmlFor="chat-composer" className="sr-only">
          Message JobShout
        </label>
        <textarea
          id="chat-composer"
          ref={ref}
          rows={hero ? 3 : 1}
          value={value}
          placeholder={
            placeholder ??
            (hero
              ? "Describe a job — “Review PR 184”, “Chase the overdue invoices”…"
              : "Reply, or ask for something else…")
          }
          className={cn(
            "w-full resize-none bg-transparent text-[15px] leading-7 outline-none placeholder:text-muted-foreground",
            hero ? "min-h-[72px]" : "min-h-[24px]"
          )}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.nativeEvent.isComposing) return;
            if (showSlash) {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                setHighlight((h) => (h + 1) % matches.length);
                return;
              }
              if (e.key === "ArrowUp") {
                e.preventDefault();
                setHighlight((h) => (h - 1 + matches.length) % matches.length);
                return;
              }
              if (e.key === "Escape") {
                e.preventDefault();
                onChange("");
                return;
              }
              if (e.key === "Tab" || e.key === "Enter") {
                e.preventDefault();
                pickSlash(highlight);
                return;
              }
            }
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              if (canSend) onSend();
            }
          }}
        />

        <div className="mt-1.5 flex items-center gap-2">
          <p className="hidden min-w-0 flex-1 items-center gap-1.5 text-xs text-muted-foreground sm:flex">
            <Kbd>
              <CornerDownLeft className="h-2.5 w-2.5" />
            </Kbd>
            send
            <span className="opacity-40">·</span>
            <Kbd>Shift</Kbd>
            <Kbd>
              <CornerDownLeft className="h-2.5 w-2.5" />
            </Kbd>
            newline
            <span className="opacity-40">·</span>
            <Kbd>/</Kbd>
            commands
          </p>
          <span className="flex-1 sm:hidden" />

          {streaming && onStop ? (
            <button
              type="button"
              onClick={onStop}
              aria-label="Stop generating"
              className="flex h-9 items-center gap-1.5 rounded-full border border-border px-3 text-xs font-medium text-foreground transition-colors hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-sm:h-11"
            >
              <Square className="h-3 w-3 fill-current" />
              Stop
            </button>
          ) : (
            <button
              type="button"
              disabled={!canSend}
              onClick={onSend}
              aria-label="Send message"
              className={cn(
                "flex h-9 w-9 shrink-0 items-center justify-center rounded-full transition-all duration-200 max-sm:h-11 max-sm:w-11",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card",
                canSend
                  ? "bg-primary text-primary-foreground hover:opacity-90"
                  : "cursor-not-allowed bg-muted text-muted-foreground"
              )}
            >
              <ArrowUp className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="inline-flex h-4 min-w-4 items-center justify-center rounded border border-border bg-muted px-1 font-sans text-xs font-medium text-muted-foreground">
      {children}
    </kbd>
  );
}
