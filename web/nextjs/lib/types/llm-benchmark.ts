/**
 * LLM benchmark records: one row per text LLM call, metadata only. Token
 * counts are null when the provider did not report them — unknown, not zero.
 */
export interface LLMCall {
  id: string;
  agent_id?: string;
  task_id?: string;
  task_run_id?: string;
  execution_id?: string;
  run_kind: string;
  run_id: string;
  stage: string;
  attempt: number;
  /** The concrete client that made the call. */
  provider: string;
  /** The model the provider's reply named when model_reported, else the model sent. */
  model: string;
  model_reported: boolean;
  /** The model sent to the provider. */
  requested_model: string;
  /** The whole call, retries and backoff included. */
  duration_ms: number;
  /** Only the HTTP exchanges with the provider; null when not measured. */
  api_duration_ms: number | null;
  /** HTTP requests actually sent (first try + transport retries); null when not reported. */
  api_attempt_count: number | null;
  api_attempts: LLMCallAttempt[];
  input_tokens: number | null;
  output_tokens: number | null;
  /** The provider's own total; null when it reports none (never computed). */
  total_tokens: number | null;
  reasoning_tokens: number | null;
  provider_request_id: string | null;
  retries: number;
  status: "success" | "failed" | "cancelled" | string;
  error?: string;
  started_at: string | null;
  completed_at: string | null;
}

/** One HTTP request a call actually sent to its provider. */
export interface LLMCallAttempt {
  started_at: string;
  duration_ms: number;
  /** Absent when no response arrived (connection error, timeout). */
  http_status?: number;
  request_id?: string;
  error?: string;
}

export interface LLMModelStat {
  provider: string;
  model: string;
  stage?: string | null;
  calls: number;
  runs: number;
  successes: number;
  failures: number;
  cancelled: number;
  avg_duration_ms: number;
  min_duration_ms: number;
  max_duration_ms: number;
  p50_duration_ms: number;
  p95_duration_ms: number;
  input_tokens: number | null;
  output_tokens: number | null;
  total_tokens: number | null;
  calls_with_usage: number;
  avg_input_tokens: number | null;
  avg_output_tokens: number | null;
  retries: number;
  retried_calls: number;
}

export interface LLMRunSummary {
  run_kind: string;
  run_id: string;
  agent_id?: string;
  agent_name: string;
  models: string[];
  call_count: number;
  failures: number;
  retries: number;
  llm_time_ms: number;
  input_tokens: number | null;
  output_tokens: number | null;
  first_call_at: string | null;
  last_call_at: string | null;
  run_status: string;
  /** The run's own wall-clock time from its run table; separate from call time. */
  run_duration_ms: number | null;
}

export interface LLMRunDetail extends LLMRunSummary {
  calls: LLMCall[];
}

export interface LLMBenchmarkQuery {
  from?: string;
  to?: string;
  run_kind?: string;
  provider?: string;
  model?: string;
  stage?: string;
}
