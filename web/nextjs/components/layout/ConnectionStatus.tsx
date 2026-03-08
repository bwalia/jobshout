"use client";

import { useWebSocket } from "@/lib/hooks/useWebSocket";

type ConnectionState = "connected" | "connecting" | "disconnected";

interface DotStyleConfig {
  outer: string;
  inner: string;
  label: string;
}

const DOT_STYLES: Record<ConnectionState, DotStyleConfig> = {
  connected: {
    outer: "bg-green-500/20",
    inner: "bg-green-500",
    label: "Connected",
  },
  connecting: {
    outer: "bg-yellow-500/20",
    inner: "bg-yellow-500 animate-pulse",
    label: "Connecting",
  },
  disconnected: {
    outer: "bg-red-500/20",
    inner: "bg-red-500",
    label: "Disconnected",
  },
};

export function ConnectionStatus() {
  const { connected } = useWebSocket();

  // Treat a falsy `connected` value as disconnected for display purposes.
  // A "connecting" state would be managed by a more complex state machine;
  // for the topbar indicator two states (connected / disconnected) are sufficient.
  const state: ConnectionState = connected ? "connected" : "disconnected";
  const styles = DOT_STYLES[state];

  return (
    <div
      className="flex items-center gap-2"
      title={styles.label}
      aria-label={`WebSocket status: ${styles.label}`}
    >
      {/* Outer glow ring */}
      <span className={`relative flex h-3 w-3 items-center justify-center rounded-full ${styles.outer}`}>
        {/* Inner filled dot */}
        <span className={`h-2 w-2 rounded-full ${styles.inner}`} />
      </span>

      {/* Label - hidden on small screens to save topbar space */}
      <span className="hidden text-xs text-muted-foreground sm:inline">
        {styles.label}
      </span>
    </div>
  );
}
