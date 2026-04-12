"use client";

import { Handle, Position, type NodeProps } from "reactflow";
import { cn } from "@/lib/utils/cn";

export function DecisionNode({ data }: NodeProps) {
  const status = data.status as string | undefined;

  return (
    <div
      className={cn(
        "rounded-lg border-2 bg-card px-4 py-3 shadow-md min-w-[160px]",
        status === "completed" && "border-green-500",
        status === "running" && "border-yellow-500 animate-pulse",
        status === "failed" && "border-red-500",
        !status && "border-pink-500"
      )}
    >
      <Handle type="target" position={Position.Top} className="!bg-pink-500" />
      <div className="flex items-center gap-2">
        <div className="h-6 w-6 rounded bg-pink-500/20 flex items-center justify-center text-xs rotate-45">
          ?
        </div>
        <span className="text-sm font-medium text-foreground">{data.label}</span>
      </div>
      {status && (
        <p className="mt-1 text-xs text-muted-foreground capitalize">{status}</p>
      )}
      <Handle type="source" position={Position.Bottom} id="yes" className="!bg-green-500 !left-1/3" />
      <Handle type="source" position={Position.Bottom} id="no" className="!bg-red-500 !left-2/3" />
    </div>
  );
}
