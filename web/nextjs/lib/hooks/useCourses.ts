"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  cancelCourseRun,
  getCourseRun,
  listCourseChapters,
  listCourseRuns,
} from "@/lib/api/courses";
import { apiErrorMessage } from "@/lib/api/client";
import type { PaginationParams } from "@/lib/types/common";
import { isCourseRunActive, type CourseRun } from "@/lib/types/course";

export const courseKeys = {
  all: ["courses"] as const,
  list: (params: PaginationParams) => [...courseKeys.all, "list", params] as const,
  detail: (id: string) => [...courseKeys.all, "detail", id] as const,
  chapters: (id: string) => [...courseKeys.all, "chapters", id] as const,
};

/** Run list, polled while any run is still working. */
export function useCourseRuns(params: PaginationParams = {}) {
  return useQuery({
    queryKey: courseKeys.list(params),
    queryFn: () => listCourseRuns(params),
    refetchInterval: (query) =>
      query.state.data?.data?.some((r: CourseRun) => isCourseRunActive(r.status)) ? 3000 : false,
  });
}

/** One run, polled while in flight so steps advance live. */
export function useCourseRun(id: string) {
  return useQuery({
    queryKey: courseKeys.detail(id),
    queryFn: () => getCourseRun(id),
    enabled: Boolean(id),
    refetchInterval: (query) =>
      query.state.data && isCourseRunActive(query.state.data.status) ? 3000 : false,
  });
}

/** Chapters appear one at a time as they are written; poll while the run is active. */
export function useCourseChapters(id: string, active: boolean) {
  return useQuery({
    queryKey: courseKeys.chapters(id),
    queryFn: () => listCourseChapters(id),
    enabled: Boolean(id),
    refetchInterval: active ? 5000 : false,
  });
}

export function useCancelCourseRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: cancelCourseRun,
    onSuccess: () => {
      toast.success("Course run cancelled");
      void qc.invalidateQueries({ queryKey: courseKeys.all });
    },
    onError: (err) => toast.error(apiErrorMessage(err, "Failed to cancel course run")),
  });
}
