import type {
  ApplicationStatus,
  Compensation,
  EmploymentType,
  Job,
  Location,
} from "@/lib/api";

export const EMPLOYMENT_LABELS: Record<EmploymentType, string> = {
  permanent: "Permanent",
  contract: "Contract",
  freelance: "Freelance",
  temporary: "Temporary",
  part_time: "Part time",
  internship: "Internship",
  apprenticeship: "Apprenticeship",
};

export const APPLICATION_LABELS: Record<ApplicationStatus, string> = {
  submitted: "Submitted",
  reviewing: "In review",
  shortlisted: "Shortlisted",
  interviewing: "Interviewing",
  offered: "Offer",
  rejected: "Not moving forward",
  withdrawn: "Withdrawn",
};

const CURRENCY_SYMBOLS: Record<string, string> = {
  GBP: "£",
  USD: "$",
  EUR: "€",
  INR: "₹",
  AUD: "A$",
  CAD: "C$",
};

export function employmentLabel(t: EmploymentType): string {
  return EMPLOYMENT_LABELS[t] ?? t.replaceAll("_", " ");
}

function money(amount: number, currency?: string | null): string {
  const symbol = currency ? (CURRENCY_SYMBOLS[currency] ?? "") : "";
  const rounded = Math.round(amount);
  // Six figures read better abbreviated in dense listing rows.
  const value =
    rounded >= 10000 && rounded % 1000 === 0
      ? `${rounded / 1000}k`
      : rounded.toLocaleString("en-GB");
  return symbol ? `${symbol}${value}` : `${currency ?? ""} ${value}`.trim();
}

export function formatCompensation(c: Compensation): string {
  if (!c.min_amount && !c.max_amount) return "Salary not listed";
  const period = c.period ? `/${c.period === "annual" ? "yr" : c.period.replace(/ly$/, "")}` : "";
  if (c.min_amount && c.max_amount && c.min_amount !== c.max_amount) {
    return `${money(c.min_amount, c.currency)}–${money(c.max_amount, c.currency)}${period}`;
  }
  return `${money((c.min_amount ?? c.max_amount) as number, c.currency)}${period}`;
}

export function hasSalary(c: Compensation): boolean {
  return Boolean(c.min_amount || c.max_amount);
}

/** Lowest listed figure, normalised to an annual number for sorting/filtering. */
export function annualSalary(c: Compensation): number | null {
  const base = c.min_amount ?? c.max_amount;
  if (!base) return null;
  switch (c.period) {
    case "hourly":
      return base * 8 * 220;
    case "daily":
      return base * 220;
    case "weekly":
      return base * 46;
    case "monthly":
      return base * 12;
    default:
      return base;
  }
}

export function formatLocation(loc: Location): string {
  const parts = [loc.city, loc.region, loc.country].filter(Boolean);
  return parts.join(", ") || loc.country || "Location not listed";
}

/** City-first short form for cards; "Remote" is surfaced as its own badge. */
export function shortLocation(loc: Location): string {
  return loc.city || loc.region || loc.country || "Anywhere";
}

export function relativeTime(iso?: string | null): string {
  if (!iso) return "Recently";
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "Recently";
  const days = Math.floor((Date.now() - then) / 86_400_000);
  if (days <= 0) return "Today";
  if (days === 1) return "Yesterday";
  if (days < 7) return `${days} days ago`;
  if (days < 14) return "Last week";
  if (days < 60) return `${Math.floor(days / 7)} weeks ago`;
  return `${Math.floor(days / 30)} months ago`;
}

/** Posted within the last week — drives the "New" badge. */
export function isFresh(job: Job): boolean {
  const iso = job.published_at || job.created_at;
  const then = new Date(iso).getTime();
  return !Number.isNaN(then) && Date.now() - then < 7 * 86_400_000;
}

/**
 * Group a job into a browsable family from its title and requirements.
 * The API has no category field, so this is derived — keep the buckets broad.
 */
const CATEGORY_RULES: Array<{ name: string; test: RegExp }> = [
  { name: "Engineering", test: /engineer|developer|programmer|architect|sre|devops|platform/i },
  { name: "Data & AI", test: /data|machine learning|\bml\b|\bai\b|scientist|analytics/i },
  { name: "Design", test: /design|\bux\b|\bui\b|research|brand/i },
  { name: "Product", test: /product|program manager|\bpm\b|delivery|scrum/i },
  { name: "Security", test: /security|infosec|compliance|risk|penetration/i },
  { name: "Sales & Marketing", test: /sales|marketing|growth|account|revenue|content/i },
  { name: "Operations", test: /operations|support|success|finance|people|recruit|\bhr\b/i },
];

export function jobCategory(job: Job): string {
  const haystack = `${job.title} ${job.requirements.join(" ")}`;
  return CATEGORY_RULES.find((r) => r.test.test(haystack))?.name ?? "Other";
}

export const CATEGORY_NAMES = [...CATEGORY_RULES.map((r) => r.name), "Other"];

/** Two-letter mark used in place of a company logo the API does not have. */
export function initials(value: string): string {
  const words = value.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return "JS";
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase();
  return (words[0][0] + words[1][0]).toUpperCase();
}
