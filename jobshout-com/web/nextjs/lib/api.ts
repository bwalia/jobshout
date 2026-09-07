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

export async function getJob(id: string): Promise<Job | null> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${id}`, { next: { revalidate: 30 } });
  if (res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load job");
  return (await res.json()) as Job;
}

export async function createJob(input: CreateJobInput): Promise<Job> {
  const res = await fetch(`${API_BASE}/api/v1/jobs`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(input),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to publish job");
  return (await res.json()) as Job;
}

/* --- Candidate profiles --------------------------------------------------- */

export async function getProfileByEmail(email: string): Promise<CandidateProfile | null> {
  const res = await fetch(
    `${API_BASE}/api/v1/profiles/by-email?email=${encodeURIComponent(email)}`,
    { cache: "no-store" },
  );
  if (res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load profile");
  return (await res.json()) as CandidateProfile;
}

export async function upsertProfile(
  input: UpsertCandidateProfileInput,
): Promise<CandidateProfile> {
  const res = await fetch(`${API_BASE}/api/v1/profiles`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(input),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to save profile");
  return (await res.json()) as CandidateProfile;
}

export async function listProfileMatches(profileId: string): Promise<JobMatch[]> {
  const res = await fetch(`${API_BASE}/api/v1/profiles/${profileId}/matches?limit=20`, {
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
): Promise<JobApplication> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${jobId}/applications`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(input),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to send application");
  return (await res.json()) as JobApplication;
}

export async function listMyApplications(email: string): Promise<JobApplicationWithJob[]> {
  const res = await fetch(
    `${API_BASE}/api/v1/applications?email=${encodeURIComponent(email)}&limit=50`,
    { cache: "no-store" },
  );
  if (!res.ok) throw await failure(res, "Failed to load applications");
  const body = (await res.json()) as { data: JobApplicationWithJob[] };
  return body.data;
}

export async function countApplications(jobId: string): Promise<number> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${jobId}/applications?limit=1`, {
    cache: "no-store",
  });
  if (!res.ok) return 0;
  const body = (await res.json()) as { total: number };
  return body.total;
}
