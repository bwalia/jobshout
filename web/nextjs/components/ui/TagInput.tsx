"use client";

import { useRef, type KeyboardEvent, type ClipboardEvent } from "react";
import { flushSync } from "react-dom";
import { X } from "lucide-react";
import { cn } from "@/lib/utils/cn";

export function parseTags(raw: string): string[] {
  return raw
    .split(/[,;\n]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

export function uniqueTags(tags: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const t of tags) {
    const key = t.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(t);
  }
  return out;
}

interface TagInputProps {
  id?: string;
  value: string[];
  onChange: (tags: string[]) => void;
  placeholder?: string;
  disabled?: boolean;
  /** Short hint under the field. */
  hint?: string;
  className?: string;
  /** Default box height. Titles need more room than a handful of emails. */
  size?: "md" | "lg";
}

/**
 * Chip/tag editor: Enter, comma, or Tab commits; × removes; Backspace on an
 * empty draft removes the last chip. The box is vertically resizable.
 */
export function TagInput({
  id,
  value,
  onChange,
  placeholder = "Type and press Enter",
  disabled,
  hint,
  className,
  size = "md",
}: TagInputProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const draftRef = useRef("");

  function commit(raw: string, rest = "") {
    const next = uniqueTags([...value, ...parseTags(raw)]);
    // Save/click runs after blur. flushSync so the parent sees the new chip
    // in the same turn instead of PATCHing the previous (empty) list.
    flushSync(() => {
      onChange(next);
    });
    if (inputRef.current) inputRef.current.value = rest;
    draftRef.current = rest;
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    const draft = e.currentTarget.value;
    if (e.key === "Tab" && draft.trim()) {
      commit(draft);
      return;
    }
    if (e.key === "Enter" || e.key === ",") {
      if (draft.trim()) {
        e.preventDefault();
        commit(draft);
      } else if (e.key === "Enter") {
        e.preventDefault();
      }
      return;
    }
    if (e.key === "Backspace" && !draft && value.length > 0) {
      e.preventDefault();
      onChange(value.slice(0, -1));
    }
    if (e.key === "Escape") {
      e.currentTarget.value = "";
      draftRef.current = "";
    }
  }

  function onPaste(e: ClipboardEvent<HTMLInputElement>) {
    const text = e.clipboardData.getData("text");
    if (!/[,;\n]/.test(text)) return;
    e.preventDefault();
    commit(e.currentTarget.value + text);
  }

  function removeAt(i: number) {
    onChange(value.filter((_, idx) => idx !== i));
    inputRef.current?.focus();
  }

  const sizeCls =
    size === "lg"
      ? "h-[9rem] min-h-[9rem] max-h-[40vh]"
      : "h-[4.5rem] min-h-[4.5rem] max-h-64";

  return (
    <div className="space-y-1">
      <div
        className={cn(
          "w-full resize-y overflow-auto rounded-md border border-input bg-background focus-within:ring-2 focus-within:ring-ring",
          sizeCls,
          disabled && "opacity-50",
          className
        )}
        onClick={() => inputRef.current?.focus()}
      >
        <div className="flex min-h-full flex-wrap content-start gap-1.5 p-2">
          {value.map((tag, i) => (
            <span
              key={`${tag}-${i}`}
              className="inline-flex max-w-full items-center gap-1 rounded-full bg-primary/15 px-2 py-0.5 text-sm text-foreground"
            >
              <span className="truncate">{tag}</span>
              <button
                type="button"
                aria-label={`Remove ${tag}`}
                disabled={disabled}
                onClick={(e) => {
                  e.stopPropagation();
                  removeAt(i);
                }}
                className="rounded-full p-0.5 text-muted-foreground hover:bg-background/60 hover:text-foreground"
              >
                <X className="h-3 w-3" strokeWidth={2.5} />
              </button>
            </span>
          ))}
          <input
            ref={inputRef}
            id={id}
            type="text"
            disabled={disabled}
            autoComplete="off"
            placeholder={value.length === 0 ? placeholder : "Add another"}
            onKeyDown={onKeyDown}
            onPaste={onPaste}
            onBlur={(e) => {
              if (e.currentTarget.value.trim()) commit(e.currentTarget.value);
            }}
            onChange={(e) => {
              draftRef.current = e.currentTarget.value;
            }}
            className="min-w-[8rem] flex-1 bg-transparent px-1 py-0.5 text-sm outline-none placeholder:text-muted-foreground"
          />
        </div>
      </div>
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  );
}
