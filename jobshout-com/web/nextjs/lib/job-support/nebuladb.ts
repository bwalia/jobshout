/**
 * Cluster NebulaDB client for Job Support.
 * Auth is optional (NEBULA_API_KEY). LLM + embeddings stay on our Ollama —
 * never OpenAI/Anthropic from this path.
 */

import { redactPii } from "./pii";

export const JOBS_BUCKET = "jobshout_jobs";
export const CANDIDATES_BUCKET = "jobshout_candidates";

type Json = Record<string, unknown>;

function config(): { base: string; apiKey: string | null } | null {
  const base = process.env.NEBULA_URL?.trim().replace(/\/$/, "");
  if (!base) return null;
  return { base, apiKey: process.env.NEBULA_API_KEY?.trim() || null };
}

export function nebuladbEnabled(): boolean {
  return config() !== null;
}

async function nebula<T>(
  path: string,
  init: RequestInit & { timeoutMs?: number } = {},
): Promise<T> {
  const cfg = config();
  if (!cfg) throw new Error("NEBULA_URL is not configured");
  const headers = new Headers(init.headers);
  headers.set("content-type", "application/json");
  headers.set("accept", "application/json");
  if (cfg.apiKey) headers.set("authorization", `Bearer ${cfg.apiKey}`);

  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), init.timeoutMs ?? 90_000);
  try {
    const res = await fetch(`${cfg.base}${path}`, {
      ...init,
      headers,
      signal: ctrl.signal,
      cache: "no-store",
    });
    const text = await res.text();
    if (!res.ok) {
      throw new Error(`NebulaDB ${res.status}: ${text.slice(0, 240)}`);
    }
    return (text ? JSON.parse(text) : {}) as T;
  } finally {
    clearTimeout(t);
  }
}

export async function chat(params: {
  message: string;
  system: string;
  temperature?: number;
}): Promise<{ answer: string; model: string; provider: string }> {
  const body = await nebula<{
    answer?: string;
    model?: string;
    provider?: string;
  }>("/api/v1/ai/chat", {
    method: "POST",
    body: JSON.stringify({
      message: params.message,
      system: params.system,
      stream: false,
      temperature: params.temperature ?? 0.3,
      max_tokens: 1024,
    }),
    timeoutMs: 120_000,
  });
  return {
    answer: (body.answer ?? "").trim(),
    model: body.model ?? "unknown",
    provider: body.provider ?? "unknown",
  };
}

export async function upsertDoc(
  bucket: string,
  id: string,
  text: string,
  metadata: Json = {},
): Promise<void> {
  const clean = redactPii(text);
  if (!clean) return;

  await nebula(`/api/v1/bucket/${encodeURIComponent(bucket)}/document`, {
    method: "POST",
    body: JSON.stringify({
      doc_id: id,
      text: clean,
      metadata: { ...metadata, pii_redacted: true, source: "jobshout-com" },
    }),
  });
}

export type SearchHit = {
  id: string;
  text: string;
  score: number;
  bucket?: string;
  metadata?: Json;
};

export async function semanticSearch(
  query: string,
  bucket: string,
  topK = 8,
): Promise<SearchHit[]> {
  const clean = redactPii(query);
  if (!clean) return [];
  const body = await nebula<{
    hits?: Array<{
      id?: string;
      text?: string;
      score?: number;
      bucket?: string;
      metadata?: Json;
    }>;
    results?: Array<{
      id?: string;
      text?: string;
      score?: number;
      bucket?: string;
      metadata?: Json;
    }>;
  }>("/api/v1/ai/search", {
    method: "POST",
    body: JSON.stringify({ query: clean, top_k: topK, bucket, hybrid: true }),
    timeoutMs: 60_000,
  });
  const rows = body.hits ?? body.results ?? [];
  return rows
    .filter((h) => h.id && h.text)
    .map((h) => ({
      id: String(h.id),
      text: String(h.text),
      score: Number(h.score ?? 0),
      bucket: h.bucket,
      metadata: h.metadata,
    }));
}

/** Index a published marketplace job (already public copy — still redact). */
export async function indexJob(job: {
  id: string;
  title: string;
  summary: string;
  description: string;
  requirements?: string[];
  employment_type?: string;
  location?: { city?: string | null; country?: string; remote?: boolean };
}): Promise<void> {
  const loc = job.location;
  const place = [loc?.city, loc?.country, loc?.remote ? "remote" : null]
    .filter(Boolean)
    .join(", ");
  const text = [
    job.title,
    job.summary,
    job.description,
    (job.requirements ?? []).join(", "),
    job.employment_type,
    place,
  ]
    .filter(Boolean)
    .join("\n");
  await upsertDoc(JOBS_BUCKET, `job_${job.id}`, text, {
    kind: "marketplace_job",
    job_id: job.id,
    title: job.title,
    employment_type: job.employment_type ?? null,
  });
}
