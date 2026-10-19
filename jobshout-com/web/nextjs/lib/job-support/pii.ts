/**
 * Strip identity before anything reaches NebulaDB.
 * Names, emails, phones, URLs with tokens, and street addresses must never
 * be embedded — OpsAPI CRM holds that; NebulaDB only sees skills/role text.
 */

const EMAIL_RE = /[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/gi;
const PHONE_RE =
  /(?:\+|00)?[\d\s().-]{7,}\d/g;
const LINKEDIN_RE = /https?:\/\/(?:www\.)?linkedin\.com\/\S+/gi;
const URL_RE = /https?:\/\/\S+/gi;

/** Best-effort redaction for free text headed to NebulaDB. */
export function redactPii(text: string): string {
  return text
    .replace(EMAIL_RE, "[email]")
    .replace(LINKEDIN_RE, "[profile]")
    .replace(URL_RE, "[link]")
    .replace(PHONE_RE, "[phone]")
    .replace(/\b(?:mr|mrs|ms|dr)\.?\s+[A-Z][a-z]+(?:\s+[A-Z][a-z]+)?\b/g, "[name]")
    .trim();
}

export function extractEmail(text: string): string | null {
  const m = text.match(EMAIL_RE);
  return m?.[0]?.toLowerCase() ?? null;
}

export function extractPhone(text: string): string | null {
  const m = text.match(PHONE_RE);
  if (!m) return null;
  const digits = m[0].replace(/\D/g, "");
  return digits.length >= 7 ? m[0].trim() : null;
}
