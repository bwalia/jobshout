import { apiClient } from "@/lib/api/client";
import type { VideoRun } from "@/lib/types/video";

// Videos start through POST /tasks/launch (New task, Run task, chat), so the
// launch fields live in one schema on the server. This module reads the runs
// and cancels one.

export async function listVideoRuns(): Promise<VideoRun[]> {
  const { data } = await apiClient.get<{ data: VideoRun[] }>("/videos/runs");
  return Array.isArray(data.data) ? data.data : [];
}

export async function cancelVideoRun(id: string): Promise<VideoRun> {
  const { data } = await apiClient.post<VideoRun>(`/videos/runs/${encodeURIComponent(id)}/cancel`);
  return data;
}
