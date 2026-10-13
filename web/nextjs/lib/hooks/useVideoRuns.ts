"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { apiErrorMessage } from "@/lib/api/client";
import { cancelVideoRun, listVideoRuns } from "@/lib/api/videos";
import { isVideoRunActive, type VideoRun } from "@/lib/types/video";

export const videoKeys = {
  all: ["videos"] as const,
  runs: () => [...videoKeys.all, "runs"] as const,
};

/** Every run with its live progress, polled while any is still going. */
export function useVideoRuns() {
  return useQuery({
    queryKey: videoKeys.runs(),
    queryFn: listVideoRuns,
    refetchInterval: (query) =>
      query.state.data?.some((r: VideoRun) => isVideoRunActive(r.status)) ? 4000 : false,
  });
}

export function useCancelVideoRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: cancelVideoRun,
    onSuccess: () => {
      toast.success("Video cancelled");
      void qc.invalidateQueries({ queryKey: videoKeys.runs() });
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not cancel the video")),
  });
}
