import { apiClient } from "@/lib/api/client";
import type { PaginatedResponse, PaginationParams } from "@/lib/types/common";
import type {
  Session,
  CreateSessionRequest,
  UpdateSessionRequest,
  CopyContextRequest,
  SessionSnapshot,
  CreateSnapshotRequest,
} from "@/lib/types/session";

export async function getSessions(
  params: PaginationParams = {}
): Promise<PaginatedResponse<Session>> {
  const { data } = await apiClient.get<PaginatedResponse<Session>>("/sessions", { params });
  return data;
}

export async function getSession(id: string): Promise<Session> {
  const { data } = await apiClient.get<Session>(`/sessions/${id}`);
  return data;
}

export async function createSession(payload: CreateSessionRequest): Promise<Session> {
  const { data } = await apiClient.post<Session>("/sessions", payload);
  return data;
}

export async function updateSession(
  id: string,
  payload: UpdateSessionRequest
): Promise<Session> {
  const { data } = await apiClient.put<Session>(`/sessions/${id}`, payload);
  return data;
}

export async function deleteSession(id: string): Promise<void> {
  await apiClient.delete(`/sessions/${id}`);
}

export async function copySessionContext(
  sessionId: string,
  payload: CopyContextRequest
): Promise<Session> {
  const { data } = await apiClient.post<Session>(
    `/sessions/${sessionId}/copy-context`,
    payload
  );
  return data;
}

export async function createSessionSnapshot(
  sessionId: string,
  payload: CreateSnapshotRequest
): Promise<SessionSnapshot> {
  const { data } = await apiClient.post<SessionSnapshot>(
    `/sessions/${sessionId}/snapshots`,
    payload
  );
  return data;
}

export async function getSessionSnapshots(
  sessionId: string
): Promise<SessionSnapshot[]> {
  const { data } = await apiClient.get<SessionSnapshot[]>(
    `/sessions/${sessionId}/snapshots`
  );
  return data;
}

export async function restoreSessionSnapshot(
  sessionId: string,
  snapshotId: string
): Promise<Session> {
  const { data } = await apiClient.post<Session>(
    `/sessions/${sessionId}/snapshots/${snapshotId}/restore`
  );
  return data;
}
