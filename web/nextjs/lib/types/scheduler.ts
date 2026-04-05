export interface ScheduledTask {
  id: string;
  org_id: string;
  name: string;
  description: string | null;
  task_type: "agent" | "workflow";
  agent_id: string | null;
  workflow_id: string | null;
  input_prompt: string;
  input_json: Record<string, unknown>;
  provider_config_id: string | null;
  model_override: string | null;
  schedule_type: "cron" | "interval" | "once";
  cron_expression: string | null;
  interval_seconds: number | null;
  run_at: string | null;
  status: "active" | "paused" | "completed" | "failed";
  last_run_at: string | null;
  next_run_at: string | null;
  run_count: number;
  max_runs: number | null;
  retry_on_failure: boolean;
  max_retries: number;
  priority: "low" | "medium" | "high" | "critical";
  tags: string[];
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateScheduledTaskRequest {
  name: string;
  description?: string;
  task_type: "agent" | "workflow";
  agent_id?: string;
  workflow_id?: string;
  input_prompt: string;
  input_json?: Record<string, unknown>;
  provider_config_id?: string;
  model_override?: string;
  schedule_type: "cron" | "interval" | "once";
  cron_expression?: string;
  interval_seconds?: number;
  run_at?: string;
  max_runs?: number;
  retry_on_failure?: boolean;
  max_retries?: number;
  priority?: "low" | "medium" | "high" | "critical";
  tags?: string[];
}

export interface UpdateScheduledTaskRequest {
  name?: string;
  description?: string;
  input_prompt?: string;
  cron_expression?: string;
  interval_seconds?: number;
  status?: "active" | "paused" | "completed";
  max_runs?: number;
  priority?: "low" | "medium" | "high" | "critical";
  model_override?: string;
}

export interface ScheduledTaskRun {
  id: string;
  scheduled_task_id: string;
  execution_id: string | null;
  workflow_run_id: string | null;
  status: "pending" | "running" | "completed" | "failed";
  output: string | null;
  error_message: string | null;
  started_at: string | null;
  completed_at: string | null;
  created_at: string;
}
