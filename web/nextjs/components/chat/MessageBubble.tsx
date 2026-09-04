"use client";

import { useState } from "react";
import { Check, Copy, RotateCcw, Sparkles } from "lucide-react";
import type { ChatMessage, ChatResponse, EntityRef } from "@/lib/types/chat";
import { ToolCallChip } from "./ToolCallChip";
import { ConfirmationCard } from "./ConfirmationCard";
import { ClarifyPrompt } from "./ClarifyPrompt";
import { EntityCardList } from "./cards/EntityCard";
import { ExecutionCard } from "./ExecutionCard";
import { WorkflowCard } from "./WorkflowCard";
import { Markdown } from "./Markdown";
import { messageTime } from "./time";

/**
 * One turn in the transcript.
 *
 * User turns stay as a compact right-aligned bubble; agent turns are
 * full-width with an avatar and no bubble fill. Agent replies routinely carry
 * tables, code and run cards, and a 85%-width tinted bubble was squeezing all
 * of it into a column too narrow to read.
 */
export function MessageBubble({
  message,
  onConfirm,
  onCancel,
  onClarify,
  onRetry,
  busy,
  isLatest,
  answeredAs,
  showAvatar = true,
}: {
  message: ChatMessage;
  onConfirm?: (token: string) => void;
  onCancel?: () => void;
  onClarify?: (value: string, label: string) => void;
  onRetry?: () => void;
  busy?: boolean;
  isLatest?: boolean;
  answeredAs?: string;
  showAvatar?: boolean;
}) {
  const mine = message.role === "user";
  const envelope = envelopeOf(message);
  const meta = message.metadata ?? {};
  const fromEntities = idsFromEntities(envelope?.entities);
  const executionId = stringOrNull(meta.execution_id) ?? fromEntities.executionId;
  const workflowRunId = stringOrNull(meta.workflow_run_id) ?? fromEntities.workflowRunId;
  const agentId = stringOrNull(meta.agent_id) ?? fromEntities.agentId;
  const agentName = stringOrNull(meta.agent_name) ?? fromEntities.agentName;
  const usage = envelope?.usage;

  if (message.role === "tool" || message.role === "system") {
    return null;
  }

  if (mine) {
    return (
      <div className="group/msg flex animate-chat-message-in justify-end">
        <div className="flex min-w-0 max-w-[85%] flex-col items-end gap-1">
          <div className="min-w-0 rounded-2xl rounded-br-md bg-secondary px-4 py-3 text-[15px] text-foreground">
            <p className="whitespace-pre-wrap break-words leading-7">{message.content}</p>
          </div>
          <div className="flex items-center gap-1.5 pr-1 opacity-0 transition-opacity duration-200 group-hover/msg:opacity-100 focus-within:opacity-100">
            <CopyButton text={message.content} />
            <time className="tabular text-xs text-muted-foreground">
              {messageTime(message.created_at)}
            </time>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="group/msg flex animate-chat-message-in gap-3">
      <div className="w-8 shrink-0">
        {showAvatar ? (
          <span className="flex h-8 w-8 items-center justify-center rounded-full border border-border bg-card">
            <Sparkles className="h-4 w-4 text-primary" />
          </span>
        ) : null}
      </div>

      <div className="flex min-w-0 max-w-[92ch] flex-1 flex-col">
        {showAvatar ? (
          <div className="mb-1 flex items-baseline gap-2">
            <span className="text-sm font-semibold text-foreground">
              {agentName ?? "JobShout"}
            </span>
            <time className="tabular text-xs text-muted-foreground">
              {messageTime(message.created_at)}
            </time>
          </div>
        ) : null}

        {message.content ? (
          <Markdown>{message.content}</Markdown>
        ) : (
          <p className="text-[15px] italic text-muted-foreground">No reply text.</p>
        )}

        {envelope?.actions?.length ? (
          <div className="mt-2 space-y-1">
            {envelope.actions.map((a, i) => (
              <ToolCallChip key={`${a.tool}-${i}`} action={a} />
            ))}
          </div>
        ) : null}

        {envelope ? <EntityCardList entities={envelope.entities ?? []} /> : null}

        {executionId ? (
          <ExecutionCard
            executionId={executionId}
            agentId={agentId ?? undefined}
            agentName={agentName ?? undefined}
          />
        ) : null}

        {workflowRunId ? <WorkflowCard runId={workflowRunId} /> : null}

        {envelope?.confirmation && onConfirm ? (
          <ConfirmationCard
            confirmation={envelope.confirmation}
            busy={busy}
            live={Boolean(isLatest)}
            answeredAs={answeredAs}
            onApprove={() => onConfirm(envelope.confirmation!.token)}
            onCancel={() => onCancel?.()}
          />
        ) : null}

        {envelope?.clarify && onClarify ? (
          <ClarifyPrompt
            clarify={envelope.clarify}
            live={Boolean(isLatest)}
            answeredAs={answeredAs}
            onPick={onClarify}
          />
        ) : null}

        {/* Actions and cost sit below the reply, revealed on hover so the
            transcript stays quiet but the numbers are one glance away. */}
        <div className="mt-1.5 flex items-center gap-1.5 opacity-0 transition-opacity duration-200 group-hover/msg:opacity-100 focus-within:opacity-100">
          <CopyButton text={message.content} />
          {isLatest && onRetry ? (
            <button
              type="button"
              onClick={onRetry}
              disabled={busy}
              className="flex h-8 items-center gap-1.5 rounded-md px-2 text-xs text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-40"
            >
              <RotateCcw className="h-3.5 w-3.5" />
              Retry
            </button>
          ) : null}
          {usage ? (
            <span className="tabular ml-1 text-xs text-muted-foreground/80">
              {usage.model ? `${usage.model} · ` : ""}
              {usage.input_tokens + usage.output_tokens} tok
              {typeof usage.cost_usd === "number"
                ? ` · $${usage.cost_usd.toFixed(4)}`
                : ""}
            </span>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  if (!text) return null;
  return (
    <button
      type="button"
      aria-label={copied ? "Copied to clipboard" : "Copy message"}
      onClick={() => {
        navigator.clipboard
          .writeText(text)
          .then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1600);
          })
          .catch(() => undefined);
      }}
      className="flex h-8 items-center gap-1.5 rounded-md px-2 text-xs text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      {copied ? (
        <Check className="h-3.5 w-3.5 text-signal-live" />
      ) : (
        <Copy className="h-3.5 w-3.5" />
      )}
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

function envelopeOf(message: ChatMessage): ChatResponse | null {
  const raw = message.metadata?.response;
  if (!raw || typeof raw !== "object") return null;
  return raw as ChatResponse;
}

function stringOrNull(v: unknown): string | null {
  return typeof v === "string" && v ? v : null;
}

function idsFromEntities(entities: EntityRef[] | undefined): {
  executionId: string | null;
  workflowRunId: string | null;
  agentId: string | null;
  agentName: string | null;
} {
  const out = {
    executionId: null as string | null,
    workflowRunId: null as string | null,
    agentId: null as string | null,
    agentName: null as string | null,
  };
  for (const e of entities ?? []) {
    if (e.kind === "execution" && e.id) {
      out.executionId = e.id;
      if (e.label && e.label !== "execution") out.agentName = e.label;
    }
    if (e.kind === "workflow_run" && e.id) out.workflowRunId = e.id;
    if (e.kind === "agent" && e.id) {
      out.agentId = e.id;
      if (e.label) out.agentName = e.label;
    }
  }
  return out;
}
