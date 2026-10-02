import { useQuery } from "@tanstack/react-query";
import { isAxiosError } from "axios";
import { getLLMModelStats, getLLMRun, getLLMRuns } from "@/lib/api/llm-benchmarks";
import type { LLMBenchmarkQuery } from "@/lib/types/llm-benchmark";

export const llmBenchmarkKeys = {
  all: ["llm-benchmarks"] as const,
  models: (q: LLMBenchmarkQuery, byStage: boolean) =>
    [...llmBenchmarkKeys.all, "models", q, byStage] as const,
  runs: (q: LLMBenchmarkQuery, page: number) =>
    [...llmBenchmarkKeys.all, "runs", q, page] as const,
  run: (kind: string, id: string) => [...llmBenchmarkKeys.all, "run", kind, id] as const,
};

export function useLLMModelStats(q: LLMBenchmarkQuery, byStage: boolean) {
  return useQuery({
    queryKey: llmBenchmarkKeys.models(q, byStage),
    queryFn: () => getLLMModelStats(byStage ? { ...q, group_by: "stage" } : q),
  });
}

export function useLLMRuns(q: LLMBenchmarkQuery, page: number) {
  return useQuery({
    queryKey: llmBenchmarkKeys.runs(q, page),
    queryFn: () => getLLMRuns({ ...q, page, per_page: 20 }),
  });
}

/**
 * One run's LLM calls. A run with no recorded calls is a 404, which is an
 * answer ("nothing recorded"), not an error worth retrying.
 */
export function useLLMRun(kind: string | null, id: string | null, poll = false) {
  return useQuery({
    queryKey: llmBenchmarkKeys.run(kind ?? "", id ?? ""),
    queryFn: () => getLLMRun(kind as string, id as string),
    enabled: Boolean(kind && id),
    retry: (count, err) => !(isAxiosError(err) && err.response?.status === 404) && count < 2,
    refetchInterval: poll ? 5000 : false,
  });
}
