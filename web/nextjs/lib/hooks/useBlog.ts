"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { PaginationParams } from "@/lib/types/common";
import type { GenerateBlogRequest } from "@/lib/types/blog";
import {
  generateBlog,
  getBlogArticles,
  getBlogConfig,
  getBlogRun,
  getBlogRuns,
  publishBlogRun,
  retryBlogRun,
  cancelBlogRun,
  deleteBlogRun,
} from "@/lib/api/blog";
import { apiErrorMessage } from "@/lib/api/client";

export const blogKeys = {
  all: ["blogs"] as const,
  config: () => [...blogKeys.all, "config"] as const,
  lists: () => [...blogKeys.all, "list"] as const,
  list: (params: PaginationParams) => [...blogKeys.lists(), params] as const,
  details: () => [...blogKeys.all, "detail"] as const,
  detail: (id: string) => [...blogKeys.details(), id] as const,
  articles: (runId: string) => [...blogKeys.all, "articles", runId] as const,
};

/** Whether the CMS connection is configured on this deployment. */
export function useBlogConfig() {
  return useQuery({
    queryKey: blogKeys.config(),
    queryFn: getBlogConfig,
    // Deployment-level configuration; it will not change while the tab is open.
    staleTime: Infinity,
  });
}

/**
 * The run list, polled while any run is still working so the list reflects
 * progress without the user refreshing.
 */
export function useBlogRuns(params: PaginationParams = {}) {
  return useQuery({
    queryKey: blogKeys.list(params),
    queryFn: () => getBlogRuns(params),
    refetchInterval: (query) => {
      const runs = query.state.data?.data;
      const hasActive = runs?.some(
        (r: { status: string }) => r.status === "running" || r.status === "pending"
      );
      return hasActive ? 2000 : false;
    },
  });
}

/** A single run, polled while it is in flight so the step trace advances live. */
export function useBlogRun(id: string) {
  return useQuery({
    queryKey: blogKeys.detail(id),
    queryFn: () => getBlogRun(id),
    enabled: Boolean(id),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "running" || status === "pending" ? 2000 : false;
    },
  });
}

/**
 * Article bodies for a run. Only fetched once the run has produced them —
 * there is nothing to load while it is still writing.
 */
export function useBlogArticles(runId: string, enabled = true) {
  return useQuery({
    queryKey: blogKeys.articles(runId),
    queryFn: () => getBlogArticles(runId),
    enabled: Boolean(runId) && enabled,
  });
}

export function useGenerateBlog() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: GenerateBlogRequest) => generateBlog(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: blogKeys.lists() });
      toast.success("Article Writer is working on it");
    },
    onError: (e) =>
      toast.error(apiErrorMessage(e, "Failed to start generation")),
  });
}

export function usePublishBlogRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => publishBlogRun(id),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: blogKeys.detail(id) });
      qc.invalidateQueries({ queryKey: blogKeys.lists() });
      toast.success("Filed in the CMS as a draft");
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Failed to publish")),
  });
}

export function useRetryBlogRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => retryBlogRun(id),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: blogKeys.detail(id) });
      qc.invalidateQueries({ queryKey: blogKeys.lists() });
      toast.success("Article Writer is trying again");
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Failed to retry")),
  });
}

export function useCancelBlogRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => cancelBlogRun(id),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: blogKeys.detail(id) });
      qc.invalidateQueries({ queryKey: blogKeys.lists() });
      toast.success("Run cancelled");
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Failed to cancel")),
  });
}

export function useDeleteBlogRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteBlogRun(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: blogKeys.lists() });
      toast.success("Run deleted");
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Failed to delete")),
  });
}
