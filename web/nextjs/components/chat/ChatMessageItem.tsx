"use client";

import { useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Bot, Check, Copy, User } from "lucide-react";

import { ExecutionCard } from "@/components/chat/ExecutionCard";
import { WorkflowCard } from "@/components/chat/WorkflowCard";
import type { Agent } from "@/lib/types/agent";
import type { ChatMessage } from "@/lib/types/chat";

function CodeBlock({ children }: { children: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="group relative my-2">
      <button
        onClick={() => {
          navigator.clipboard?.writeText(children).then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          });
        }}
        className="absolute right-2 top-2 inline-flex items-center gap-1 rounded-md border border-border bg-card px-2 py-1 text-xs text-muted-foreground opacity-0 transition-opacity hover:text-foreground group-hover:opacity-100"
        aria-label="Copy code"
      >
        {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
        {copied ? "Copied" : "Copy"}
      </button>
      <pre className="overflow-x-auto rounded-md border border-border bg-background p-3 text-sm scrollbar-thin">
        <code>{children}</code>
      </pre>
    </div>
  );
}

export function ChatMessageItem({
  message,
  agents = [],
}: {
  message: ChatMessage;
  agents?: Agent[];
}) {
  const isUser = message.role === "user";
  const meta = message.metadata ?? {};
  const isError = meta.error === true;
  const agentName = meta.agent_id
    ? agents.find((a) => a.id === meta.agent_id)?.name
    : undefined;
  const [copiedMsg, setCopiedMsg] = useState(false);

  return (
    <div className={"flex gap-3 " + (isUser ? "flex-row-reverse" : "")}>
      {/* Avatar */}
      <div
        className={
          "flex h-8 w-8 shrink-0 items-center justify-center rounded-full " +
          (isUser
            ? "bg-primary/15 text-primary"
            : "bg-accent text-muted-foreground")
        }
      >
        {isUser ? <User className="h-4 w-4" /> : <Bot className="h-4 w-4" />}
      </div>

      {/* Bubble */}
      <div className={"min-w-0 max-w-[85%] " + (isUser ? "items-end" : "")}>
        <div
          className={
            "rounded-2xl px-4 py-2.5 text-sm " +
            (isUser
              ? "bg-primary text-primary-foreground"
              : isError
                ? "border border-signal-error/40 bg-signal-error/10 text-foreground"
                : "border border-border bg-card text-foreground")
          }
        >
          {isUser ? (
            <p className="whitespace-pre-wrap break-words">{message.content}</p>
          ) : (
            <div className="prose prose-sm dark:prose-invert max-w-none prose-pre:m-0 prose-pre:bg-transparent prose-pre:p-0">
              <ReactMarkdown
                remarkPlugins={[remarkGfm]}
                components={{
                  code({ className, children }) {
                    const text = String(children).replace(/\n$/, "");
                    // Fenced blocks carry a language class; inline code does not.
                    if (className) {
                      return <CodeBlock>{text}</CodeBlock>;
                    }
                    return (
                      <code className="rounded bg-muted/60 px-1 py-0.5 font-mono text-xs">
                        {text}
                      </code>
                    );
                  },
                }}
              >
                {message.content}
              </ReactMarkdown>
            </div>
          )}
        </div>

        {/* Actions (assistant only) */}
        {!isUser && !isError && (
          <div className="mt-1 flex items-center gap-1">
            <button
              onClick={() => {
                navigator.clipboard?.writeText(message.content).then(() => {
                  setCopiedMsg(true);
                  setTimeout(() => setCopiedMsg(false), 1500);
                });
              }}
              className="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
              aria-label="Copy message"
            >
              {copiedMsg ? (
                <Check className="h-3 w-3" />
              ) : (
                <Copy className="h-3 w-3" />
              )}
              {copiedMsg ? "Copied" : "Copy"}
            </button>
          </div>
        )}

        {/* Rich reference cards (assistant only) */}
        {!isUser && (
          <>
            {meta.intent && (
              <div className="mt-1.5">
                <span className="inline-flex items-center rounded-md bg-accent px-2 py-0.5 font-mono text-xs text-muted-foreground">
                  {String(meta.intent)}
                </span>
              </div>
            )}
            {meta.execution_id && (
              <ExecutionCard
                executionId={String(meta.execution_id)}
                agentName={agentName}
              />
            )}
            {meta.workflow_run_id && (
              <WorkflowCard runId={String(meta.workflow_run_id)} />
            )}
          </>
        )}
      </div>
    </div>
  );
}
