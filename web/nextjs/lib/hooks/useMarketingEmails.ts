"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { apiErrorMessage } from "@/lib/api/client";
import {
  getMarketingEmailRun,
  listMarketingEmailRuns,
  regenerateMarketingEmailRun,
  unpublishMarketingEmailRun,
} from "@/lib/api/marketing-emails";
import {
  isMarketingEmailRunActive,
  type MarketingEmailRun,
} from "@/lib/types/marketing-email";

export const marketingEmailKeys = {
  all: ["marketing-emails"] as const,
  runs: () => [...marketingEmailKeys.all, "runs"] as const,
  run: (id: string) => [...marketingEmailKeys.all, "run", id] as const,
};

export function useMarketingEmailRuns() {
  return useQuery({
    queryKey: marketingEmailKeys.runs(),
    queryFn: listMarketingEmailRuns,
    refetchInterval: (query) =>
      query.state.data?.some((r: MarketingEmailRun) => isMarketingEmailRunActive(r.status))
        ? 4000
        : false,
  });
}

export function useMarketingEmailRun(id: string | null) {
  return useQuery({
    queryKey: marketingEmailKeys.run(id ?? ""),
    queryFn: () => getMarketingEmailRun(id!),
    enabled: Boolean(id),
    refetchInterval: (query) =>
      query.state.data && isMarketingEmailRunActive(query.state.data.status) ? 3000 : false,
  });
}

export function useRegenerateMarketingEmail() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: regenerateMarketingEmailRun,
    onSuccess: (run) => {
      toast.success("Re-generating email");
      void qc.invalidateQueries({ queryKey: marketingEmailKeys.runs() });
      void qc.invalidateQueries({ queryKey: marketingEmailKeys.run(run.id) });
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not re-generate")),
  });
}

export function useUnpublishMarketingEmail() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: unpublishMarketingEmailRun,
    onSuccess: (run) => {
      toast.success("Email unpublished (archived)");
      void qc.invalidateQueries({ queryKey: marketingEmailKeys.runs() });
      void qc.invalidateQueries({ queryKey: marketingEmailKeys.run(run.id) });
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not unpublish")),
  });
}
