export interface EntityRef {
  kind: string;
  id: string;
  label: string;
  href?: string;
  /** Fetchable picture for kind=image (stored path or data URL). */
  url?: string;
}

export interface ActionRecord {
  tool: string;
  args?: Record<string, unknown>;
  status: "ok" | "failed" | "denied" | "pending_confirmation" | string;
  result_ref?: EntityRef;
  error?: string;
  duration_ms: number;
}

export interface ConfirmRequest {
  token: string;
  tool: string;
  summary: string;
  effect: string;
  expires_at?: string;
}

export interface ClarifyOption {
  label: string;
  value: string;
}

export interface ClarifyRequest {
  question: string;
  slot?: string;
  options?: ClarifyOption[];
}

export interface UsageInfo {
  model?: string;
  input_tokens: number;
  output_tokens: number;
  latency_ms: number;
  cost_usd?: number;
}

export interface ChatResponse {
  message: string;
  actions: ActionRecord[];
  entities: EntityRef[];
  confirmation?: ConfirmRequest;
  clarify?: ClarifyRequest;
  usage?: UsageInfo;
}

export interface ChatSession {
  id: string;
  org_id: string;
  user_id: string;
  agent_id?: string;
  source: string;
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface ChatMessage {
  id: string;
  session_id: string;
  org_id: string;
  role: "user" | "agent" | "system" | "tool";
  source: string;
  content: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface ChatTurnResult {
  user_message: ChatMessage;
  agent_message: ChatMessage;
  response: ChatResponse;
}

export interface ChatStreamEvent {
  type:
    | "token"
    | "tool_call"
    | "tool_result"
    | "confirmation"
    | "clarify"
    | "done"
    | "error"
    | "model";
  token?: string;
  tool?: string;
  label?: string;
  args?: Record<string, unknown>;
  status?: string;
  duration_ms?: number;
  entity?: EntityRef;
  confirmation?: ConfirmRequest;
  clarify?: ClarifyRequest;
  response?: ChatResponse;
  error?: string;
  /** Model serving this turn — sent up front, and again on a mid-turn fallback. */
  model?: string;
  provider?: string;
}

export function sessionTitle(session: ChatSession): string {
  const t = session.metadata?.title;
  if (typeof t === "string" && t.trim()) return t;
  return "New chat";
}
