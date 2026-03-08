import { apiClient } from "./client";

export interface KnowledgeFile {
  id: string;
  agent_id: string;
  filename: string;
  content: string;
  size_bytes: number;
  created_at: string;
  updated_at: string;
}

export async function getKnowledgeFiles(
  agentId: string
): Promise<KnowledgeFile[]> {
  const { data } = await apiClient.get<KnowledgeFile[]>(
    `/agents/${agentId}/knowledge`
  );
  return data;
}

export async function getKnowledgeFile(
  agentId: string,
  fileId: string
): Promise<KnowledgeFile> {
  const { data } = await apiClient.get<KnowledgeFile>(
    `/agents/${agentId}/knowledge/${fileId}`
  );
  return data;
}

export async function createKnowledgeFile(
  agentId: string,
  payload: { filename: string; content: string }
): Promise<{ id: string; filename: string }> {
  const { data } = await apiClient.post<{ id: string; filename: string }>(
    `/agents/${agentId}/knowledge`,
    payload
  );
  return data;
}

export async function updateKnowledgeFile(
  agentId: string,
  fileId: string,
  content: string
): Promise<{ message: string }> {
  const { data } = await apiClient.put<{ message: string }>(
    `/agents/${agentId}/knowledge/${fileId}`,
    { content }
  );
  return data;
}

export async function deleteKnowledgeFile(
  agentId: string,
  fileId: string
): Promise<void> {
  await apiClient.delete(`/agents/${agentId}/knowledge/${fileId}`);
}
