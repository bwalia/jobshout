import type { CareerApplication, CareerEvaluation, CareerPipelineItem } from "@/types/career";

export type CareerJob = {
  key: string;
  company: string;
  role: string;
  listing_url: string;
  score?: number | null;
  status?: string;
  liveness?: string;
  source?: string;
  application?: CareerApplication;
  pipeline?: CareerPipelineItem;
  evaluation?: CareerEvaluation;
};

export function mergeJobs(
  pipeline: CareerPipelineItem[],
  tracker: CareerApplication[],
  evals: CareerEvaluation[]
): CareerJob[] {
  const byKey = new Map<string, CareerJob>();

  const keyOf = (url: string, fallback: string) => (url.trim() ? url.trim() : fallback);

  const upsert = (partial: Partial<CareerJob> & { key: string }) => {
    const prev = byKey.get(partial.key) ?? {
      key: partial.key,
      company: "",
      role: "",
      listing_url: "",
    };
    byKey.set(partial.key, {
      ...prev,
      ...partial,
      company: partial.company || prev.company,
      role: partial.role || prev.role,
      listing_url: partial.listing_url || prev.listing_url,
      score: partial.score ?? prev.score,
      status: partial.status || prev.status,
      liveness: partial.liveness || prev.liveness,
      source: partial.source || prev.source,
      application: partial.application ?? prev.application,
      pipeline: partial.pipeline ?? prev.pipeline,
      evaluation: partial.evaluation ?? prev.evaluation,
    });
  };

  for (const p of pipeline) {
    upsert({
      key: keyOf(p.listing_url, `pipe:${p.id}`),
      company: p.company,
      role: p.title,
      listing_url: p.listing_url,
      liveness: p.liveness,
      source: p.source,
      pipeline: p,
    });
  }
  for (const a of tracker) {
    upsert({
      key: keyOf(a.listing_url, `app:${a.id}`),
      company: a.company,
      role: a.role,
      listing_url: a.listing_url,
      score: a.score,
      status: a.status,
      application: a,
    });
  }
  for (const e of evals) {
    const key = keyOf(
      e.listing_url,
      e.application_id ? `app:${e.application_id}` : `eval:${e.id}`
    );
    upsert({
      key,
      company: e.company,
      role: e.role,
      listing_url: e.listing_url,
      score: e.score?.overall,
      evaluation: e,
    });
  }

  return Array.from(byKey.values()).sort((a, b) => {
    const as = a.score ?? -1;
    const bs = b.score ?? -1;
    if (as !== bs) return bs - as;
    return `${a.company}${a.role}`.localeCompare(`${b.company}${b.role}`);
  });
}

export function postingHref(url: string): string | null {
  const raw = url.trim();
  if (!raw) return null;
  try {
    const parsed = new URL(raw);
    if (parsed.protocol === "http:" || parsed.protocol === "https:") {
      return parsed.toString();
    }
  } catch {
    return null;
  }
  return null;
}
