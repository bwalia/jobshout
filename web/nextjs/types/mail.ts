export type MailThreadStatus =
  | "new"
  | "classifying"
  | "researching"
  | "draft_ready"
  | "sent"
  | "rejected"
  | "ignored"
  | "failed";

export type MailDraftStatus = "draft" | "approved" | "sent" | "rejected";

export interface MailWatchRules {
  labels: string[];
  senders: string[];
  subject_prefixes: string[];
}

export interface MailScopeDoc {
  scope: string;
  why: string;
}

export interface MailConnectionStatus {
  configured: boolean;
  connected: boolean;
  email?: string;
  status: string;
  status_error?: string;
  allow_mailbox_mutations: boolean;
  rules: MailWatchRules;
  scopes?: string[];
  scopes_documented: MailScopeDoc[];
  last_sync_at?: string | null;
  connected_at?: string | null;
  agent_id?: string | null;
}

export interface MailClassification {
  intent: string;
  needs_research: boolean;
  urgency: string;
  suggested_action: string;
  reason: string;
  triage_label: string;
}

export interface MailThread {
  id: string;
  org_id: string;
  status: MailThreadStatus;
  from_email: string;
  from_name: string;
  to_email: string;
  subject: string;
  snippet: string;
  body_text: string;
  classification?: MailClassification | null;
  needs_research: boolean;
  research_summary?: string | null;
  research_brief_id?: string | null;
  error_message?: string | null;
  received_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface MailDraft {
  id: string;
  thread_id: string;
  status: MailDraftStatus;
  subject: string;
  body: string;
  to_email: string;
  cc_email?: string;
  research_brief_id?: string | null;
  approved_by?: string | null;
  approved_at?: string | null;
  gmail_message_id?: string | null;
  sent_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface MailThreadDetail {
  thread: MailThread;
  draft?: MailDraft | null;
}

export interface PaginatedMailThreads {
  data: MailThread[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}
