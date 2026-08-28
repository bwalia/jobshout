import { apiClient } from "@/lib/api/client";
import type { MailConnectionStatus } from "@/types/mail";

function splitComma(s: string): string[] {
  return s
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
}

function splitLines(s: string): string[] {
  return s
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
}

/** Form keys for the Mail Agent Task Manager schema. */
export const MAIL_FORM_KEYS = [
  "senders",
  "subject_prefixes",
  "labels",
  "knowledge_urls",
  "research_focus",
  "reply_instructions",
] as const;

export function mailFormValuesFromStatus(
  st: MailConnectionStatus
): Record<string, string> {
  return {
    senders: (st.rules?.senders ?? []).join(", "),
    subject_prefixes: (st.rules?.subject_prefixes ?? []).join(", "),
    labels: (st.rules?.labels ?? []).join(", "),
    knowledge_urls: (st.knowledge_urls ?? []).join("\n"),
    research_focus: st.research_focus ?? "",
    reply_instructions: st.reply_instructions ?? "",
  };
}

export function mailPatchFromFormValues(values: Record<string, string>) {
  return {
    rules: {
      senders: splitComma(values.senders ?? ""),
      labels: splitComma(values.labels ?? ""),
      subject_prefixes: splitComma(values.subject_prefixes ?? ""),
    },
    knowledge_urls: splitLines(values.knowledge_urls ?? ""),
    research_focus: (values.research_focus ?? "").trim(),
    reply_instructions: (values.reply_instructions ?? "").trim(),
  };
}

/** Load saved mailbox playbook so Task Manager does not wipe existing rules. */
export async function fetchMailFormValues(): Promise<Record<string, string> | null> {
  try {
    const { data } = await apiClient.get<MailConnectionStatus>("/mail/connection");
    return mailFormValuesFromStatus(data);
  } catch {
    return null;
  }
}
