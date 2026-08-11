/**
 * Article generator types — mirrors `model.BlogRun` and friends on the server.
 * Field names are the wire format (snake_case); Go pointers become `T | null`.
 */

export type BlogRunStatus = "pending" | "running" | "completed" | "failed";

/** Step keys, in the order the pipeline moves through them. */
export type BlogStepKey =
  | "queued"
  | "generating"
  | "generated"
  | "publishing"
  | "opening_pr"
  | "published";

export type StepStatus = "pending" | "running" | "done" | "failed";

/** One entry in a run's progress trace. */
export interface BlogStep {
  key: BlogStepKey;
  label: string;
  status: StepStatus;
  started_at?: string;
  completed_at?: string;
  error?: string;
}

/** Per-article summary carried on the run. The body is fetched separately. */
export interface BlogRunArticle {
  id: string;
  topic: string;
  slug: string;
  path: string;
  word_count: number;
}

/** A generated article including its markdown body. */
export interface BlogArticle {
  id: string;
  run_id: string;
  org_id: string;
  topic: string;
  slug: string;
  path: string;
  markdown: string;
  word_count: number;
  created_at: string;
}

export interface BlogRun {
  id: string;
  org_id: string;
  agent_id: string | null;
  triggered_by: string | null;
  source: "api" | "schedule";
  status: BlogRunStatus;
  topics: string[];
  model: string | null;
  branch: string | null;
  pr_number: number | null;
  pr_url: string | null;
  articles: BlogRunArticle[];
  steps: BlogStep[];
  error_message: string | null;
  started_at: string | null;
  completed_at: string | null;
  published_at: string | null;
  created_at: string;
}

export interface GenerateBlogRequest {
  topics: string[];
  model?: string;
  max_articles?: number;
}

/**
 * What the UI needs to decide which actions to offer. `can_publish` is false
 * when the server has no GitHub token, in which case articles can still be
 * generated and read — they just cannot be pushed to the content repository.
 */
export interface BlogConfig {
  can_publish: boolean;
}
