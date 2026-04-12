"use client";

import { Handle, Position, type NodeProps } from "reactflow";
import { cn } from "@/lib/utils/cn";

export function AgentNode({ data }: NodeProps) {
  const status = data.status as string | undefined;

  return (
    <div
      className={cn(
        "rounded-lg border-2 bg-card px-4 py-3 shadow-md min-w-[160px]",
        status === "completed" && "border-green-500",
        status === "running" && "border-yellow-500 animate-pulse",
        status === "failed" && "border-red-500",
        !status && "border-cyan-500"
      )}
    >
      <Handle type="target" position={Position.Top} className="!bg-cyan-500" />
      <div className="flex items-center gap-2">
        <div className="h-6 w-6 rounded bg-cyan-500/20 flex items-center justify-center text-xs">
          A
        </div>
        <span className="text-sm font-medium text-foreground">{data.label}</span>
      </div>
      {status && (
        <p className="mt-1 text-xs text-muted-foreground capitalize">{status}</p>
      )}
      <Handle type="source" position={Position.Bottom} className="!bg-cyan-500" />
    </div>
  );
}
