"use client";

import { useRef, useState } from "react";
import { ArrowUp, Bot, Loader2, Slash } from "lucide-react";

import type { Agent } from "@/lib/types/agent";

// Slash commands expand to the natural-language query the existing router
// already understands — they are discoverable shortcuts, not a separate parser.
const SLASH_COMMANDS: { cmd: string; desc: string; send: string }[] = [
  { cmd: "/agents", desc: "List your agents", send: "List my agents" },
  { cmd: "/tasks", desc: "Show your tasks", send: "Show my tasks" },
  { cmd: "/workflows", desc: "List your workflows", send: "List my workflows" },
  { cmd: "/status", desc: "Status of recent work", send: "Show the status of my recent tasks" },
  { cmd: "/help", desc: "What can JobShout do?", send: "help" },
];

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

  const slashQuery = value.startsWith("/") ? value.slice(1).toLowerCase() : null;
  const slashMatches =
    slashQuery !== null
      ? SLASH_COMMANDS.filter((c) => c.cmd.slice(1).startsWith(slashQuery))
      : [];
  const showSlash = slashQuery !== null && slashMatches.length > 0;

  function send(content: string) {
    const trimmed = content.trim();
    if (!trimmed || sending || disabled) return;
    onSend(trimmed);
    setValue("");
    if (taRef.current) taRef.current.style.height = "auto";
  }

  function submit() {
    send(value);
  }

  function autosize(el: HTMLTextAreaElement) {
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, 200) + "px";
  }

  return (
    <div className="relative rounded-2xl border border-border bg-card p-2 shadow-signal">
      {showSlash && (
        <div className="absolute bottom-full left-0 mb-2 w-full overflow-hidden rounded-lg border border-border bg-popover shadow-signal">
          {slashMatches.map((c) => (
            <button
              key={c.cmd}
              onClick={() => send(c.send)}
              className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-accent"
            >
              <Slash className="h-3.5 w-3.5 text-muted-foreground" />
              <span className="font-mono">{c.cmd}</span>
              <span className="text-xs text-muted-foreground">{c.desc}</span>
            </button>
          ))}
        </div>
      )}
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
