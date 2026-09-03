"use client";

import { Trash2 } from "lucide-react";
import { useDeleteAgent } from "@/lib/hooks/useAgents";
import type { Agent } from "@/lib/types/agent";

export function isSeededAgent(agent: Agent): boolean {
  return Boolean(agent.metadata?.builtin);
}

export function RemoveAgentButton({
  agent,
  onRemoved,
}: {
  agent: Agent;
  onRemoved?: () => void;
}) {
  const remove = useDeleteAgent();

  if (isSeededAgent(agent)) {
    return null;
  }

  return (
    <button
      type="button"
      onClick={() => {
        if (
          !confirm(
            `Remove “${agent.name}”? This deletes the agent. It cannot be undone.`,
          )
        ) {
          return;
        }
        remove.mutate(agent.id, { onSuccess: () => onRemoved?.() });
      }}
      disabled={remove.isPending}
      className="inline-flex h-9 items-center gap-1.5 rounded-md border border-border px-3 text-sm text-destructive hover:bg-destructive/10 disabled:opacity-50"
    >
      <Trash2 className="h-4 w-4" />
      {remove.isPending ? "Removing…" : "Remove"}
    </button>
  );
}
