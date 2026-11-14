import type { EmploymentType, Job } from "@/lib/api";

/** The post-a-job form, as strings, shared by the new and edit pages. */
export type JobFormValues = {
  title: string;
  summary: string;
  description: string;
  employment_type: EmploymentType;
  country: string;
  city: string;
  region: string;
  remote: boolean;
  currency: string;
  min_amount: string;
  max_amount: string;
  period: string;
  requirements: string;
};

export const BLANK_JOB_FORM: JobFormValues = {
  title: "",
  summary: "",
  description: "",
  employment_type: "permanent",
  country: "GB",
  city: "",
  region: "",
  remote: true,
  currency: "GBP",
  min_amount: "",
  max_amount: "",
  period: "annual",
  requirements: "",
};

/** Turn a saved job back into form values for the edit page. */
export function jobToFormValues(job: Job): JobFormValues {
  const amount = (n: number | null | undefined) => (n == null ? "" : String(n));
  return {
    title: job.title,
    summary: job.summary ?? "",
    description: job.description,
    employment_type: job.employment_type,
    country: job.location.country,
    city: job.location.city ?? "",
    region: job.location.region ?? "",
    remote: job.location.remote ?? false,
    currency: job.compensation?.currency ?? "GBP",
    min_amount: amount(job.compensation?.min_amount),
    max_amount: amount(job.compensation?.max_amount),
    period: job.compensation?.period ?? "annual",
    requirements: (job.requirements ?? []).join("\n"),
  };
}
