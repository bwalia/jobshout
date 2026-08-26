"use client";

import { cn } from "@/lib/utils/cn";
import type { ChatMessage, ChatResponse } from "@/lib/types/chat";
import { ToolCallChip } from "./ToolCallChip";
import { ConfirmationCard } from "./ConfirmationCard";
import { ClarifyPrompt } from "./ClarifyPrompt";
import { EntityCardList } from "./cards/EntityCard";
import { ExecutionCard } from "./ExecutionCard";
import { WorkflowCard } from "./WorkflowCard";

export function MessageBubble({
  message,
  onConfirm,
  onCancel,
  onClarify,
  busy,
}: {
  message: ChatMessage;
  onConfirm?: (token: string) => void;
  onCancel?: () => void;
  onClarify?: (value: string) => void;
  busy?: boolean;
}) {
  const mine = message.role === "user";
  const envelope = envelopeOf(message);
  const meta = message.metadata ?? {};
  const executionId = stringOrNull(meta.execution_id);
  const workflowRunId = stringOrNull(meta.workflow_run_id);
  const agentId = stringOrNull(meta.agent_id);
  const agentName = stringOrNull(meta.agent_name);

  if (message.role === "tool" || message.role === "system") {
    return null;
  }

  return (
    <div className={cn("flex", mine ? "justify-end" : "justify-start")}>
      <div
        className={cn(
          "max-w-[85%] rounded-2xl px-3.5 py-2.5 text-sm",
          mine
            ? "bg-primary text-primary-foreground"
            : "bg-secondary text-foreground"
        )}
      >
        <p className="whitespace-pre-wrap leading-relaxed">{message.content}</p>
        {!mine && (
          <div className="text-foreground">
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
                onApprove={() => onConfirm(envelope.confirmation!.token)}
                onCancel={() => onCancel?.()}
              />
            ) : null}
            {envelope?.clarify && onClarify ? (
              <ClarifyPrompt clarify={envelope.clarify} onPick={onClarify} />
            ) : null}
          </div>
        )}
      </div>
    </div>
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
