import { apiClient } from "@/lib/api/client";
import type { PaginatedResponse, PaginationParams } from "@/lib/types/common";
import type { CourseChapter, CourseRun } from "@/lib/types/course";

// Runs are started through POST /tasks/launch (New task, Run task, chat), so
// the launch fields live in one schema on the server. This module only reads
// and cancels runs.

export async function listCourseRuns(
  params: PaginationParams = {}
): Promise<PaginatedResponse<CourseRun>> {
  const { data } = await apiClient.get<PaginatedResponse<CourseRun>>("/courses/runs", {
    params,
  });
  return data;
}

export async function getCourseRun(id: string): Promise<CourseRun> {
  const { data } = await apiClient.get<CourseRun>(`/courses/runs/${encodeURIComponent(id)}`);
  return data;
}

export async function listCourseChapters(runId: string): Promise<CourseChapter[]> {
  const { data } = await apiClient.get<{ data: CourseChapter[] }>(
    `/courses/runs/${encodeURIComponent(runId)}/chapters`
  );
  return Array.isArray(data.data) ? data.data : [];
}

export async function cancelCourseRun(id: string): Promise<CourseRun> {
  const { data } = await apiClient.post<CourseRun>(
    `/courses/runs/${encodeURIComponent(id)}/cancel`
  );
  return data;
}
