/**
 * LLM benchmark records: one row per text LLM call, metadata only. Token
 * counts are null when the provider did not report them — unknown, not zero.
 */
export interface LLMCall {
  id: string;
  agent_id?: string;
  task_id?: string;
  run_kind: string;
  run_id: string;
  stage: string;
  attempt: number;
  provider: string;
  model: string;
  requested_model: string;
  duration_ms: number;
  input_tokens: number | null;
  output_tokens: number | null;
  total_tokens: number | null;
  retries: number;
  status: "success" | "error" | "cancelled" | string;
  error?: string;
  started_at: string | null;
  completed_at: string | null;
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
