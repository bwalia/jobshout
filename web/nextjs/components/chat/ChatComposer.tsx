"use client";

import { useRef, useState } from "react";
import { ArrowUp, Bot, Loader2 } from "lucide-react";

import type { Agent } from "@/lib/types/agent";

interface ChatComposerProps {
  agents: Agent[];
  /** Currently selected agent id, or "" for Auto. */
  agentId: string;
  onAgentChange: (agentId: string) => void;
  onSend: (content: string) => void;
  sending: boolean;
  disabled?: boolean;
}

/**
 * ChatGPT-style composer: multiline textarea, Enter to send, Shift+Enter for a
 * newline, an agent selector (Auto = let the router pick), and a send button.
 */
export function ChatComposer({
  agents,
  agentId,
  onAgentChange,
  onSend,
  sending,
  disabled,
}: ChatComposerProps) {
  const [value, setValue] = useState("");
  const taRef = useRef<HTMLTextAreaElement>(null);

  function submit() {
    const content = value.trim();
    if (!content || sending || disabled) return;
    onSend(content);
    setValue("");
    if (taRef.current) taRef.current.style.height = "auto";
  }

  function autosize(el: HTMLTextAreaElement) {
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, 200) + "px";
  }

  return (
    <div className="rounded-2xl border border-border bg-card p-2 shadow-signal">
      <textarea
        ref={taRef}
        rows={1}
        value={value}
        disabled={disabled}
        onChange={(e) => {
          setValue(e.target.value);
          autosize(e.target);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            submit();
          }
        }}
        placeholder="Ask JobShout to do something…"
        className="max-h-52 w-full resize-none bg-transparent px-2 py-1.5 text-sm outline-none placeholder:text-muted-foreground"
      />
      <div className="flex items-center justify-between gap-2 px-1 pt-1">
        <label className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          <Bot className="h-3.5 w-3.5" />
          <select
            value={agentId}
            onChange={(e) => onAgentChange(e.target.value)}
            className="rounded-md border border-input bg-background px-2 py-1 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="">Auto</option>
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </label>
        <button
          onClick={submit}
          disabled={!value.trim() || sending || disabled}
          className="inline-flex h-8 w-8 items-center justify-center rounded-full bg-primary text-primary-foreground transition-opacity hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-40"
          aria-label="Send"
        >
          {sending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <ArrowUp className="h-4 w-4" />
          )}
        </button>
      </div>
    </div>
  );
}
