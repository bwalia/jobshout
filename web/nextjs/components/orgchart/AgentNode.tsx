"use client";

import { memo } from "react";
import { Handle, Position, type NodeProps } from "reactflow";
import type { Agent } from "@/lib/types/agent";
import type { AgentNodeData } from "@/lib/hooks/useOrgChart";

// ---------------------------------------------------------------------------
// Colour helpers
// ---------------------------------------------------------------------------

/**
 * Returns a deterministic Tailwind background-colour class based on the
 * agent's name.  This gives each avatar a stable, visually distinct colour
 * without needing a stored colour value.
 */
const AVATAR_COLOURS = [
  "bg-violet-500",
  "bg-blue-500",
  "bg-cyan-500",
  "bg-emerald-500",
  "bg-amber-500",
  "bg-rose-500",
  "bg-pink-500",
  "bg-indigo-500",
] as const;

function avatarColour(name: string): string {
  // Sum the char codes for a stable hash
  const hash = Array.from(name).reduce(
    (acc, char) => acc + char.charCodeAt(0),
    0
  );
  return AVATAR_COLOURS[hash % AVATAR_COLOURS.length];
}

/** Extracts up to two initials from a name string */
function initials(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length >= 2) {
    return `${parts[0][0]}${parts[1][0]}`.toUpperCase();
  }
  return name.slice(0, 2).toUpperCase();
}

// ---------------------------------------------------------------------------
// Status dot
// ---------------------------------------------------------------------------

const STATUS_DOT_COLOURS: Record<string, string> = {
  active: "bg-emerald-500",
  idle: "bg-yellow-400",
  paused: "bg-orange-500",
  offline: "bg-zinc-500",
};

interface StatusDotProps {
  status: Agent["status"];
}

function StatusDot({ status }: StatusDotProps) {
  const colour = STATUS_DOT_COLOURS[status] ?? "bg-zinc-500";
  return (
    <span
      className={`inline-block h-2.5 w-2.5 rounded-full ${colour} ring-2 ring-card`}
      aria-label={status}
      title={status}
    />
  );
}

// ---------------------------------------------------------------------------
// AgentNode component
// ---------------------------------------------------------------------------

/**
 * Custom React Flow node that represents a single agent in the org chart.
 *
 * Renders:
 * - A coloured avatar circle with the agent's initials
 * - The agent name and role
 * - A status dot indicating current agent status
 * - Source (bottom) and target (top) Handles so reporting lines can be drawn
 */
function AgentNodeComponent({ data }: NodeProps<AgentNodeData>) {
  const { agent } = data;
  const colour = avatarColour(agent.name);

  return (
    <>
      {/* Target handle — receives incoming edges from managers */}
      <Handle
        type="target"
        position={Position.Top}
        className="!h-3 !w-3 !rounded-full !border-2 !border-border !bg-background"
      />

      <div className="flex w-[220px] items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 shadow-sm transition-shadow hover:shadow-md">
        {/* Avatar */}
        <div
          className={`relative flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white ${colour}`}
        >
          {initials(agent.name)}
          {/* Status dot anchored to the bottom-right of the avatar */}
          <span className="absolute -bottom-0.5 -right-0.5">
            <StatusDot status={agent.status} />
          </span>
        </div>

        {/* Text info */}
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold leading-tight text-foreground">
            {agent.name}
          </p>
          <p className="truncate text-xs text-muted-foreground">{agent.role}</p>
        </div>
      </div>

      {/* Source handle — emits outgoing edges to direct reports */}
      <Handle
        type="source"
        position={Position.Bottom}
        className="!h-3 !w-3 !rounded-full !border-2 !border-border !bg-background"
      />
    </>
  );
}

export const AgentNode = memo(AgentNodeComponent);
