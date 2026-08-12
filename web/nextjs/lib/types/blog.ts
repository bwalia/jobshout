/**
 * Article generator types — mirrors `model.BlogRun` and friends on the server.
 * Field names are the wire format (snake_case); Go pointers become `T | null`.
 */

export type BlogRunStatus = "pending" | "running" | "completed" | "failed";

/** Step keys, in the order the pipeline moves through them. */
export type BlogStepKey =
  | "queued"
  | "generating"
  | "converting"
  | "generated"
  | "publishing"
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

/**
 * A generated article: the markdown, the HTML it converts to, and where it
 * ended up in the CMS once published.
 */
export interface BlogArticle {
  id: string;
  run_id: string;
  org_id: string;
  topic: string;
  slug: string;
  path: string;
  markdown: string;
  /** The body as sent to the CMS. */
  html: string;
  /** The CMS draft this article was posted as; null until published. */
  post_uuid: string | null;
  post_status: string | null;
  posted_at: string | null;
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
  /** The CMS namespace the drafts were created in; null until published. */
  cms_namespace: string | null;
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
 * when the server has no CMS connection configured, in which case articles can
 * still be generated and read — they just cannot be filed as drafts.
 */
export interface BlogConfig {
  can_publish: boolean;
}
