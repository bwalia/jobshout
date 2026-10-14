import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiErrorMessage } from "@/lib/api/client";
import {
  addNewSprintJob,
  addSprintAgent,
  createSprint,
  getSprint,
  listSprints,
  removeSprintAgent,
  updateSprintStatus,
  type CreateSprintRequest,
  type NewSprintJob,
  type Sprint,
  type SprintDetail,
  type SprintRole,
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

/** A change to one sprint's squad or jobs: refreshes that sprint's board. */
function useSprintChange<V extends { id: string }>(
  fn: (v: V) => Promise<void>,
  done: string,
  failed: string
) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: (_, v) => {
      toast.success(done);
      qc.invalidateQueries({ queryKey: sprintKeys.detail(v.id) });
    },
    onError: (e) => toast.error(apiErrorMessage(e, failed)),
  });
}

export function useAddSprintAgent() {
  return useSprintChange(
    (v: { id: string; agentId: string; role: SprintRole }) =>
      addSprintAgent(v.id, v.agentId, v.role),
    "Added to the squad",
    "Could not add the agent"
  );
}

export function useRemoveSprintAgent() {
  return useSprintChange(
    (v: { id: string; agentId: string; role: SprintRole }) =>
      removeSprintAgent(v.id, v.agentId, v.role),
    "Removed from the squad",
    "Could not remove the agent"
  );
}

export function useAddSprintJob() {
  return useSprintChange(
    (v: { id: string; job: NewSprintJob; position: number }) =>
      addNewSprintJob(v.id, v.job, v.position),
    "Job started and added to the sprint",
    "Could not add the job"
  );
}
