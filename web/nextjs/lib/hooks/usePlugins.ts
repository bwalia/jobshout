"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { PaginationParams } from "@/lib/types/common";
import type { CreatePluginRequest, UpdatePluginRequest } from "@/lib/types/workflow";
import {
  getPlugins,
  getPlugin,
  createPlugin,
  updatePlugin,
  deletePlugin,
  executePlugin,
  getPluginExecutions,
} from "@/lib/api/plugins";

export const pluginKeys = {
  all: ["plugins"] as const,
  lists: () => [...pluginKeys.all, "list"] as const,
  list: (params: PaginationParams) => [...pluginKeys.lists(), params] as const,
  details: () => [...pluginKeys.all, "detail"] as const,
  detail: (id: string) => [...pluginKeys.details(), id] as const,
  executions: (pluginId: string) => [...pluginKeys.all, "executions", pluginId] as const,
};

export function usePlugins(params: PaginationParams = {}) {
  return useQuery({
    queryKey: pluginKeys.list(params),
    queryFn: () => getPlugins(params),
  });
}

export function usePlugin(id: string) {
  return useQuery({
    queryKey: pluginKeys.detail(id),
    queryFn: () => getPlugin(id),
    enabled: Boolean(id),
  });
}

export function useCreatePlugin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreatePluginRequest) => createPlugin(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: pluginKeys.lists() });
      toast.success("Plugin created");
    },
    onError: () => toast.error("Failed to create plugin"),
  });
}

export function useUpdatePlugin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: UpdatePluginRequest }) =>
      updatePlugin(id, payload),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: pluginKeys.detail(id) });
      qc.invalidateQueries({ queryKey: pluginKeys.lists() });
      toast.success("Plugin updated");
    },
    onError: () => toast.error("Failed to update plugin"),
  });
}

export function useDeletePlugin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deletePlugin(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: pluginKeys.lists() });
      toast.success("Plugin deleted");
    },
    onError: () => toast.error("Failed to delete plugin"),
  });
}

export function useExecutePlugin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input?: Record<string, unknown> }) =>
      executePlugin(id, input),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: pluginKeys.executions(id) });
      toast.success("Plugin execution started");
    },
    onError: () => toast.error("Failed to execute plugin"),
  });
}

export function usePluginExecutions(pluginId: string, params: PaginationParams = {}) {
  return useQuery({
    queryKey: pluginKeys.executions(pluginId),
    queryFn: () => getPluginExecutions(pluginId, params),
    enabled: Boolean(pluginId),
  });
}
