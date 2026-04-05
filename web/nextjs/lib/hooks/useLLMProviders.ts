import {
  useQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { toast } from "sonner";
import {
  getBuiltinProviders,
  getLLMProviders,
  createLLMProvider,
  updateLLMProvider,
  deleteLLMProvider,
} from "@/lib/api/llm-providers";
import type { CreateLLMProviderRequest, UpdateLLMProviderRequest } from "@/lib/types/llm-provider";

export const llmProviderKeys = {
  all: ["llm-providers"] as const,
  builtin: () => [...llmProviderKeys.all, "builtin"] as const,
  list: () => [...llmProviderKeys.all, "list"] as const,
};

export function useBuiltinProviders() {
  return useQuery({
    queryKey: llmProviderKeys.builtin(),
    queryFn: getBuiltinProviders,
  });
}

export function useLLMProviders() {
  return useQuery({
    queryKey: llmProviderKeys.list(),
    queryFn: getLLMProviders,
  });
}

export function useCreateLLMProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createLLMProvider,
    onSuccess: (p) => {
      qc.invalidateQueries({ queryKey: llmProviderKeys.list() });
      toast.success(`Provider "${p.name}" created.`);
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}

export function useUpdateLLMProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: UpdateLLMProviderRequest }) =>
      updateLLMProvider(id, payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: llmProviderKeys.list() });
      toast.success("Provider updated.");
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}

export function useDeleteLLMProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteLLMProvider,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: llmProviderKeys.list() });
      toast.success("Provider deleted.");
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}
