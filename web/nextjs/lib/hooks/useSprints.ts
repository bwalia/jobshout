import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  createSprint,
  getSprint,
  listSprints,
  updateSprintStatus,
  type CreateSprintRequest,
  type Sprint,
  type SprintDetail,
  type SprintStatus,
} from "@/lib/api/sprints";

export const sprintKeys = {
  all: ["sprints"] as const,
  detail: (id: string) => ["sprints", id] as const,
};

export function useSprints() {
  return useQuery<Sprint[]>({
    queryKey: sprintKeys.all,
    queryFn: listSprints,
  });
}

export function useSprint(id: string | undefined) {
  return useQuery<SprintDetail>({
    queryKey: id ? sprintKeys.detail(id) : ["sprints", "noop"],
    queryFn: () => getSprint(id!),
    enabled: Boolean(id),
    refetchInterval: 7_000,
  });
}

export function useCreateSprint() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: CreateSprintRequest) => createSprint(req),
    onSuccess: () => {
      toast.success("Sprint created");
      qc.invalidateQueries({ queryKey: sprintKeys.all });
    },
    onError: (e: Error) => toast.error(e.message),
  });
}

export function useUpdateSprintStatus() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: SprintStatus }) =>
      updateSprintStatus(id, status),
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: sprintKeys.all });
      qc.invalidateQueries({ queryKey: sprintKeys.detail(vars.id) });
    },
    onError: (e: Error) => toast.error(e.message),
  });
}
