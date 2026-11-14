import { headers as identityHeaders, type Viewer } from "@/lib/insights";

export type EmploymentType =
  | "permanent"
  | "contract"
  | "freelance"
  | "temporary"
  | "part_time"
  | "internship"
  | "apprenticeship";

export const EMPLOYMENT_TYPES: EmploymentType[] = [
  "permanent",
  "contract",
  "freelance",
  "temporary",
  "part_time",
  "internship",
  "apprenticeship",
];

export type JobStatus = "draft" | "published" | "closed" | "archived";

export type ApplicationStatus =
  | "submitted"
  | "reviewing"
  | "shortlisted"
  | "interviewing"
  | "offered"
  | "rejected"
  | "withdrawn";

export interface Location {
  country: string;
  region?: string | null;
  city?: string | null;
  remote?: boolean;
}

export interface Compensation {
  currency?: string | null;
  min_amount?: number | null;
  max_amount?: number | null;
  period?: string | null;
}

export interface Job {
  id: string;
  organisation_id: string;
  title: string;
  summary: string;
  description: string;
  employment_type: EmploymentType;
  location: Location;
  compensation: Compensation;
  requirements: string[];
  status: JobStatus;
  created_at: string;
  updated_at: string;
  published_at?: string | null;
}

export interface JobListResponse {
  data: Job[];
  limit: number;
  offset: number;
  /** Matching jobs across all pages (public board only). */
  total?: number;
}

export type JobSearchParams = {
  q?: string;
  where?: string;
  types?: string[];
  remote?: boolean;
};

/** Most matches the board pulls for one search before local filters apply. */
export const BOARD_SEARCH_CAP = 1000;

/**
 * Search the public board on the server (keyword, location, work type,
 * remote) and return every match up to BOARD_SEARCH_CAP, plus the server's
 * total. Category, salary floor and sort are applied by the page.
 */
export async function searchJobs(
  params: JobSearchParams,
): Promise<{ jobs: Job[]; total: number }> {
  const qs = new URLSearchParams();
  if (params.q) qs.set("q", params.q);
  if (params.where) qs.set("where", params.where);
  if (params.types?.length) qs.set("type", params.types.join(","));
  if (params.remote) qs.set("remote", "1");
  const jobs: Job[] = [];
  let total = 0;
  for (let offset = 0; offset < BOARD_SEARCH_CAP; offset += 100) {
    qs.set("limit", "100");
    qs.set("offset", String(offset));
    const res = await fetch(`${API_BASE}/api/v1/jobs?${qs}`, { next: { revalidate: 30 } });
    if (!res.ok) throw await failure(res, "Failed to search jobs");
    const body = (await res.json()) as JobListResponse;
    jobs.push(...body.data);
    total = body.total ?? jobs.length;
    if (body.data.length < 100 || jobs.length >= total) break;
  }
  return { jobs, total };
}

export interface CandidateProfile {
  id: string;
  email: string;
  display_name: string;
  headline: string;
  summary: string;
  skills: string[];
  years_experience?: number | null;
  preferred_roles: string[];
  preferred_locations: Location[];
  preferred_employment_types: EmploymentType[];
  open_to_remote: boolean;
  salary_expectation: Compensation;
  cv_text: string;
  matching_notes: string;
  created_at: string;
  updated_at: string;
}

export type UpsertCandidateProfileInput = {
  email: string;
  display_name: string;
  headline?: string;
  summary?: string;
  skills?: string[];
  years_experience?: number | null;
  preferred_roles?: string[];
  preferred_locations?: Location[];
  preferred_employment_types?: EmploymentType[];
  open_to_remote?: boolean;
  salary_expectation?: Compensation;
  cv_text?: string;
  matching_notes?: string;
};

export interface JobMatch {
  job: Job;
  score: number;
  reasons: string[];
}

export interface JobApplication {
  id: string;
  job_id: string;
  candidate_profile_id?: string | null;
  full_name: string;
  email: string;
  phone: string;
  cover_letter: string;
  cv_text: string;
  cv_url: string;
  status: ApplicationStatus;
  created_at: string;
  updated_at: string;
}

/** `GET /api/v1/applications` flattens the application and nests the job. */
export type JobApplicationWithJob = JobApplication & { job: Job };

export type CreateJobApplicationInput = {
  full_name: string;
  email: string;
  phone?: string;
  cover_letter?: string;
  cv_text?: string;
  cv_url?: string;
};

export type CreateJobInput = {
  title: string;
  summary?: string;
  description: string;
  employment_type: EmploymentType;
  location: Location;
  compensation?: Compensation;
  requirements?: string[];
  publish?: boolean;
};

