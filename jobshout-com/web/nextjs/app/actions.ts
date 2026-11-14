"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import type { FormState } from "@/lib/form-state";
import {
  applyToJob,
  closeJob,
  createJob,
  deleteJob,
  publishJob,
  setApplicationStatus,
  updateJob,
  withdrawApplication,
  setJobSaved,
  saveJobAlert,
  deleteJobAlert,
  EMPLOYMENT_TYPES,
  RECRUITER_STATUSES,
  type ApplicationStatus,
  upsertProfile,
  type CreateJobInput,
  type EmploymentType,
} from "@/lib/api";
import { currentViewer } from "@/lib/session";



function text(form: FormData, key: string): string {
  return (form.get(key) as string | null)?.trim() ?? "";
}

function list(form: FormData, key: string): string[] {
  return text(form, key)
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean);
}

function number(form: FormData, key: string): number | null {
  const raw = text(form, key).replace(/[, ]/g, "");
  if (!raw) return null;
  const n = Number(raw);
  return Number.isFinite(n) && n >= 0 ? n : null;
}

function isEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
}

/* -------------------------------------------------------------------------- */
/* Apply to a job                                                             */
/* -------------------------------------------------------------------------- */

export async function applyAction(
  _prev: FormState,
  form: FormData,
): Promise<FormState> {
  const jobId = text(form, "job_id");
  const full_name = text(form, "full_name");
  const email = text(form, "email");
  const cover_letter = text(form, "cover_letter");
  const cv_text = text(form, "cv_text");
  const cv_url = text(form, "cv_url");

  const fieldErrors: Record<string, string> = {};
  if (!full_name) fieldErrors.full_name = "Tell the employer who you are.";
  if (!email) fieldErrors.email = "We need an email to reply to.";
  else if (!isEmail(email)) fieldErrors.email = "That does not look like an email address.";
  if (!cover_letter && !cv_text && !cv_url) {
    fieldErrors.cover_letter = "Add a message, paste your CV, or link to one.";
  }
  if (cv_url && !/^https?:\/\//i.test(cv_url)) {
    fieldErrors.cv_url = "Links need to start with http:// or https://";
  }
  if (Object.keys(fieldErrors).length > 0) {
    return { ok: false, message: "Check the highlighted fields.", fieldErrors };
  }

  const viewer = await currentViewer();
  if (!viewer) {
    return { ok: false, message: "Sign in to apply for roles." };
  }
  if (email && email.toLowerCase() !== viewer.email.toLowerCase()) {
    return {
      ok: false,
      message: "Apply with the email on your signed-in account.",
      fieldErrors: { email: "Must match your signed-in email." },
    };
  }

  try {
    const application = await applyToJob(
      jobId,
      {
        full_name,
        email: viewer.email,
        phone: text(form, "phone"),
        cover_letter,
        cv_text,
        cv_url,
      },
      viewer,
    );
    revalidatePath("/applications");
    return {
      ok: true,
      message: "Application sent.",
      result: { id: application.id, jobId, status: application.status },
    };
  } catch (e) {
    return {
      ok: false,
      message: e instanceof Error ? e.message : "Could not send your application.",
    };
  }
}

/* -------------------------------------------------------------------------- */
/* Post a job                                                                 */
/* -------------------------------------------------------------------------- */

type ParsedJob =
  | { ok: true; input: CreateJobInput }
  | { ok: false; state: FormState };

/** Shared by create and edit, so both validate the same way. */
function parseJobForm(form: FormData): ParsedJob {
  const title = text(form, "title");
  const description = text(form, "description");
  const country = text(form, "country");
  const min_amount = number(form, "min_amount");
  const max_amount = number(form, "max_amount");

  const fieldErrors: Record<string, string> = {};
  if (!title) fieldErrors.title = "Give the role a title.";
  if (!description) fieldErrors.description = "Describe the role so candidates can self-select.";
  else if (description.length < 40) {
    fieldErrors.description = "A little more detail — at least a couple of sentences.";
  }
  if (!country) fieldErrors.country = "Country is required.";
  if (min_amount !== null && max_amount !== null && min_amount > max_amount) {
    fieldErrors.max_amount = "Maximum must be at or above the minimum.";
  }
  if (Object.keys(fieldErrors).length > 0) {
    return { ok: false, state: { ok: false, message: "Check the highlighted fields.", fieldErrors } };
  }

  return {
    ok: true,
    input: {
      title,
      summary: text(form, "summary"),
      description,
      employment_type: (text(form, "employment_type") || "permanent") as EmploymentType,
      location: {
        country,
        region: text(form, "region") || null,
        city: text(form, "city") || null,
        remote: form.get("remote") === "on",
      },
      compensation: {
        currency: text(form, "currency") || null,
        min_amount,
        max_amount,
        period: text(form, "period") || null,
      },
      requirements: list(form, "requirements"),
      // Drafts stay off the public board until the employer publishes.
      publish: form.get("publish") !== "draft",
    },
  };
}

