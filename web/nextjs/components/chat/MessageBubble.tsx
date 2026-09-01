"use client";

import { cn } from "@/lib/utils/cn";
import type { ChatMessage, ChatResponse, EntityRef } from "@/lib/types/chat";
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
  isLatest,
  answeredAs,
}: {
  message: ChatMessage;
  onConfirm?: (token: string) => void;
  onCancel?: () => void;
  onClarify?: (value: string, label: string) => void;
  busy?: boolean;
  isLatest?: boolean;
  answeredAs?: string;
}) {
  const mine = message.role === "user";
  const envelope = envelopeOf(message);
  const meta = message.metadata ?? {};
  const fromEntities = idsFromEntities(envelope?.entities);
  const executionId = stringOrNull(meta.execution_id) ?? fromEntities.executionId;
  const workflowRunId =
    stringOrNull(meta.workflow_run_id) ?? fromEntities.workflowRunId;
  const agentId = stringOrNull(meta.agent_id) ?? fromEntities.agentId;
  const agentName = stringOrNull(meta.agent_name) ?? fromEntities.agentName;

  if (message.role === "tool" || message.role === "system") {
    return null;
  }

  return (
    <div className={cn("flex", mine ? "justify-end" : "justify-start")}>
      <div
        className={cn(
          "max-w-[85%] min-w-0 rounded-2xl px-3.5 py-2.5 text-sm",
          mine
            ? "bg-primary text-primary-foreground"
            : "bg-secondary text-foreground"
        )}
      >
        <p className="break-words whitespace-pre-wrap leading-relaxed">
          {message.content}
        </p>
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
