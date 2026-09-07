"use server";

import { revalidatePath } from "next/cache";
import {
  applyToJob,
  createJob,
  upsertProfile,
  type CreateJobInput,
  type EmploymentType,
} from "@/lib/api";

/** Shared shape for every form action: one error bag, one success payload. */
export type FormState = {
  ok: boolean;
  message?: string;
  fieldErrors?: Record<string, string>;
  /** Set on success so the client can route on or show a receipt. */
  result?: { id?: string; jobId?: string; profileId?: string; status?: string };
};

export const EMPTY_FORM_STATE: FormState = { ok: false };

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

  try {
    const application = await applyToJob(jobId, {
      full_name,
      email,
      phone: text(form, "phone"),
      cover_letter,
      cv_text,
      cv_url,
    });
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

export async function createJobAction(
  _prev: FormState,
  form: FormData,
): Promise<FormState> {
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
    return { ok: false, message: "Check the highlighted fields.", fieldErrors };
  }

  const input: CreateJobInput = {
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
  };

  try {
    const job = await createJob(input);
    revalidatePath("/jobs");
    revalidatePath("/");
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

  try {
    const profile = await upsertProfile({
      email,
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
    });
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
