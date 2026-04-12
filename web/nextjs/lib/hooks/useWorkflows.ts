"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { PaginationParams } from "@/lib/types/common";
import type {
  CreateWorkflowRequest,
  UpdateWorkflowRequest,
  ExecuteWorkflowRequest,
} from "@/lib/types/workflow";
import {
  getWorkflows,
  getWorkflow,
  createWorkflow,
  updateWorkflow,
  deleteWorkflow,
  executeWorkflow,
  getWorkflowRuns,
  getWorkflowRun,
  getEngines,
} from "@/lib/api/workflows";

export const workflowKeys = {
  all: ["workflows"] as const,
  lists: () => [...workflowKeys.all, "list"] as const,
  list: (params: PaginationParams) =>
    [...workflowKeys.lists(), params] as const,
  details: () => [...workflowKeys.all, "detail"] as const,
  detail: (id: string) => [...workflowKeys.details(), id] as const,
  runs: (workflowId: string) =>
    [...workflowKeys.all, "runs", workflowId] as const,
  run: (runId: string) => [...workflowKeys.all, "run", runId] as const,
  engines: () => ["engines"] as const,
};

export function useWorkflows(params: PaginationParams = {}) {
  return useQuery({
    queryKey: workflowKeys.list(params),
    queryFn: () => getWorkflows(params),
  });
}

export function useWorkflow(id: string) {
  return useQuery({
    queryKey: workflowKeys.detail(id),
    queryFn: () => getWorkflow(id),
    enabled: Boolean(id),
  });
}

export function useCreateWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreateWorkflowRequest) => createWorkflow(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: workflowKeys.lists() });
      toast.success("Workflow created");
    },
    onError: () => toast.error("Failed to create workflow"),
  });
}

export function useUpdateWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      payload,
    }: {
      id: string;
      payload: UpdateWorkflowRequest;
    }) => updateWorkflow(id, payload),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: workflowKeys.detail(id) });
      qc.invalidateQueries({ queryKey: workflowKeys.lists() });
      toast.success("Workflow updated");
    },
    onError: () => toast.error("Failed to update workflow"),
  });
}

export function useDeleteWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteWorkflow(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: workflowKeys.lists() });
      toast.success("Workflow deleted");
    },
    onError: () => toast.error("Failed to delete workflow"),
  });
}

export function useExecuteWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      payload,
    }: {
      id: string;
      payload?: ExecuteWorkflowRequest;
    }) => executeWorkflow(id, payload),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: workflowKeys.runs(id) });
      toast.success("Workflow execution started");
    },
    onError: () => toast.error("Failed to execute workflow"),
  });
}

export function useWorkflowRuns(
  workflowId: string,
  params: PaginationParams = {}
) {
  return useQuery({
    queryKey: workflowKeys.runs(workflowId),
    queryFn: () => getWorkflowRuns(workflowId, params),
    enabled: Boolean(workflowId),
    refetchInterval: (query) => {
      const runs = query.state.data?.data;
      const hasActive = runs?.some(
        (r: { status: string }) =>
          r.status === "running" || r.status === "pending",
      );
      return hasActive ? 3000 : false;
    },
  });
}

export function useWorkflowRun(runId: string) {
  return useQuery({
    queryKey: workflowKeys.run(runId),
    queryFn: () => getWorkflowRun(runId),
    enabled: Boolean(runId),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "running" || status === "pending" ? 2000 : false;
    },
  });
}

export function useEngines() {
  return useQuery({
    queryKey: workflowKeys.engines(),
    queryFn: () => getEngines(),
  });
}
