/**
 * Recruiting agent — recruiter briefs + candidate CVs, matched in NebulaDB.
 * When session.jobId is set, the turn is a role-aware apply flow for that job.
 *
 * Privacy rules (enforced here, not in the prompt alone):
 * - NebulaDB receives redacted skills/role text only (see pii.ts).
 * - Identity (name/email/phone) goes to OpsAPI CRM only.
 * - LLM calls go through cluster NebulaDB → Ollama, never public model APIs.
 */

import { applyToJob, getJob, listJobs } from "@/lib/api";
import { crmEnabled, upsertJobSupportLead } from "./crm";
import type { JobSupportEvent, MatchItem } from "./events";
import {
  CANDIDATES_BUCKET,
  JOBS_BUCKET,
  chat,
  indexJob,
  nebuladbEnabled,
  semanticSearch,
  upsertDoc,
} from "./nebuladb";
import { extractEmail, extractPhone, redactPii } from "./pii";
import type { Role, SessionState } from "./types";

export type { Role, SessionState };

export type TurnInput = {
  session: SessionState;
  message: string;
  history: { role: "user" | "assistant"; content: string }[];
  emit: (e: JobSupportEvent) => void;
};

const SYSTEM = `You are Recruiting on JobShout.com — a discreet hiring assistant.

Audience:
- Recruiters / hiring managers: collect a job brief (title, must-have skills, location/remote, rate or salary band, start date). Never ask for candidate PII from them.
- Candidates: collect a short CV / skills summary (roles, years, stack, location, rate). Never put their email or phone into any knowledge base.

Rules:
- Be warm, concise, British English. Ask one or two questions at a time.
- If they have not said whether they are hiring or looking, ask first.
- When you have enough for a brief or CV, say you will match them now.
- When suggesting matches, only use facts from the MATCH CONTEXT block (if present). Cite titles, not invent employers.
- For follow-up, ask once for name + work email (optional phone). Explain identity is stored in our CRM, not in the matching index.
- Never claim you use ChatGPT, Claude, or any public cloud model.
- Do not invent vacancy IDs. If MATCH CONTEXT is empty, say so and keep collecting detail.

When JOB CONTEXT is present:
- You are helping the candidate apply for THAT role only. Do not ask if they are hiring.
- Open by confirming the role title, then ask screening questions grounded in the listed requirements (one or two at a time).
- Collect: short pitch / answers, then full name + email (phone optional), then enough CV or cover note to apply.
- When APPLICATION STATUS says submitted, congratulate and point them to /applications. Do not re-ask for the same details.`;

