import { apiClient } from "@/lib/api/client";
import type { PaginatedResponse, PaginationParams } from "@/lib/types/common";
import type {
  BlogArticle,
  BlogConfig,
  BlogRun,
  GenerateBlogRequest,
} from "@/lib/types/blog";

export async function getBlogConfig(): Promise<BlogConfig> {
  const { data } = await apiClient.get<BlogConfig>("/blogs/config");
  return data;
}

export async function getBlogRuns(
  params: PaginationParams = {}
): Promise<PaginatedResponse<BlogRun>> {
  const { data } = await apiClient.get("/blogs/runs", { params });
  return data;
}

export async function getBlogRun(id: string): Promise<BlogRun> {
  const { data } = await apiClient.get<BlogRun>(`/blogs/runs/${id}`);
  return data;
}

/**
 * Starts a run. The server responds 202 with the pending record — generation
 * continues in the background, so poll the run for progress.
 */
export async function generateBlog(
  payload: GenerateBlogRequest
): Promise<BlogRun> {
  const { data } = await apiClient.post<BlogRun>("/blogs/generate", payload);
  return data;
}

/** Files a completed run's articles in the CMS as drafts. */
export async function publishBlogRun(id: string): Promise<BlogRun> {
  const { data } = await apiClient.post<BlogRun>(`/blogs/runs/${id}/publish`);
  return data;
}

export async function getBlogArticles(runId: string): Promise<BlogArticle[]> {
  const { data } = await apiClient.get<BlogArticle[]>(
    `/blogs/runs/${runId}/articles`
  );
  return data ?? [];
}

export async function getBlogArticle(id: string): Promise<BlogArticle> {
  const { data } = await apiClient.get<BlogArticle>(`/blogs/articles/${id}`);
  return data;
}
