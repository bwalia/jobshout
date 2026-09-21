import { apiClient } from "@/lib/api/client";

export type SEORunStatus = "queued" | "running" | "completed" | "failed" | "cancelled";
export type SEOMode = "analyze" | "improve" | "publish";

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
  url: string;
  platform: string;
  detected_cms?: string;
  git_repo?: string;
  git_branch?: string;
  keywords?: string;
  instruction?: string | null;
  score?: SEOScore | null;
  report?: SEOReport | null;
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
  url: string;
  mode: SEOMode | string;
  platform?: string;
  git_repo?: string;
  git_branch?: string;
  sitemap_path?: string;
  keywords?: string;
  instruction?: string;
}

export interface PaginatedSEORuns {
  data: SEORun[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
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