function revalidateJob(jobId: string) {
  revalidatePath("/jobs/mine");
  revalidatePath(`/jobs/${jobId}`);
  revalidatePath("/jobs");
  revalidatePath("/");
}

export async function createJobAction(
  _prev: FormState,
  form: FormData,
): Promise<FormState> {
  const parsed = parseJobForm(form);
  if (!parsed.ok) return parsed.state;

  const viewer = await currentViewer();
  if (!viewer) {
    return { ok: false, message: "Sign in to post a job." };
  }

  try {
    const job = await createJob(parsed.input, viewer);
    revalidateJob(job.id);
    return {
      ok: true,
      message: job.status === "published" ? "Job published." : "Draft saved.",
      result: { jobId: job.id, status: job.status },
    };
  } catch (e) {
    return {
      ok: false,
      message: e instanceof Error ? e.message : "Could not publish the job.",
    };
  }
}

/** Save edits to a job you posted. Bound to the job id by the edit page. */
export async function updateJobAction(
  jobId: string,
  _prev: FormState,
  form: FormData,
): Promise<FormState> {
  const parsed = parseJobForm(form);
  if (!parsed.ok) return parsed.state;

  const viewer = await currentViewer();
  if (!viewer) {
    return { ok: false, message: "Sign in to edit this job." };
  }

  try {
    const job = await updateJob(jobId, parsed.input, viewer);
    revalidateJob(job.id);
    return {
      ok: true,
      message: job.status === "published" ? "Changes are live." : "Draft saved.",
      result: { jobId: job.id, status: job.status },
    };
  } catch (e) {
    return {
      ok: false,
      message: e instanceof Error ? e.message : "Could not save your changes.",
    };
  }
}

/**
 * Publish a draft, close a live job, or delete a draft from /jobs/mine.
 * The outcome comes back on the URL so the page can say what happened.
 */
export async function manageJobAction(form: FormData): Promise<void> {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=/jobs/mine");
  const jobId = text(form, "job_id");
  const op = text(form, "op");
  if (!jobId || !["publish", "close", "delete"].includes(op)) redirect("/jobs/mine");

  let outcome: string;
  try {
    if (op === "publish") await publishJob(jobId, viewer);
    else if (op === "close") await closeJob(jobId, viewer);
    else await deleteJob(jobId, viewer);
    outcome = `done=${op}`;
  } catch (e) {
    const message = e instanceof Error ? e.message : "That did not work.";
    outcome = `error=${encodeURIComponent(message)}`;
  }
  revalidateJob(jobId);
  redirect(`/jobs/mine?${outcome}`);
}

/* -------------------------------------------------------------------------- */
/* Save matching profile                                                      */
/* -------------------------------------------------------------------------- */

export async function saveProfileAction(
  _prev: FormState,
  form: FormData,
): Promise<FormState> {
  const email = text(form, "email");
  const display_name = text(form, "display_name");

  const fieldErrors: Record<string, string> = {};
  if (!display_name) fieldErrors.display_name = "What should employers call you?";
  if (!email) fieldErrors.email = "Email is how your profile is found.";
  else if (!isEmail(email)) fieldErrors.email = "That does not look like an email address.";
  if (Object.keys(fieldErrors).length > 0) {
    return { ok: false, message: "Check the highlighted fields.", fieldErrors };
  }

  const years = number(form, "years_experience");
  const country = text(form, "country");

  const viewer = await currentViewer();
  if (!viewer) {
    return { ok: false, message: "Sign in to save your matching profile." };
  }

  try {
    const profile = await upsertProfile(
      {
      email: viewer.email,
      display_name,
      headline: text(form, "headline"),
      summary: text(form, "summary"),
      skills: list(form, "skills"),
      years_experience: years === null ? null : Math.round(years),
      preferred_roles: list(form, "preferred_roles"),
      preferred_locations: country
        ? [
            {
              country,
              city: text(form, "city") || null,
              remote: form.get("open_to_remote") === "on",
            },
          ]
        : [],
      preferred_employment_types: form
        .getAll("preferred_employment_types")
        .map((v) => v as EmploymentType),
      open_to_remote: form.get("open_to_remote") === "on",
      salary_expectation: {
        currency: text(form, "currency") || null,
        min_amount: number(form, "salary_min"),
        period: text(form, "period") || null,
      },
      cv_text: text(form, "cv_text"),
      matching_notes: text(form, "matching_notes"),
      },
      viewer,
    );
    revalidatePath("/profile");
    return {
      ok: true,
      message: "Profile saved.",
      result: { profileId: profile.id },
    };
  } catch (e) {
    return {
      ok: false,
      message: e instanceof Error ? e.message : "Could not save your profile.",
    };
  }
}

