import type { EmploymentType, Job } from "@/lib/api";
import { annualSalary, jobCategory } from "@/lib/format";

export type SortKey = "recent" | "salary" | "title";

export const SORT_OPTIONS: Array<{ value: SortKey; label: string }> = [
  { value: "recent", label: "Most recent" },
  { value: "salary", label: "Highest salary" },
  { value: "title", label: "A–Z" },
];

export interface JobFilterState {
  q: string;
  where: string;
  types: EmploymentType[];
  category: string;
  remote: boolean;
  minSalary: number | null;
  sort: SortKey;
}

type RawParams = Record<string, string | string[] | undefined>;

function one(value: string | string[] | undefined): string {
  return (Array.isArray(value) ? value[0] : value)?.trim() ?? "";
}

/** URL query params are the single source of truth for board state. */
export function parseFilters(params: RawParams): JobFilterState {
  const typesRaw = one(params.type);
  const min = Number.parseInt(one(params.min), 10);
  const sort = one(params.sort) as SortKey;

  return {
    q: one(params.q),
    where: one(params.where),
    types: typesRaw ? (typesRaw.split(",").filter(Boolean) as EmploymentType[]) : [],
    category: one(params.category),
    remote: one(params.remote) === "1",
    minSalary: Number.isFinite(min) && min > 0 ? min : null,
    sort: SORT_OPTIONS.some((o) => o.value === sort) ? sort : "recent",
  };
}

export function hasActiveFilters(f: JobFilterState): boolean {
  return Boolean(
    f.q || f.where || f.types.length || f.category || f.remote || f.minSalary !== null,
  );
}

export function activeFilterCount(f: JobFilterState): number {
  return (
    (f.q ? 1 : 0) +
    (f.where ? 1 : 0) +
    f.types.length +
    (f.category ? 1 : 0) +
    (f.remote ? 1 : 0) +
    (f.minSalary !== null ? 1 : 0)
  );
}

function matchesKeyword(job: Job, q: string): boolean {
  const needle = q.toLowerCase();
  return [job.title, job.summary, job.description, job.requirements.join(" ")]
    .join(" ")
    .toLowerCase()
    .includes(needle);
}

function matchesLocation(job: Job, where: string): boolean {
  const needle = where.toLowerCase();
  // "remote" typed into the location box should behave like the remote filter.
  if (needle === "remote" || needle === "anywhere") return Boolean(job.location.remote);
  const haystack = [job.location.city, job.location.region, job.location.country]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
  return haystack.includes(needle) || (Boolean(job.location.remote) && "remote".includes(needle));
}

export function applyFilters(jobs: Job[], f: JobFilterState): Job[] {
  const filtered = jobs.filter((job) => {
    if (f.q && !matchesKeyword(job, f.q)) return false;
    if (f.where && !matchesLocation(job, f.where)) return false;
    if (f.types.length && !f.types.includes(job.employment_type)) return false;
    if (f.category && jobCategory(job) !== f.category) return false;
    if (f.remote && !job.location.remote) return false;
    if (f.minSalary !== null) {
      const salary = annualSalary(job.compensation);
      // Unlisted salaries drop out once a floor is set — otherwise the filter
      // silently does nothing for the roles the user most wants to exclude.
      if (salary === null || salary < f.minSalary) return false;
    }
    return true;
  });

  const sorted = [...filtered];
  switch (f.sort) {
    case "salary":
      sorted.sort((a, b) => (annualSalary(b.compensation) ?? 0) - (annualSalary(a.compensation) ?? 0));
      break;
    case "title":
      sorted.sort((a, b) => a.title.localeCompare(b.title));
      break;
    default:
      sorted.sort(
        (a, b) =>
          new Date(b.published_at || b.created_at).getTime() -
          new Date(a.published_at || a.created_at).getTime(),
      );
  }
  return sorted;
}

/** Jobs sharing a category, excluding the one being viewed. */
export function similarJobs(jobs: Job[], job: Job, limit = 3): Job[] {
  const category = jobCategory(job);
  return jobs
    .filter((j) => j.id !== job.id && jobCategory(j) === category)
    .slice(0, limit);
}
