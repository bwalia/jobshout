import type { AgentStatus } from "@/lib/types/common";
import { SignalDot, type SignalStatus } from "@/components/ui/signal-dot";
import { cn } from "@/lib/utils/cn";

interface AgentStatusBadgeProps {
  status: AgentStatus;
}

/** Maps each agent status to its label, signal motif and token-based colours. */
const STATUS_CONFIG: Record<
  AgentStatus,
  { label: string; signal: SignalStatus; text: string; bg: string }
> = {
  active: {
    label: "Live",
    signal: "live",
    text: "text-signal-live",
    bg: "bg-signal-live/10",
  },
  idle: {
    label: "Idle",
    signal: "idle",
    text: "text-muted-foreground",
    bg: "bg-muted",
  },
  paused: {
    label: "Paused",
    signal: "attention",
    text: "text-signal",
    bg: "bg-signal/10",
  },
  offline: {
    label: "Offline",
    signal: "error",
    text: "text-signal-error",
    bg: "bg-signal-error/10",
  },
};

/**
 * A pill that communicates an agent's status. Live agents emit the broadcast
 * pulse (the Signal Room signature) via SignalDot.
 */
export function AgentStatusBadge({ status }: AgentStatusBadgeProps) {
  const c = STATUS_CONFIG[status] ?? STATUS_CONFIG.offline;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium",
        c.bg,
        c.text
      )}
    >
      <SignalDot status={c.signal} size="sm" />
      {c.label}
    </span>
  );
}
