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
