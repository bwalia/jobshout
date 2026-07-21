"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  getSkills,
  getSkill,
  createSkill,
  updateSkill,
  deleteSkill,
  getAgentSkills,
  enableAgentSkill,
  disableAgentSkill,
  type CreateSkillRequest,
  type UpdateSkillRequest,
} from "@/lib/api/skills";

export const skillKeys = {
  all: ["skills"] as const,
  lists: () => [...skillKeys.all, "list"] as const,
  details: () => [...skillKeys.all, "detail"] as const,
  detail: (id: string) => [...skillKeys.details(), id] as const,
  forAgent: (agentId: string) => [...skillKeys.all, "agent", agentId] as const,
};

// ─── Registry ────────────────────────────────────────────────────────────────

export function useSkills() {
  return useQuery({
    queryKey: skillKeys.lists(),
    queryFn: getSkills,
  });
}

export function useSkill(id: string) {
  return useQuery({
    queryKey: skillKeys.detail(id),
    queryFn: () => getSkill(id),
    enabled: Boolean(id),
  });
}

export function useCreateSkill() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreateSkillRequest) => createSkill(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: skillKeys.lists() });
      toast.success("Skill created");
    },
    onError: () => toast.error("Failed to create skill"),
  });
}

export function useUpdateSkill() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: UpdateSkillRequest }) =>
      updateSkill(id, payload),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: skillKeys.detail(id) });
      qc.invalidateQueries({ queryKey: skillKeys.lists() });
      toast.success("Skill updated");
    },
    onError: () => toast.error("Failed to update skill"),
  });
}

export function useDeleteSkill() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteSkill(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: skillKeys.lists() });
      toast.success("Skill deleted");
    },
    onError: () => toast.error("Failed to delete skill"),
  });
}

// ─── Per-agent enablement ────────────────────────────────────────────────────

export function useAgentSkills(agentId: string) {
  return useQuery({
    queryKey: skillKeys.forAgent(agentId),
    queryFn: () => getAgentSkills(agentId),
    enabled: Boolean(agentId),
  });
}

export function useEnableAgentSkill(agentId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (skillId: string) => enableAgentSkill(agentId, skillId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: skillKeys.forAgent(agentId) });
      toast.success("Skill enabled");
    },
    onError: () => toast.error("Failed to enable skill"),
  });
}

export function useDisableAgentSkill(agentId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (skillId: string) => disableAgentSkill(agentId, skillId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: skillKeys.forAgent(agentId) });
      toast.success("Skill disabled");
    },
    onError: () => toast.error("Failed to disable skill"),
  });
}
