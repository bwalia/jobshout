/**
 * The state every form action returns. Lives outside the "use server" files:
 * those may only export async functions, and Next.js throws on the first
 * action call if one exports a value (every job post, application and
 * profile save failed that way until this moved here).
 */
export type FormState = {
  ok: boolean;
  message?: string;
  fieldErrors?: Record<string, string>;
  /** Set on success so the client can route on or show a receipt. */
  result?: { id?: string; jobId?: string; profileId?: string; status?: string };
};

export const EMPTY_FORM_STATE: FormState = { ok: false };
