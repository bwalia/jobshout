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
}: {
  value: string;
  onChange: (v: string) => void;
  onSend: () => void;
  disabled?: boolean;
  placeholder?: string;
}) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const showSlash = value.startsWith("/") && !value.includes(" ");

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }, [value]);

  return (
    <div className="relative">
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
      <div className="flex items-end gap-2 rounded-xl border border-border bg-card px-3 py-2 focus-within:border-primary/50">
        <textarea
          ref={ref}
          rows={1}
          value={value}
          disabled={disabled}
          placeholder={placeholder ?? "Ask JobShout to run an agent, create a task, check status…"}
          className="max-h-40 min-h-[24px] flex-1 resize-none bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              if (!disabled && value.trim()) onSend();
            }
          }}
        />
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
      <p className="mt-1 text-[10px] text-muted-foreground">⌘↵ to send</p>
    </div>
  );
}
