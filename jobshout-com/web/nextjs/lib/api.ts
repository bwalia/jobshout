export type EmploymentType =
  | "permanent"
  | "contract"
  | "freelance"
  | "temporary"
  | "part_time"
  | "internship"
  | "apprenticeship";

export type JobStatus = "draft" | "published" | "closed" | "archived";

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

const API_BASE = process.env.JOBSHOUT_COM_API_URL ?? "http://127.0.0.1:8088";

export async function listJobs(): Promise<Job[]> {
  const res = await fetch(`${API_BASE}/api/v1/jobs?limit=50`, {
    next: { revalidate: 30 },
  });
  if (!res.ok) {
    throw new Error(`Failed to list jobs (${res.status})`);
  }
  const body = (await res.json()) as JobListResponse;
  return body.data;
}

export async function getJob(id: string): Promise<Job | null> {
  const res = await fetch(`${API_BASE}/api/v1/jobs/${id}`, {
    next: { revalidate: 30 },
  });
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`Failed to load job (${res.status})`);
  }
  return (await res.json()) as Job;
}

export async function getProfileByEmail(email: string): Promise<CandidateProfile | null> {
  const res = await fetch(
    `${API_BASE}/api/v1/profiles/by-email?email=${encodeURIComponent(email)}`,
    { cache: "no-store" },
  );
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`Failed to load profile (${res.status})`);
  }
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
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: { message?: string };
    } | null;
    throw new Error(body?.error?.message ?? `Failed to save profile (${res.status})`);
  }
  return (await res.json()) as CandidateProfile;
}

export async function listProfileMatches(profileId: string): Promise<JobMatch[]> {
  const res = await fetch(`${API_BASE}/api/v1/profiles/${profileId}/matches?limit=20`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`Failed to load matches (${res.status})`);
  }
  const body = (await res.json()) as { data: JobMatch[] };
  return body.data;
}

export function formatCompensation(c: Compensation): string {
  if (!c.min_amount && !c.max_amount) return "Compensation not listed";
  const cur = c.currency ?? "";
  const period = c.period ? ` / ${c.period}` : "";
  if (c.min_amount && c.max_amount) {
    return `${cur} ${Math.round(c.min_amount).toLocaleString()}–${Math.round(c.max_amount).toLocaleString()}${period}`;
  }
  const n = c.min_amount ?? c.max_amount ?? 0;
  return `${cur} ${Math.round(n).toLocaleString()}${period}`;
}

export function formatLocation(loc: Location): string {
  const parts = [loc.city, loc.region, loc.country].filter(Boolean);
  const base = parts.join(", ") || loc.country;
  return loc.remote ? `${base} · Remote` : base;
}
