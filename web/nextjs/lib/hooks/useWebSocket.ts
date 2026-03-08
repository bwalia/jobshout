"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { WSClient, type WSMessage } from "@/lib/websocket/ws-client";
import { getAccessToken } from "@/lib/auth/auth";

interface UseWebSocketResult {
  /** Whether the WebSocket connection is currently open */
  connected: boolean;
  /**
   * Subscribe to a typed message event.
   * Returns an unsubscribe function - call it in cleanup to prevent memory leaks.
   *
   * @example
   * useEffect(() => {
   *   return subscribe("agent.status_changed", (msg) => {
   *     console.log(msg.payload);
   *   });
   * }, [subscribe]);
   */
  subscribe: <TPayload = unknown>(
    eventType: string,
    callback: (message: WSMessage<TPayload>) => void
  ) => () => void;
}

export function useWebSocket(): UseWebSocketResult {
  const [connected, setConnected] = useState(false);
  // Store the WSClient instance in a ref so it persists across renders without
  // triggering re-renders when updated.
  const clientRef = useRef<WSClient | null>(null);

  useEffect(() => {
    const accessToken = getAccessToken();

    // Do not open a connection when there is no access token (e.g. logged-out state)
    if (!accessToken) return;

    const client = new WSClient(accessToken);
    clientRef.current = client;

    const removeOpenListener = client.on("open", () => setConnected(true));
    const removeCloseListener = client.on("close", () => setConnected(false));
    const removeErrorListener = client.on("error", () => setConnected(false));

    return () => {
      // Remove lifecycle listeners to avoid calling setConnected after unmount
      removeOpenListener();
      removeCloseListener();
      removeErrorListener();
      client.destroy();
      clientRef.current = null;
    };
  }, []);

  /**
   * Subscribe to a specific WebSocket message type.
   * Stable across renders thanks to useCallback with no deps -
   * the WSClient reference is accessed through the ref.
   */
  const subscribe = useCallback(
    <TPayload = unknown>(
      eventType: string,
      callback: (message: WSMessage<TPayload>) => void
    ): (() => void) => {
      const client = clientRef.current;

      if (!client) {
        // Return a no-op unsubscribe if the client is not yet initialised
        return () => undefined;
      }

      return client.on<WSMessage<TPayload>>(eventType, callback);
    },
    []
  );

  return { connected, subscribe };
}
