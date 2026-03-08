import type { AgentStatus } from "@/lib/types/common";

interface AgentStatusBadgeProps {
  status: AgentStatus;
}

/** Maps each status to its display label and Tailwind colour tokens. */
const STATUS_CONFIG: Record<
  AgentStatus,
  { label: string; dotClass: string; textClass: string; bgClass: string }
> = {
  idle: {
    label: "Idle",
    dotClass: "bg-slate-400",
    textClass: "text-slate-400",
    bgClass: "bg-slate-400/10",
  },
  active: {
    label: "Active",
    dotClass: "bg-emerald-500",
    textClass: "text-emerald-500",
    bgClass: "bg-emerald-500/10",
  },
  paused: {
    label: "Paused",
    dotClass: "bg-amber-500",
    textClass: "text-amber-500",
    bgClass: "bg-amber-500/10",
  },
  offline: {
    label: "Offline",
    dotClass: "bg-red-500",
    textClass: "text-red-500",
    bgClass: "bg-red-500/10",
  },
};

/**
 * A small pill badge that communicates an agent's current status.
 * Active agents render a pulsing dot to convey real-time activity.
 */
export function AgentStatusBadge({ status }: AgentStatusBadgeProps) {
  const config = STATUS_CONFIG[status] ?? STATUS_CONFIG.offline;

  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium ${config.bgClass} ${config.textClass}`}
    >
      <span className="relative flex h-2 w-2 flex-shrink-0">
        {/* Pulse ring – shown only for the "active" status */}
        {status === "active" && (
          <span
            className={`absolute inline-flex h-full w-full animate-ping rounded-full opacity-75 ${config.dotClass}`}
            aria-hidden="true"
          />
        )}
        <span
          className={`relative inline-flex h-2 w-2 rounded-full ${config.dotClass}`}
        />
      </span>
      {config.label}
    </span>
  );
}
