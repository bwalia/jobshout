"use client";

import { useRef, useEffect } from "react";
import { ArrowUp } from "lucide-react";
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
  disabled,
  placeholder,
  variant = "docked",
}: {
  value: string;
  onChange: (v: string) => void;
  onSend: () => void;
  disabled?: boolean;
  placeholder?: string;
  variant?: "hero" | "docked";
}) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const showSlash = value.startsWith("/") && !value.includes(" ");
  const hero = variant === "hero";

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, hero ? 220 : 160)}px`;
  }, [value, hero]);

  return (
    <div className="relative w-full">
      {showSlash ? (
        <ul className="absolute bottom-full mb-1 w-full rounded-lg border border-border bg-card p-1 shadow-lg">
          {SLASH.filter((s) => s.cmd.startsWith(value)).map((s) => (
            <li key={s.cmd}>
              <button
                type="button"
                className="flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left text-sm hover:bg-secondary"
                onClick={() => onChange(s.hint)}
              >
                <span className="font-mono text-xs">{s.cmd}</span>
                <span className="text-muted-foreground">{s.hint}</span>
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      <div
        className={cn(
          "border border-border bg-card focus-within:border-primary/40",
          hero ? "rounded-2xl px-3 pt-3 pb-2 shadow-sm" : "rounded-xl px-3 pt-2 pb-2"
        )}
      >
        <textarea
          ref={ref}
          rows={hero ? 3 : 1}
          value={value}
          disabled={disabled}
          placeholder={
            placeholder ??
            (hero
              ? "Ask JobShout to build, fix bugs, explore"
              : "Ask JobShout to run an agent, create a task, check status…")
          }
          className={cn(
            "w-full resize-none bg-transparent text-sm outline-none placeholder:text-muted-foreground",
            hero ? "min-h-[72px]" : "min-h-[24px] max-h-40"
          )}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.nativeEvent.isComposing) return;
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              if (!disabled && value.trim()) onSend();
            }
          }}
        />
        <div className="mt-1 flex items-center justify-end">
          <button
            type="button"
            disabled={disabled || !value.trim()}
            onClick={onSend}
            aria-label="Send message"
            className={cn(
              "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground",
              (disabled || !value.trim()) && "opacity-40"
            )}
          >
            <ArrowUp className="h-4 w-4" />
          </button>
        </div>
      </div>
    </div>
  );
}
