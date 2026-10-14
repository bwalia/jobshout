"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  createIntegration,
  deleteIntegration,
  listIntegrations,
  updateIntegration,
  type UpdateIntegrationRequest,
} from "@/lib/api/integrations";
import { apiErrorMessage } from "@/lib/api/client";

export const integrationKeys = {
  all: ["integrations"] as const,
};

export function useIntegrations() {
  return useQuery({ queryKey: integrationKeys.all, queryFn: listIntegrations, retry: false });
}

// The writer's website list is built from opsapi integrations, so refresh it too.
function useInvalidate() {
  const qc = useQueryClient();
  return () => {
    qc.invalidateQueries({ queryKey: integrationKeys.all });
    qc.invalidateQueries({ queryKey: ["blog-destinations"] });
  };
}

// Errors are shown inline by the form; the toast is the short summary.
export function useCreateIntegration() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: createIntegration,
    onSuccess: (i) => {
      invalidate();
      toast.success(`"${i.name}" connected`);
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not add the integration")),
  });
}

export function useUpdateIntegration() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: UpdateIntegrationRequest }) =>
      updateIntegration(id, payload),
    onSuccess: (i) => {
      invalidate();
      toast.success(`"${i.name}" saved`);
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not save the integration")),
  });
}

export function useDeleteIntegration() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: deleteIntegration,
    onSuccess: () => {
      invalidate();
      toast.success("Integration removed");
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not remove the integration")),
  });
}
