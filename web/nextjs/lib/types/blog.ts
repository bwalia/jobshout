/**
 * Article generator types — mirrors `model.BlogRun` and friends on the server.
 * Field names are the wire format (snake_case); Go pointers become `T | null`.
 */

export type BlogRunStatus =
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "cancelled";

/** Step keys, in the order the pipeline moves through them. */
export type BlogStepKey =
  | "queued"
  | "discovering"
  | "researching"
  | "outlining"
  | "generating"
  | "reviewing"
  | "revising"
  | "expanding"
  | "illustrating"
  | "converting"
  | "generated"
  | "publishing"
  | "published";

/**
 * `skipped` marks a step that was never needed — a draft the reviewer had no
 * complaints about is never revised. It is distinct from `pending` so a
 * finished run does not look like it stalled.
 */
export type StepStatus =
  | "pending"
  | "running"
  | "done"
  | "failed"
  | "skipped";

/** One entry in a run's progress trace. */
export interface BlogStep {
  key: BlogStepKey;
  label: string;
  /**
   * Which agent performs this step. A run is a collaboration — the Research
   * Agent gathers sources, the Article Writer turns them into a piece — and
   * naming that makes the handover visible. Absent on runs written before the
   * field existed.
   */
  agent?: string;
  status: StepStatus;
  started_at?: string;
  completed_at?: string;
  error?: string;
}

/** One source an article cites. Every reference was retrieved and verified. */
export interface BlogReference {
  url: string;
  title: string;
  site?: string;
  published_at?: string;
}

/** Per-article summary carried on the run. The body is fetched separately. */
export interface BlogRunArticle {
  id: string;
  /** What the agent was asked to write about. */
  topic: string;
  /** What the agent decided to call it, after researching the topic. */
  title: string;
  slug: string;
  path: string;
  word_count: number;
  /** How well-sourced the article is, without loading its references. */
  reference_count: number;
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
  title: string;
  slug: string;
  path: string;
  /** The verified sources this article cites, in citation order. */
  references: BlogReference[];
  markdown: string;
  /** The body as sent to the CMS. */
  html: string;
  /** The CMS draft this article was posted as; null until published. */
  post_uuid: string | null;
  post_status: string | null;
  posted_at: string | null;
  /** The JobShout.com Insights item this article was filed as; null until sent. */
  insights_item_id: string | null;
  insights_slug: string | null;
  insights_status: string | null;
  insights_posted_at: string | null;
  word_count: number;
  created_at: string;
  /** Where the generated cover image lives; absent when the run drew none. */
  cover_image_url?: string;
  /** What the cover was asked for, so a reader can see why it looks as it does. */
  cover_image_prompt?: string;
  /** The settings behind the cover. The seed is what makes it reproducible. */
  cover_image_meta?: {
    provider?: string;
    model?: string;
    seed?: number;
    width?: number;
    height?: number;
  };
}

export interface BlogRunOptions {
  /** The run finds its own topic instead of being given one. */
  trending?: boolean;
  trending_count?: number;
  focus?: string[];
  max_articles?: number;
  auto_publish?: boolean;
  /** The reader and sector the run was asked for; Retry replays these. */
  audience?: string;
  industry?: string;
}

/**
 * A heading for a run. A trending run that failed while choosing its topic has
 * none, and "0 articles" read as a broken card rather than a run that never
 * got as far as picking a subject.
 */
export function blogRunTitle(run: BlogRun): string {
  if (run.topics.length === 1) return run.topics[0];
  const discovers =
    run.options?.trending || run.steps.some((s) => s.key === "discovering");
  if (run.topics.length === 0 && discovers) {
    const focus = run.options?.focus ?? [];
    return focus.length > 0
      ? `Trending in ${focus.join(", ")} — no topic chosen yet`
      : "Trending topic — not chosen yet";
  }
  return `${run.topics.length} articles`;
}

export interface BlogRun {
  id: string;
  org_id: string;
  agent_id: string | null;
  triggered_by: string | null;
  source: "api" | "schedule";
  status: BlogRunStatus;
  /** What the run was asked to write: a topic and its guidance, per article. */
  briefs: BlogBrief[];
  /** The same subjects without their context, kept for older runs. */
  topics: string[];
  model: string | null;
  /** How the run was asked to work; Retry replays these. */
  options?: BlogRunOptions;
  /** The CMS namespace the drafts were created in; null until published. */
  cms_namespace: string | null;
  articles: BlogRunArticle[];
  steps: BlogStep[];
  error_message: string | null;
  started_at: string | null;
  completed_at: string | null;
  published_at: string | null;
  /** When the articles were filed in JobShout.com Insights; independent of the CMS. */
  insights_published_at: string | null;
  created_at: string;
}

/**
 * One article's instructions. The topic is a subject, not a title — the agent
 * researches it and chooses the title from what it finds.
 */
export interface BlogBrief {
  topic: string;
  /** Optional guidance: angle, points to hit, things to avoid. */
  context?: string;
  /**
   * Which reader this piece is written for — an `audience` key from the
   * Article Writer's schema. Omitted means the run's, and a run that names
   * none means the server default (a developer deep dive).
   *
   * The values are NOT enumerated here on purpose: they live in the Go
   * audience registry and reach the UI through GET /agent-schemas, so adding a
   * reader does not mean editing TypeScript.
   */
  audience?: string;
  /** Free-text sector framing, e.g. "NHS trusts". Omitted means none. */
  industry?: string;
}

export interface GenerateBlogRequest {
  briefs: BlogBrief[];
  model?: string;
  max_articles?: number;
  /** The run's default reader and sector; every brief inherits them. */
  audience?: string;
  industry?: string;
}

/**
 * What the UI needs to decide which actions to offer. `can_publish` is false
 * when the server has no CMS connection configured, in which case articles can
 * still be generated and read — they just cannot be filed as drafts.
 */
export interface BlogConfig {
  can_publish: boolean;
  /** Whether JobShout.com Insights is reachable, for "Send to Insights". */
  can_publish_insights: boolean;
  /**
   * The provider the writing pipeline is bound to at startup. The model picker
   * filters on it: the pipeline sends a bare model name to this one provider,
   * so a model from any other provider could only fail once a run started.
   */
  provider: string;
  /** Which model each role uses when neither the agent nor the run names one. */
  effective_models: Partial<Record<ModelRole, string>>;
  /** Which model the benchmark favoured for each role, and why. */
  recommended_models: ModelRecommendation[];
}

/** The two kinds of call the writing pipeline makes. */
export type ModelRole = "prose" | "structured";

/**
 * A measured suggestion for one role.
 *
 * Advice, not a default — the picker labels the suggested model, it does not
 * select it. `model` may name something this deployment has never pulled, in
 * which case no badge is shown rather than a recommendation you cannot take.
 */
export interface ModelRecommendation {
  role: ModelRole;
  /** What the setting is called in the UI. */
  label: string;
  model: string;
  reason: string;
  /** The cost of taking the advice, where there is one. */
  caveat?: string;
  /** Which calls the setting governs. */
  covers: string;
}
