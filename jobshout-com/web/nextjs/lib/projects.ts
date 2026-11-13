import { API_BASE, failure, headers, type Viewer } from "@/lib/insights";

export type ProjectType = "fixed_price" | "hourly" | "retainer" | "milestone";

export type Project = {
  id: string;
  title: string;
  summary: string;
  description: string;
  project_type: ProjectType;
  status: string;
  visibility: string;
  budget_min?: string | number | null;
  budget_max?: string | number | null;
  currency: string;
  duration_estimate?: string | null;
  skills_required: string[];
  deliverables: string[];
  bidding_end_at?: string | null;
  max_bids?: number | null;
  experience_level?: string;
  bid_count?: number;
  created_at: string;
  updated_at: string;
};

export type CreateProjectInput = {
  title: string;
  description: string;
  summary?: string;
  project_type: ProjectType;
  budget_min?: number | null;
  budget_max?: number | null;
  currency?: string;
  duration_estimate?: string;
  skills_required?: string[];
  deliverables?: string[];
  bidding_end_at?: string | null;
  max_bids?: number | null;
  experience_level?: string;
};

export type CreateBidInput = {
  bid_amount?: number | null;
  hourly_rate?: number | null;
  estimated_hours?: number | null;
  proposal: string;
  delivery_timeline?: string;
};

function num(v: string | number | null | undefined): number | null {
  if (v == null || v === "") return null;
  const n = typeof v === "number" ? v : Number(v);
  return Number.isFinite(n) ? n : null;
}

export function formatProjectBudget(p: Project): string {
  const min = num(p.budget_min);
  const max = num(p.budget_max);
  const cur = p.currency || "GBP";
  if (p.project_type === "hourly") {
    if (min != null && max != null) return `${cur} ${min}–${max}/hr`;
    if (min != null) return `${cur} ${min}/hr+`;
    return "Hourly";
  }
  if (min != null && max != null) return `${cur} ${min.toLocaleString()}–${max.toLocaleString()}`;
  if (min != null) return `${cur} ${min.toLocaleString()}+`;
  return "Budget TBD";
}

export function projectTypeLabel(t: ProjectType): string {
  switch (t) {
    case "fixed_price":
      return "Fixed price";
    case "hourly":
      return "Hourly";
    case "retainer":
      return "Retainer";
    case "milestone":
      return "Milestone";
  }
}

export async function listProjects(limit = 24, offset = 0): Promise<Project[]> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  const res = await fetch(`${API_BASE}/api/v1/projects?${params}`, { cache: "no-store" });
  if (res.status === 404) return [];
  if (!res.ok) throw await failure(res, "Failed to load projects");
  const body = (await res.json()) as { data: Project[] };
  return body.data;
}

export async function getProject(id: string): Promise<Project | null> {
  const res = await fetch(`${API_BASE}/api/v1/projects/${encodeURIComponent(id)}`, {
    cache: "no-store",
  });
  if (res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load project");
  return (await res.json()) as Project;
}

export async function createProject(viewer: Viewer, input: CreateProjectInput): Promise<Project> {
  const res = await fetch(`${API_BASE}/api/v1/projects`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify(input),
  });
  if (!res.ok) throw await failure(res, "Failed to create project");
  return (await res.json()) as Project;
}

export async function submitBid(
  viewer: Viewer,
  projectId: string,
  input: CreateBidInput,
): Promise<unknown> {
  const res = await fetch(`${API_BASE}/api/v1/projects/${encodeURIComponent(projectId)}/bids`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify(input),
  });
  if (!res.ok) throw await failure(res, "Failed to submit bid");
  return res.json();
}