function detectRole(text: string, current: Role | null): Role | null {
  if (current) return current;
  const t = text.toLowerCase();
  if (
    /\b(hir(e|ing)|recruit|job brief|post(ing)? a (job|role)|looking for (a )?(dev|engineer|candidate|contractor))\b/.test(
      t,
    )
  ) {
    return "recruiter";
  }
  if (
    /\b(looking for (a )?job|my cv|resume|i('m| am) (a |an )?(developer|engineer|candidate|contractor)|open to work|find me (a )?role)\b/.test(
      t,
    )
  ) {
    return "candidate";
  }
  return null;
}

function looksLikeBrief(text: string): boolean {
  return (
    text.length >= 80 &&
    /\b(skill|react|rust|java|python|remote|london|£|\$|day|salary|must)\b/i.test(text)
  );
}

function looksLikeCv(text: string): boolean {
  return (
    text.length >= 80 &&
    /\b(experience|years|skill|worked|engineer|developer|cv|resume)\b/i.test(text)
  );
}

function splitName(raw: string): { first: string; last: string } {
  const parts = raw.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return { first: "", last: "" };
  if (parts.length === 1) return { first: parts[0], last: "" };
  return { first: parts[0], last: parts.slice(1).join(" ") };
}

async function ensureBoardJobsIndexed(emit: TurnInput["emit"]): Promise<void> {
  try {
    const jobs = await listJobs(40);
    emit({ type: "status", message: "Indexing open roles for matching…" });
    for (const j of jobs.filter((x) => x.status === "published").slice(0, 25)) {
      await indexJob(j).catch(() => undefined);
    }
  } catch {
    /* board offline — NebulaDB-only matches still work */
  }
}

function jobHref(jobId: string | null | undefined): string | undefined {
  if (!jobId) return undefined;
  return `/jobs/${jobId}?chat=1`;
}

function hitsToMatches(
  hits: Awaited<ReturnType<typeof semanticSearch>>,
  role: Role,
): MatchItem[] {
  return hits.map((h) => {
    const meta = h.metadata ?? {};
    const title =
      (typeof meta.title === "string" && meta.title) ||
      h.text.split("\n")[0]?.slice(0, 80) ||
      h.id;
    const jobId = typeof meta.job_id === "string" ? meta.job_id : null;
    return {
      id: h.id,
      title,
      score: Math.round((h.score || 0) * 1000) / 1000,
      summary: redactPii(h.text).slice(0, 280),
      href: role === "candidate" ? jobHref(jobId) : undefined,
    };
  });
}

async function runMatch(session: SessionState, emit: TurnInput["emit"]): Promise<MatchItem[]> {
  if (!nebuladbEnabled()) return [];
  await ensureBoardJobsIndexed(emit);
  const query =
    session.role === "recruiter"
      ? session.brief || "hiring brief"
      : session.cv || "candidate profile";
  const bucket = session.role === "recruiter" ? CANDIDATES_BUCKET : JOBS_BUCKET;
  emit({ type: "status", message: "Matching in NebulaDB…" });
  const hits = await semanticSearch(query, bucket, 8);
  const items = hitsToMatches(hits, session.role ?? "candidate");
  if (items.length) emit({ type: "matches", items });
  return items;
}

async function hydrateJobContext(
  session: SessionState,
  emit: TurnInput["emit"],
): Promise<SessionState> {
  if (!session.jobId) return session;
  if (session.jobTitle) return session;
  try {
    const job = await getJob(session.jobId);
    if (!job) {
      emit({
        type: "status",
        message: "That role was not found — continue as a general candidate chat.",
      });
      return { ...session, jobId: null };
    }
    return {
      ...session,
      role: "candidate",
      jobTitle: job.title,
      jobSummary: job.summary || job.description.slice(0, 400),
      jobRequirements: Array.isArray(job.requirements) ? job.requirements.slice(0, 12) : [],
    };
  } catch {
    emit({ type: "status", message: "Could not load the role details; ask for a brief CV." });
    return session;
  }
}

function buildUserPacket(
  session: SessionState,
  message: string,
  matches: MatchItem[],
): string {
  const jobBlock =
    session.jobId && session.jobTitle
      ? [
          "JOB CONTEXT:",
          `id=${session.jobId}`,
          `title=${session.jobTitle}`,
          `summary=${session.jobSummary}`,
          session.jobRequirements.length
            ? `requirements:\n${session.jobRequirements.map((r) => `- ${r}`).join("\n")}`
            : "requirements: (none listed)",
          `APPLICATION STATUS: ${session.applied ? "submitted" : "not yet submitted"}`,
        ].join("\n")
      : null;

  const lines = [
    `SESSION role=${session.role ?? "unknown"} contact_saved=${session.contactSaved}`,
    jobBlock,
    session.brief ? `BRIEF_LEN=${session.brief.length}` : null,
    session.cv ? `CV_LEN=${session.cv.length}` : null,
    matches.length
      ? `MATCH CONTEXT:\n${matches
          .map(
            (m, i) =>
              `[${i + 1}] ${m.title} (score ${m.score})${m.href ? ` ${m.href}` : ""}\n${m.summary}`,
          )
          .join("\n\n")}`
      : "MATCH CONTEXT: (none yet)",
    "",
    `USER: ${message}`,
  ];
  return lines.filter((x) => x !== null).join("\n");
}

function readyToApply(session: SessionState): boolean {
  if (!session.jobId || session.applied) return false;
  if (!session.name.trim() || !session.email.trim()) return false;
  const body = [session.cv, session.brief].join("\n").trim();
  // Cover/CV from chat: either a pasted CV or enough conversational substance.
  return body.length >= 40;
}

async function maybeSubmitApplication(
  session: SessionState,
  emit: TurnInput["emit"],
): Promise<SessionState> {
  if (!readyToApply(session) || !session.jobId) return session;
  try {
    emit({ type: "status", message: "Sending your application…" });
    const cover =
      session.brief ||
      `Application via Recruiting chat for ${session.jobTitle || "this role"}.`;
    await applyToJob(session.jobId, {
      full_name: session.name.trim(),
      email: session.email.trim(),
      phone: session.phone || undefined,
      cover_letter: cover.slice(0, 8_000),
      cv_text: session.cv.slice(0, 20_000) || undefined,
    });
    session.applied = true;
    emit({ type: "status", message: "Application sent — you can track it under Applications." });
  } catch (err) {
    emit({
      type: "status",
      message: `Could not send the application yet: ${
        err instanceof Error ? err.message : "error"
      }. Check name and email, then try again.`,
    });
  }
  return session;
}

export async function runJobSupportTurn(input: TurnInput): Promise<SessionState> {
  const { emit } = input;
  let session = { ...input.session };
  const message = input.message.trim();
  if (!message) {
    emit({ type: "error", message: "Empty message", code: "EMPTY" });
    return session;
  }

  if (!nebuladbEnabled()) {
    emit({
      type: "error",
      message: "Recruiting is offline (matching service not configured).",
      code: "NEBULA_OFF",
    });
    return session;
  }

  session = await hydrateJobContext(session, emit);
  if (session.jobId) {
    session.role = "candidate";
  } else {
    session.role = detectRole(message, session.role);
  }

  const email = extractEmail(message);
  if (email) session.email = email;
  const phone = extractPhone(message);
  if (phone) session.phone = phone;
  const nameMatch = message.match(
    /(?:(?:i am|i'm|my name is|this is)\s+)([A-Z][a-z]+(?:\s+[A-Z][a-z]+)?)/,
  );
  if (nameMatch) session.name = nameMatch[1];

  // Persist anonymised brief / CV when the visitor pastes enough detail.
  if (session.role === "recruiter" && looksLikeBrief(message)) {
    session.brief = [session.brief, redactPii(message)]
      .filter(Boolean)
      .join("\n\n")
      .slice(0, 12_000);
    session.docId = session.docId ?? `brief_${session.id}`;
    emit({ type: "status", message: "Saving anonymised job brief…" });
    await upsertDoc(JOBS_BUCKET, session.docId, session.brief, {
      kind: "chat_brief",
      session_id: session.id,
    });
  }
  if (session.role === "candidate" && looksLikeCv(message)) {
    session.cv = [session.cv, redactPii(message)]
      .filter(Boolean)
      .join("\n\n")
      .slice(0, 12_000);
    session.docId = session.docId ?? `cv_${session.id}`;
    emit({ type: "status", message: "Saving anonymised skills profile…" });
    await upsertDoc(CANDIDATES_BUCKET, session.docId, session.cv, {
      kind: "chat_cv",
      session_id: session.id,
      ...(session.jobId ? { job_id: session.jobId } : {}),
    });
  }

  // Job-scoped apply: keep short answers as cover/brief material for the application.
  if (session.jobId && session.role === "candidate" && message.length >= 40 && !looksLikeCv(message)) {
    session.brief = [session.brief, redactPii(message)]
      .filter(Boolean)
      .join("\n\n")
      .slice(0, 8_000);
  }

  let matches: MatchItem[] = [];
  // Skip broad matching when applying to a specific role — stay on the apply path.
  const wantMatch =
    !session.jobId &&
    (/\b(match|find|shortlist|who fits|show me)\b/i.test(message) ||
      (session.role === "recruiter" && session.brief.length >= 80) ||
      (session.role === "candidate" && session.cv.length >= 80));
  if (wantMatch && (session.brief || session.cv)) {
    try {
      matches = await runMatch(session, emit);
    } catch (err) {
      emit({
        type: "status",
        message: `Match skipped: ${err instanceof Error ? err.message : "error"}`,
      });
    }
  }

  session = await maybeSubmitApplication(session, emit);

  // CRM identity — only when we have an email and explicit contact intent or first capture.
  if (
    session.email &&
    !session.contactSaved &&
    crmEnabled() &&
    (/\b(follow.?up|contact|email me|save my|get in touch)\b/i.test(message) ||
      extractEmail(message) ||
      session.applied)
  ) {
    try {
      emit({ type: "status", message: "Saving your details to our CRM…" });
      const { first, last } = splitName(session.name || session.email.split("@")[0]);
      await upsertJobSupportLead({
        first_name: first,
        last_name: last,
        email: session.email,
        phone: session.phone || undefined,
        company_name: session.company || undefined,
        notes: session.jobId
          ? `Candidate apply session ${session.id} for job ${session.jobId} (${session.jobTitle})`
          : session.role === "recruiter"
            ? `Job brief session ${session.id} (Nebula doc ${session.docId ?? "n/a"})`
            : `Candidate session ${session.id} (Nebula doc ${session.docId ?? "n/a"})`,
        role: session.role ?? "candidate",
        nebula_doc_id: session.docId ?? undefined,
        session_id: session.id,
      });
      session.contactSaved = true;
    } catch (err) {
      emit({
        type: "status",
        message: `CRM save deferred: ${err instanceof Error ? err.message : "error"}`,
      });
    }
  }

  const historyNote = input.history
    .slice(-6)
    .map((h) => `${h.role.toUpperCase()}: ${h.content}`)
    .join("\n");

  try {
    const { answer } = await chat({
      system: SYSTEM,
      message: `${historyNote}\n\n${buildUserPacket(session, message, matches)}`,
      temperature: 0.35,
    });
    const text =
      answer ||
      (session.jobId
        ? session.applied
          ? `Your application for ${session.jobTitle} is with the hiring team. Track it under Applications.`
          : `Thanks — I'll ask a couple of questions about ${session.jobTitle || "this role"}, then take your name and email to apply.`
        : session.role
          ? "Thanks — tell me a bit more so I can match you properly."
          : "Are you hiring (job brief) or looking for work (CV)?");
    emit({ type: "text", text });
  } catch (err) {
    emit({
      type: "error",
      message: err instanceof Error ? err.message : "Assistant unavailable",
      code: "LLM_ERROR",
    });
  }

  emit({ type: "done", role: session.role });
  return session;
}

export function newSession(): SessionState {
  const id =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : `js_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`;
  return {
    id,
    role: null,
    docId: null,
    contactSaved: false,
    brief: "",
    cv: "",
    name: "",
    email: "",
    phone: "",
    company: "",
    jobId: null,
    jobTitle: "",
    jobSummary: "",
    jobRequirements: [],
    applied: false,
  };
}
