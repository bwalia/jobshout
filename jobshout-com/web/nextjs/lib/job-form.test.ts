import { describe, expect, it } from "vitest";
import type { Job } from "@/lib/api";
import { BLANK_JOB_FORM, jobToFormValues } from "@/lib/job-form";

const job: Job = {
  id: "j1",
  organisation_id: "o1",
  title: "Platform Engineer",
  summary: "Run the cluster.",
  description: "A long enough description of the role for candidates.",
  employment_type: "contract",
  location: { country: "GB", city: "Leeds", region: null, remote: false },
  compensation: { currency: "EUR", min_amount: 500, max_amount: null, period: "daily" },
  requirements: ["Kubernetes", "Helm"],
  status: "draft",
  created_at: "2026-11-14T00:00:00Z",
  updated_at: "2026-11-14T00:00:00Z",
  published_at: null,
};

describe("jobToFormValues", () => {
  it("prefills every field the edit form shows", () => {
    expect(jobToFormValues(job)).toEqual({
      title: "Platform Engineer",
      summary: "Run the cluster.",
      description: "A long enough description of the role for candidates.",
      employment_type: "contract",
      country: "GB",
      city: "Leeds",
      region: "",
      remote: false,
      currency: "EUR",
      min_amount: "500",
      max_amount: "",
      period: "daily",
      requirements: "Kubernetes\nHelm",
    });
  });

  it("falls back to the blank form defaults for missing pay", () => {
    const bare = { ...job, compensation: undefined } as unknown as Job;
    const v = jobToFormValues(bare);
    expect(v.currency).toBe(BLANK_JOB_FORM.currency);
    expect(v.period).toBe(BLANK_JOB_FORM.period);
    expect(v.min_amount).toBe("");
  });
});
