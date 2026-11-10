import { apiClient } from "@/lib/api/client";

export type SEORunStatus = "queued" | "running" | "completed" | "failed" | "cancelled";
export type SEOMode = "analyze" | "improve" | "publish";
export type SEOSource = "urls" | "github";

export interface SEOScore {
  overall: number;
  title_ok: boolean;
  meta_desc_ok: boolean;
  h1_ok: boolean;
  canonical_ok: boolean;
  robots_ok: boolean;
  sitemap_ok: boolean;
  open_graph_ok: boolean;
  https_ok: boolean;
  by_category?: Record<string, number>;
  issue_count: number;
  critical_count: number;
}

export interface SEOIssue {
  id: string;
  severity: string;
  category: string;
  title: string;
  detail: string;
  fix_hint?: string;
}

export interface SEOSuggestion {
  id: string;
  kind: string;
  title: string;
  detail: string;
  proposed?: string;
}

export interface SEOPageReport {
  path: string;
  source_url?: string;
  status: string;
  error?: string;
  title?: string;
  meta_description?: string;
  h1?: string[];
  keywords_found?: string[];
  score?: SEOScore | null;
  issues?: SEOIssue[];
  suggestions?: SEOSuggestion[];
  meta_patch?: Record<string, string>;
  proposed_diff?: string;
}

export interface SEOProgress {
  total: number;
  completed: number;
  current?: string;
  phase?: string;
}

export interface SEOReport {
  final_url: string;
  status_code: number;
  title: string;
  meta_description: string;
  canonical: string;
  h1: string[];
  h2?: string[];
  keywords_found?: string[];
  robots_txt?: string;
  sitemap_url?: string;
  sitemap_found: boolean;
  open_graph?: Record<string, string>;
  issues: SEOIssue[];
  suggestions: SEOSuggestion[];
  proposed_slug?: string;
  sitemap_xml?: string;
  meta_patch?: Record<string, string>;
  page_count?: number;
  keywords?: string[];
  rollup_summary?: string;
  site_structure?: {
    content_root?: string;
    public_root?: string;
    template_hint?: string;
    pages?: string[];
    has_sitemap?: boolean;
    skill_notes?: string[];
  };
}

export interface SEOPublishResult {
  channel: string;
  success: boolean;
  message: string;
  pr_url?: string;
  pr_number?: number;
  remote_ref?: string;
  fallback?: string;
}

export interface SEORun {
  id: string;
  agent_id: string;
  task_id?: string | null;
  org_id: string;
  status: SEORunStatus;
  mode: SEOMode | string;
  source?: SEOSource | string;
  url: string;
  platform: string;
  detected_cms?: string;
  git_repo?: string;
  git_branch?: string;
  github_connection?: string;
  path_glob?: string;
  max_pages?: number;
  keywords?: string;
  instruction?: string | null;
  score?: SEOScore | null;
  report?: SEOReport | null;
  pages?: SEOPageReport[];
  progress?: SEOProgress | null;
  publish?: SEOPublishResult | null;
  error_message?: string | null;
  requested_by?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateSEORunRequest {
  agent_id: string;
  task_id?: string;
  source?: SEOSource | string;
  url?: string;
  urls?: string;
  mode: SEOMode | string;
  platform?: string;
  git_repo?: string;
  git_branch?: string;
  github_connection?: string;
  path_glob?: string;
  max_pages?: number;
  sitemap_path?: string;
  keywords: string;
  instruction?: string;
}

export interface SEOGitHubConnection {
  id: string;
  name: string;
  repo: string;
  default_branch: string;
  connected_by?: string | null;
  created_at: string;
  updated_at: string;
}

export interface SEOStatus {
  can_seal_tokens: boolean;
  connections: boolean;
}

export interface PaginatedSEORuns {
  data: SEORun[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export async function getSEOStatus() {
  const { data } = await apiClient.get<SEOStatus>("/seo/status");
  return data;
}

export async function listSEOConnections() {
  const { data } = await apiClient.get<{ data: SEOGitHubConnection[] }>("/seo/connections");
  return data.data ?? [];
}

export async function upsertSEOConnection(body: {
  name: string;
  repo: string;
  default_branch?: string;
  access_token: string;
}) {
  const { data } = await apiClient.put<SEOGitHubConnection>("/seo/connections", body);
  return data;
}

export async function deleteSEOConnection(name: string) {
  const { data } = await apiClient.delete<{ status: string }>(
    `/seo/connections/${encodeURIComponent(name)}`
  );
  return data;
}

export async function listSEORuns(page = 1, perPage = 10) {
  const { data } = await apiClient.get<PaginatedSEORuns>("/seo/runs", {
    params: { page, per_page: perPage },
  });
  return data;
}

export async function createSEORun(body: CreateSEORunRequest) {
  const { data } = await apiClient.post<SEORun>("/seo/runs", body);
  return data;
}

export async function getSEORun(runID: string) {
  const { data } = await apiClient.get<SEORun>(`/seo/runs/${runID}`);
  return data;
}

export async function cancelSEORun(runID: string) {
  const { data } = await apiClient.post<SEORun>(`/seo/runs/${runID}/cancel`);
  return data;
}
