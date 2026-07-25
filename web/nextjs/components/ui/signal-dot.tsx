import { cn } from "@/lib/utils/cn";

/**
 * SignalDot — the signature "broadcast" motif of the Signal Room identity.
 *
 * Every agent, everywhere, wears one: a status dot that emits a concentric
 * ripple while live/broadcasting and sits steady when idle. One repeated
 * motif, quiet everywhere else.
 */
export type SignalStatus =
  | "live" // running / broadcasting  → green pulse
  | "attention" // awaiting approval / needs a human → amber pulse
  | "error" // failed
  | "done" // completed
  | "idle" // idle / offline
  | "queued"; // queued / to-do

const STATUS: Record<SignalStatus, { color: string; pulse: boolean }> = {
  live: { color: "bg-signal-live", pulse: true },
  attention: { color: "bg-signal", pulse: true },
  error: { color: "bg-signal-error", pulse: false },
  done: { color: "bg-status-done", pulse: false },
  idle: { color: "bg-status-idle", pulse: false },
  queued: { color: "bg-status-todo", pulse: false },
};

const SIZE = {
  sm: "h-1.5 w-1.5",
  md: "h-2 w-2",
  lg: "h-2.5 w-2.5",
} as const;

export function SignalDot({
  status = "idle",
  size = "md",
  className,
}: {
  status?: SignalStatus;
  size?: keyof typeof SIZE;
  className?: string;
}) {
  const s = STATUS[status] ?? STATUS.idle;
  const px = SIZE[size];

  return (
    <span
      className={cn("relative inline-flex", px, className)}
      role="status"
      aria-label={status}
    >
      {s.pulse && (
        <span
          aria-hidden
          className={cn(
            "absolute inline-flex h-full w-full rounded-full animate-signal-ping",
            s.color
          )}
        />
      )}
      <span
        className={cn(
          "relative inline-flex rounded-full",
          px,
          s.color,
          s.pulse && "animate-signal-breathe"
        )}
      />
    </span>
  );
}
