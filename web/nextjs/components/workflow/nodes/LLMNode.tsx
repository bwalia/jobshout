"use client";

import { Handle, Position, type NodeProps } from "reactflow";
import { cn } from "@/lib/utils/cn";

export function LLMNode({ data }: NodeProps) {
  const status = data.status as string | undefined;

  return (
    <div
      className={cn(
        "rounded-lg border-2 bg-card px-4 py-3 shadow-md min-w-[160px]",
        status === "completed" && "border-green-500",
        status === "running" && "border-yellow-500 animate-pulse",
        status === "failed" && "border-red-500",
        !status && "border-indigo-500"
      )}
    >
      <Handle type="target" position={Position.Top} className="!bg-indigo-500" />
      <div className="flex items-center gap-2">
        <div className="h-6 w-6 rounded bg-indigo-500/20 flex items-center justify-center text-xs">
          LLM
        </div>
        <span className="text-sm font-medium text-foreground">{data.label}</span>
      </div>
      {status && (
        <p className="mt-1 text-xs text-muted-foreground capitalize">{status}</p>
      )}
      <Handle type="source" position={Position.Bottom} className="!bg-indigo-500" />
    </div>
  );
}