/* -------------------------------------------------------------------------- */
/* Applications: hiring team and candidate                                    */
/* -------------------------------------------------------------------------- */

/** The job's poster moves an applicant to a new stage. */
export async function setApplicationStatusAction(form: FormData): Promise<void> {
  const jobId = text(form, "job_id");
  const applicationId = text(form, "application_id");
  const status = text(form, "status") as ApplicationStatus;
  const back = `/jobs/${jobId}/applicants`;
  const viewer = await currentViewer();
  if (!viewer) redirect(`/login?callbackUrl=${encodeURIComponent(back)}`);
  if (!jobId || !applicationId || !RECRUITER_STATUSES.includes(status)) redirect(back);

  let outcome: string;
  try {
    await setApplicationStatus(jobId, applicationId, status, viewer);
    outcome = `done=${status}`;
  } catch (e) {
    outcome = `error=${encodeURIComponent(e instanceof Error ? e.message : "That did not work.")}`;
  }
  revalidatePath(back);
  revalidatePath("/applications");
  redirect(`${back}?${outcome}#${applicationId}`);
}

/** The candidate withdraws their own application. */
export async function withdrawApplicationAction(form: FormData): Promise<void> {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=%2Fapplications");
  const applicationId = text(form, "application_id");
  if (!applicationId) redirect("/applications");

  let outcome: string;
  try {
    await withdrawApplication(applicationId, viewer);
    outcome = "done=withdrawn";
  } catch (e) {
    outcome = `error=${encodeURIComponent(e instanceof Error ? e.message : "That did not work.")}`;
  }
  revalidatePath("/applications");
  redirect(`/applications?${outcome}`);
}

/* -------------------------------------------------------------------------- */
/* Saved jobs and job alerts                                                  */
/* -------------------------------------------------------------------------- */

/** Only same-site paths are followed after an action. */
function safeBack(value: string, fallback: string): string {
  return value.startsWith("/") && !value.startsWith("//") ? value : fallback;
}

function withParam(path: string, param: string): string {
  const [base, hash] = path.split("#");
  return `${base}${base.includes("?") ? "&" : "?"}${param}${hash ? `#${hash}` : ""}`;
}

/** Save or unsave a job, then go back where the button was. */
export async function toggleSavedJobAction(form: FormData): Promise<void> {
  const jobId = text(form, "job_id");
  const back = safeBack(text(form, "back"), `/jobs/${jobId}`);
  const viewer = await currentViewer();
  if (!viewer) redirect(`/login?callbackUrl=${encodeURIComponent(back)}`);
  const save = text(form, "op") !== "unsave";
  let outcome: string;
  try {
    await setJobSaved(jobId, save, viewer);
    outcome = save ? "saved=1" : "saved=0";
  } catch (e) {
    outcome = `error=${encodeURIComponent(e instanceof Error ? e.message : "That did not work.")}`;
  }
  revalidatePath("/jobs/saved");
  revalidatePath(`/jobs/${jobId}`);
  redirect(withParam(back, outcome));
}

/** Create (or, with alert_id, update) a job alert from a form. */
export async function saveJobAlertAction(form: FormData): Promise<void> {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=%2Fjobs%2Falerts");
  const id = text(form, "alert_id") || undefined;
  const types = form
    .getAll("types")
    .map(String)
    .filter((t): t is EmploymentType => (EMPLOYMENT_TYPES as string[]).includes(t));
  let outcome: string;
  try {
    await saveJobAlert(
      {
        q: text(form, "q"),
        location: text(form, "location"),
        types,
        remote: form.get("remote") === "on" || form.get("remote") === "1",
      },
      viewer,
      id,
    );
    outcome = id ? "done=updated" : "done=created";
  } catch (e) {
    outcome = `error=${encodeURIComponent(e instanceof Error ? e.message : "That did not work.")}`;
  }
  revalidatePath("/jobs/alerts");
  redirect(`/jobs/alerts?${outcome}`);
}

export async function deleteJobAlertAction(form: FormData): Promise<void> {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=%2Fjobs%2Falerts");
  let outcome = "done=deleted";
  try {
    await deleteJobAlert(text(form, "alert_id"), viewer);
  } catch (e) {
    outcome = `error=${encodeURIComponent(e instanceof Error ? e.message : "That did not work.")}`;
  }
  revalidatePath("/jobs/alerts");
  redirect(`/jobs/alerts?${outcome}`);
}
