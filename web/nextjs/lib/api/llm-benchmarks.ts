import { apiClient } from "@/lib/api/client";
import type { PaginatedResponse, PaginationParams } from "@/lib/types/common";
import type {
  LLMBenchmarkQuery,
  LLMCall,
  LLMModelStat,
  LLMRunDetail,
  LLMRunSummary,
} from "@/lib/types/llm-benchmark";

/** Per provider + exact model call statistics (optionally per stage). */
export async function getLLMModelStats(
  q: LLMBenchmarkQuery & { group_by?: "stage" } = {}
): Promise<LLMModelStat[]> {
  const { data } = await apiClient.get<LLMModelStat[]>("/benchmarks/models", {
    params: q,
  });
  return data;
}

/** Recorded calls grouped by run, newest first. */
export async function getLLMRuns(
  q: LLMBenchmarkQuery & PaginationParams = {}
): Promise<PaginatedResponse<LLMRunSummary>> {
  const { data } = await apiClient.get<PaginatedResponse<LLMRunSummary>>(
    "/benchmarks/runs",
    { params: q }
  );
  return data;
}

/** One run and every LLM call it made, in order. */
export async function getLLMRun(
  runKind: string,
  runId: string
): Promise<LLMRunDetail> {
  const { data } = await apiClient.get<LLMRunDetail>(
    `/benchmarks/runs/${encodeURIComponent(runKind)}/${encodeURIComponent(runId)}`
  );
  return data;
}

/** Raw per-call rows, paginated. */
export async function getLLMCalls(
  q: LLMBenchmarkQuery & PaginationParams & { run_id?: string } = {}
): Promise<PaginatedResponse<LLMCall>> {
  const { data } = await apiClient.get<PaginatedResponse<LLMCall>>(
    "/benchmarks/calls",
    { params: q }
  );
  return data;
}