const API_BASE =
  typeof window === "undefined"
    ? (process.env.JOBSHOUT_COM_API_URL ?? "http://127.0.0.1:8088")
    : (process.env.NEXT_PUBLIC_JOBSHOUT_COM_API_URL ?? "");

/** Pull the API's structured error message out, falling back to the status. */
async function failure(res: Response, fallback: string): Promise<Error> {
  const body = (await res.json().catch(() => null)) as {
    error?: { message?: string };
  } | null;
  return new Error(body?.error?.message ?? `${fallback} (${res.status})`);
}

/* --- Jobs ----------------------------------------------------------------- */

export async function listJobs(limit = 100): Promise<Job[]> {
  const res = await fetch(`${API_BASE}/api/v1/jobs?limit=${limit}`, {
    next: { revalidate: 30 },
  });
  if (!res.ok) throw await failure(res, "Failed to list jobs");
  const body = (await res.json()) as JobListResponse;
  return body.data;
}

/**
 * One job. Published jobs are public and cached briefly. Pass the viewer to
 * also see your own drafts and closed jobs (the API 404s them for anyone
 * else); that read is never cached, so it cannot leak across users.
 */
export async function getJob(id: string, viewer?: Viewer | null): Promise<Job | null> {
  const res = await fetch(
    `${API_BASE}/api/v1/jobs/${id}`,
    viewer
      ? { headers: identityHeaders(viewer), cache: "no-store" }
      : { next: { revalidate: 30 } },
  );
  if (res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load job");
  return (await res.json()) as Job;
}

/** Posting requires sign-in; the poster is recorded (never shown) so they can
 * link the job from their AI Showcase entries and review applications. */
export async function createJob(input: CreateJobInput, viewer?: Viewer | null): Promise<Job> {
  const res = await fetch(`${API_BASE}/api/v1/jobs`, {
    method: "POST",
    headers: identityHeaders(viewer),
    body: JSON.stringify(input),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to publish job");
  return (await res.json()) as Job;
}

/** Every job the signed-in poster created — drafts and closed ones too. */
export async function listMyJobs(viewer: Viewer): Promise<Job[]> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/mine`, {
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load your jobs");
  const body = (await res.json()) as JobListResponse;
  return body.data;
}

export async function publishJob(jobId: string, viewer?: Viewer | null): Promise<Job> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${jobId}/publish`, {
    method: "POST",
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to publish job");
  return (await res.json()) as Job;
}

/** Replace a job's details. Drafts stay drafts unless `publish` is true. */
export async function updateJob(
  jobId: string,
  input: CreateJobInput,
  viewer?: Viewer | null,
): Promise<Job> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${jobId}`, {
    method: "PATCH",
    headers: identityHeaders(viewer),
    body: JSON.stringify(input),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to save the job");
  return (await res.json()) as Job;
}

/** Delete a draft nobody has applied to. Live jobs are closed instead. */
export async function deleteJob(jobId: string, viewer?: Viewer | null): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${jobId}`, {
    method: "DELETE",
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to delete the draft");
}

export async function closeJob(jobId: string, viewer?: Viewer | null): Promise<Job> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${jobId}/close`, {
    method: "POST",
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to close job");
  return (await res.json()) as Job;
}

/* --- Candidate profiles --------------------------------------------------- */

/** Load the signed-in user's matching profile (owner-only). */
export async function getMyProfile(viewer?: Viewer | null): Promise<CandidateProfile | null> {
  const res = await fetch(`${API_BASE}/api/v1/profiles/me`, {
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (res.status === 401 || res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load profile");
  return (await res.json()) as CandidateProfile;
}

/** @deprecated Use getMyProfile — email query access was removed in Phase 0. */
export async function getProfileByEmail(
  _email: string,
  viewer?: Viewer | null,
): Promise<CandidateProfile | null> {
  return getMyProfile(viewer);
}

export async function upsertProfile(
  input: UpsertCandidateProfileInput,
  viewer?: Viewer | null,
): Promise<CandidateProfile> {
  const res = await fetch(`${API_BASE}/api/v1/profiles`, {
    method: "POST",
    headers: identityHeaders(viewer),
    body: JSON.stringify(input),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to save profile");
  return (await res.json()) as CandidateProfile;
}

export async function listProfileMatches(
  profileId: string,
  viewer?: Viewer | null,
): Promise<JobMatch[]> {
  const res = await fetch(`${API_BASE}/api/v1/profiles/${profileId}/matches?limit=20`, {
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load matches");
  const body = (await res.json()) as { data: JobMatch[] };
  return body.data;
}

/* --- Applications --------------------------------------------------------- */

export async function applyToJob(
  jobId: string,
  input: CreateJobApplicationInput,
  viewer?: Viewer | null,
): Promise<JobApplication> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${jobId}/applications`, {
    method: "POST",
    headers: identityHeaders(viewer),
    body: JSON.stringify(input),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to send application");
  return (await res.json()) as JobApplication;
}

export async function listMyApplications(
  viewer?: Viewer | null,
): Promise<JobApplicationWithJob[]> {
  const res = await fetch(`${API_BASE}/api/v1/applications?limit=50`, {
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load applications");
  const body = (await res.json()) as { data: JobApplicationWithJob[] };
  return body.data;
}

/** Applicants for one job. Only its poster (or staff) may read them. */
export async function listApplicationsForJob(
  jobId: string,
  viewer: Viewer,
  { limit = 50, offset = 0 }: { limit?: number; offset?: number } = {},
): Promise<{ data: JobApplication[]; total: number }> {
  const res = await fetch(
    `${API_BASE}/api/v1/jobs/${jobId}/applications?limit=${limit}&offset=${offset}`,
    { headers: identityHeaders(viewer), cache: "no-store" },
  );
  if (!res.ok) throw await failure(res, "Failed to load applicants");
  return (await res.json()) as { data: JobApplication[]; total: number };
}

/** Stages the hiring team can move an application to. */
export const RECRUITER_STATUSES: ApplicationStatus[] = [
  "submitted",
  "reviewing",
  "shortlisted",
  "interviewing",
  "offered",
  "rejected",
];

export async function setApplicationStatus(
  jobId: string,
  applicationId: string,
  status: ApplicationStatus,
  viewer: Viewer,
): Promise<JobApplication> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${jobId}/applications/${applicationId}`, {
    method: "PATCH",
    headers: identityHeaders(viewer),
    body: JSON.stringify({ status }),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to update the application");
  return (await res.json()) as JobApplication;
}

export async function withdrawApplication(
  applicationId: string,
  viewer: Viewer,
): Promise<JobApplication> {
  const res = await fetch(`${API_BASE}/api/v1/applications/${applicationId}/withdraw`, {
    method: "POST",
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to withdraw the application");
  return (await res.json()) as JobApplication;
}

export async function countApplications(
  jobId: string,
  viewer?: Viewer | null,
): Promise<number> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${jobId}/applications?limit=1`, {
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) return 0;
  const body = (await res.json()) as { total: number };
  return body.total;
}

/* --- Saved jobs and job alerts ------------------------------------------- */

export async function listSavedJobs(viewer: Viewer): Promise<Job[]> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/saved`, {
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load saved jobs");
  return ((await res.json()) as JobListResponse).data;
}

/** false on any failure: the Save button then just offers to save. */
export async function isJobSaved(jobId: string, viewer: Viewer): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/api/v1/jobs/${jobId}/save`, {
      headers: identityHeaders(viewer),
      cache: "no-store",
    });
    if (!res.ok) return false;
    return ((await res.json()) as { saved: boolean }).saved;
  } catch {
    return false;
  }
}

export async function setJobSaved(jobId: string, saved: boolean, viewer: Viewer): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${jobId}/save`, {
    method: saved ? "PUT" : "DELETE",
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, saved ? "Could not save the job" : "Could not remove the job");
}

export type JobAlert = {
  id: string;
  q: string;
  location: string;
  types: EmploymentType[];
  remote: boolean;
  created_at: string;
};

export type JobAlertInput = Omit<JobAlert, "id" | "created_at">;

export async function listJobAlerts(viewer: Viewer): Promise<JobAlert[]> {
  const res = await fetch(`${API_BASE}/api/v1/job-alerts`, {
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load job alerts");
  return ((await res.json()) as { data: JobAlert[] }).data;
}

export async function saveJobAlert(
  input: JobAlertInput,
  viewer: Viewer,
  id?: string,
): Promise<JobAlert> {
  const res = await fetch(`${API_BASE}/api/v1/job-alerts${id ? `/${id}` : ""}`, {
    method: id ? "PUT" : "POST",
    headers: identityHeaders(viewer),
    body: JSON.stringify(input),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Could not save the alert");
  return (await res.json()) as JobAlert;
}

export async function deleteJobAlert(id: string, viewer: Viewer): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/job-alerts/${id}`, {
    method: "DELETE",
    headers: identityHeaders(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Could not delete the alert");
}
