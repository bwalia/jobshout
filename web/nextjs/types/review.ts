export type ReviewRunStatus = 'queued' | 'running' | 'completed' | 'failed';

export interface ReviewFinding {
  severity: string;
  category: string;
  file: string;
  line: number;
  title: string;
  reason: string;
  suggestion: string;
  is_critical: boolean;
}

export interface ReviewResult {
  pr_number?: number;
  pr_title?: string;
  head_sha?: string;
  decision?: string;
  verdict?: string;
  summary?: string;
  blocking?: ReviewFinding[];
  inline?: ReviewFinding[];
  orphaned?: ReviewFinding[];
  url?: string | null;
  already_started?: boolean;
  nothing_to_review?: boolean;
  detail?: string;
}

export interface ReviewRun {
  id: string;
  org_id: string;
  agent_id?: string;
  repo: string;
  pr_number: number;
  dry_run: boolean;
  force: boolean;
  status: ReviewRunStatus;
  remote_job_id?: string;
  head_sha?: string;
  decision?: string;
  verdict?: string;
  summary?: string;
  github_url?: string;
  result?: ReviewResult;
  stage_log?: string[];
  error_message?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateReviewRunRequest {
  repo: string;
  pr_number: number;
  dry_run?: boolean;
  force?: boolean;
  agent_id?: string;
}

export interface PaginatedReviewRuns {
  data: ReviewRun[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface ReviewRepos {
  enabled: boolean;
  allowed: string[];
}
