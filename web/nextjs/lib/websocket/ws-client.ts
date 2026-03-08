// WebSocket client with automatic reconnection using exponential backoff.
// Emits typed events to registered callbacks.

const WS_BASE_URL =
  process.env.NEXT_PUBLIC_WS_URL ?? "ws://localhost:8080";

// Reconnect timing constraints (milliseconds)
const RECONNECT_BASE_DELAY_MS = 1_000;
const RECONNECT_MAX_DELAY_MS = 30_000;

/** Shape of every message sent over the WebSocket wire */
export interface WSMessage<TPayload = unknown> {
  type: string;
  payload: TPayload;
  timestamp: string;
}

/** Internal event names managed by the client itself */
type ClientEvent = "open" | "close" | "error" | "message";

type EventCallback<T = unknown> = (data: T) => void;

export class WSClient {
  private readonly accessToken: string;
  private socket: WebSocket | null = null;
  private reconnectAttempt = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  /** Flag set to true when the caller explicitly calls destroy() */
  private destroyed = false;

  // Separate listener maps for built-in lifecycle events and typed message events
  private readonly lifecycleListeners = new Map<ClientEvent, Set<EventCallback>>([
      ["open", new Set<EventCallback>()],
      ["close", new Set<EventCallback>()],
      ["error", new Set<EventCallback>()],
      ["message", new Set<EventCallback>()],
    ]);

  /** Listeners keyed by the message `type` field for targeted subscriptions */
  private readonly messageListeners: Map<string, Set<EventCallback>> =
    new Map();

  constructor(accessToken: string) {
    this.accessToken = accessToken;
    this.connect();
  }

  // ---------------------------------------------------------------------------
  // Public API
  // ---------------------------------------------------------------------------

  /**
   * Register a callback for a lifecycle event or a typed WebSocket message.
   * Returns an `off` function for convenient cleanup.
   */
  on<T = unknown>(event: string, callback: EventCallback<T>): () => void {
    const cb = callback as EventCallback;

    if (this.isLifecycleEvent(event)) {
      this.lifecycleListeners.get(event as ClientEvent)?.add(cb);
    } else {
      if (!this.messageListeners.has(event)) {
        this.messageListeners.set(event, new Set());
      }
      this.messageListeners.get(event)!.add(cb);
    }

    return () => this.off(event, callback);
  }

  /** Remove a previously registered callback */
  off<T = unknown>(event: string, callback: EventCallback<T>): void {
    const cb = callback as EventCallback;

    if (this.isLifecycleEvent(event)) {
      this.lifecycleListeners.get(event as ClientEvent)?.delete(cb);
    } else {
      this.messageListeners.get(event)?.delete(cb);
    }
  }

  /** Returns true when the underlying WebSocket is in the OPEN ready state */
  get isConnected(): boolean {
    return this.socket?.readyState === WebSocket.OPEN;
  }

  /**
   * Permanently close the connection and stop any reconnection attempts.
   * Call this in component cleanup (useEffect return / componentWillUnmount).
   */
  destroy(): void {
    this.destroyed = true;
    this.clearReconnectTimer();
    this.socket?.close();
    this.socket = null;
    this.lifecycleListeners.forEach((set) => set.clear());
    this.messageListeners.clear();
  }

  // ---------------------------------------------------------------------------
  // Private helpers
  // ---------------------------------------------------------------------------

  private connect(): void {
    if (this.destroyed) return;

    const url = `${WS_BASE_URL}/api/v1/ws?token=${encodeURIComponent(this.accessToken)}`;

    try {
      this.socket = new WebSocket(url);
    } catch (err) {
      // new WebSocket() can throw synchronously for invalid URLs
      this.emit("error", err);
      this.scheduleReconnect();
      return;
    }

    this.socket.onopen = () => {
      this.reconnectAttempt = 0;
      this.emit("open", undefined);
    };

    this.socket.onclose = (event: CloseEvent) => {
      this.emit("close", event);
      // Do not reconnect on clean, intentional closure (code 1000)
      if (!this.destroyed && event.code !== 1000) {
        this.scheduleReconnect();
      }
    };

    this.socket.onerror = (event: Event) => {
      this.emit("error", event);
    };

    this.socket.onmessage = (event: MessageEvent<string>) => {
      this.emit("message", event.data);

      // Attempt to parse JSON; silently skip malformed messages
      try {
        const parsed = JSON.parse(event.data) as WSMessage;

        if (typeof parsed.type === "string") {
          // Dispatch to type-specific listeners
          this.messageListeners.get(parsed.type)?.forEach((cb) => cb(parsed));
        }
      } catch {
        // Intentionally silent: non-JSON frames are forwarded as raw "message" events only
      }
    };
  }

  private scheduleReconnect(): void {
    if (this.destroyed) return;

    // Exponential backoff: delay doubles with each attempt, capped at RECONNECT_MAX_DELAY_MS
    const delay = Math.min(
      RECONNECT_BASE_DELAY_MS * 2 ** this.reconnectAttempt,
      RECONNECT_MAX_DELAY_MS
    );
    this.reconnectAttempt += 1;

    this.reconnectTimer = setTimeout(() => {
      this.connect();
    }, delay);
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private emit(event: ClientEvent, data: unknown): void {
    this.lifecycleListeners.get(event)?.forEach((cb) => cb(data));
  }

  private isLifecycleEvent(event: string): event is ClientEvent {
    return ["open", "close", "error", "message"].includes(event);
  }
}
