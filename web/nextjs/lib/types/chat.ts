// Types for the chat control-plane UI. These mirror the EXISTING JobShout chat
// backend (server/internal/model/chat.go + ChatRouterService) — the UI is a
// shell over that engine, not a new data model.

export type ChatRole = "user" | "agent" | "system";

/** Metadata the router attaches to an assistant turn, referencing the real work. */
export interface ChatMessageMetadata {
  intent?: string;
  confidence?: number;
  agent_id?: string;
  execution_id?: string;
  workflow_run_id?: string;
  error?: boolean;
  [key: string]: unknown;
}

export interface ChatMessage {
  id: string;
  session_id: string;
  org_id: string;
  role: ChatRole;
  source: string;
  content: string;
  metadata: ChatMessageMetadata;
  created_at: string;
}

export interface ChatSession {
  id: string;
  org_id: string;
  user_id: string;
  agent_id: string | null;
  source: string;
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface StartChatSessionRequest {
  agent_id?: string;
  source?: string;
}

export interface SendChatMessageRequest {
  content: string;
  source?: string;
}

/** Response of POST /chat/sessions/{id}/messages — the turn's two messages. */
export interface SendChatMessageResponse {
  user_message: ChatMessage;
  agent_message: ChatMessage;
}
