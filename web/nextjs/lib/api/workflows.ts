import { apiClient } from "@/lib/api/client";
import type { PaginatedResponse, PaginationParams } from "@/lib/types/common";
import type {
  Workflow,
  WorkflowRun,
  CreateWorkflowRequest,
  UpdateWorkflowRequest,
  ExecuteWorkflowRequest,
  EngineInfo,
} from "@/lib/types/workflow";

export async function getWorkflows(
  params: PaginationParams = {}
): Promise<PaginatedResponse<Workflow>> {
  const { data } = await apiClient.get("/workflows", { params });
  return data;
}

export async function getWorkflow(id: string): Promise<Workflow> {
  const { data } = await apiClient.get(`/workflows/${id}`);
  return data;
}

export async function createWorkflow(
  payload: CreateWorkflowRequest
): Promise<Workflow> {
  const { data } = await apiClient.post("/workflows", payload);
  return data;
}

export async function updateWorkflow(
  id: string,
  payload: UpdateWorkflowRequest
): Promise<Workflow> {
  const { data } = await apiClient.put(`/workflows/${id}`, payload);
  return data;
}

export async function deleteWorkflow(id: string): Promise<void> {
  await apiClient.delete(`/workflows/${id}`);
}

export async function executeWorkflow(
  id: string,
  payload: ExecuteWorkflowRequest = {}
): Promise<WorkflowRun> {
  const { data } = await apiClient.post(`/workflows/${id}/execute`, payload);
  return data;
}

export async function getWorkflowRuns(
  workflowId: string,
  params: PaginationParams = {}
): Promise<PaginatedResponse<WorkflowRun>> {
  const { data } = await apiClient.get(`/workflows/${workflowId}/runs`, {
    params,
  });
  return data;
}

export async function getWorkflowRun(runId: string): Promise<WorkflowRun> {
  const { data } = await apiClient.get(`/workflow-runs/${runId}`);
  return data;
}

export async function getEngines(): Promise<EngineInfo[]> {
  const { data } = await apiClient.get("/engines");
  return data;
}
