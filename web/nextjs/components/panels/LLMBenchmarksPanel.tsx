"use client";

import { useMemo, useState } from "react";
import { Loader2 } from "lucide-react";
import { useLLMModelStats, useLLMRun, useLLMRuns } from "@/lib/hooks/useLLMBenchmarks";
import type { LLMBenchmarkQuery, LLMModelStat, LLMRunSummary } from "@/lib/types/llm-benchmark";
import { LLMCallsTable } from "@/components/benchmarks/LLMCallsTable";
import { fmtMs, fmtTime, fmtTokens } from "@/components/benchmarks/format";

type Range = "1d" | "7d" | "30d" | "90d";
const RANGES: { label: string; value: Range; days: number }[] = [
  { label: "24 hours", value: "1d", days: 1 },
  { label: "7 days", value: "7d", days: 7 },
  { label: "30 days", value: "30d", days: 30 },
  { label: "90 days", value: "90d", days: 90 },
];

/**
 * Read-only LLM benchmark: measured per-call timings and token usage, from
 * raw records, for whichever provider and model actually served each call.
 * It reports; it never picks a "best" model.
 */
export function LLMBenchmarksPanel() {
  const [range, setRange] = useState<Range>("7d");
  const [byStage, setByStage] = useState(false);
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<{ kind: string; id: string } | null>(null);

  // Fixed per range selection so the query key is stable between renders.
  const query = useMemo<LLMBenchmarkQuery>(() => {
    const days = RANGES.find((r) => r.value === range)?.days ?? 7;
    return { from: new Date(Date.now() - days * 86_400_000).toISOString() };
  }, [range]);

  const stats = useLLMModelStats(query, byStage);
  const runs = useLLMRuns(query, page);

  return (
    <div className="space-y-6 p-6" data-testid="llm-benchmarks-panel">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">LLM Benchmarks</h1>
          <p className="text-sm text-muted-foreground">
            Measured duration and token usage of every text LLM call, per provider and exact model.
            Token counts show “—” when the provider did not report them.
          </p>
        </div>
        <div className="flex gap-1 rounded-md border border-border p-0.5" role="group" aria-label="Time range">
          {RANGES.map((r) => (
            <button
              key={r.value}
              type="button"
              onClick={() => {
                setRange(r.value);
                setPage(1);
              }}
              className={
                "rounded px-2.5 py-1 text-xs " +
                (range === r.value ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-muted")
              }
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>

      <section className="space-y-2">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium uppercase tracking-wide text-muted-foreground">
            By model{byStage ? " and stage" : ""}
          </h2>
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <input type="checkbox" checked={byStage} onChange={(e) => setByStage(e.target.checked)} />
            Split by stage
          </label>
        </div>
        {stats.isPending ? (
          <Loading />
        ) : stats.isError ? (
          <ErrorBox what="model statistics" />
        ) : (
          <ModelStatsTable stats={stats.data} byStage={byStage} />
        )}
      </section>

      <section className="space-y-2">
        <h2 className="text-sm font-medium uppercase tracking-wide text-muted-foreground">Runs</h2>
        {runs.isPending ? (
          <Loading />
        ) : runs.isError ? (
          <ErrorBox what="runs" />
        ) : (
          <>
            <RunsTable
              runs={runs.data.data}
              selected={selected}
              onSelect={(r) => setSelected({ kind: r.run_kind, id: r.run_id })}
            />
            {runs.data.total_pages > 1 && (
              <div className="flex items-center justify-end gap-2 text-xs">
                <button type="button" disabled={page <= 1} onClick={() => setPage((p) => p - 1)} className="rounded border border-border px-2 py-1 disabled:opacity-40">
                  Previous
                </button>
                <span className="text-muted-foreground">
                  Page {runs.data.page} of {runs.data.total_pages}
                </span>
                <button type="button" disabled={page >= runs.data.total_pages} onClick={() => setPage((p) => p + 1)} className="rounded border border-border px-2 py-1 disabled:opacity-40">
                  Next
                </button>
              </div>
            )}
          </>
        )}
      </section>

      {selected && <RunCalls kind={selected.kind} id={selected.id} />}
    </div>
  );
}

function ModelStatsTable({ stats, byStage }: { stats: LLMModelStat[]; byStage: boolean }) {
  if (stats.length === 0) {
    return <p className="text-sm text-muted-foreground">No LLM calls recorded in this range.</p>;
  }
  return (
    <div className="overflow-x-auto rounded-md border border-border scrollbar-thin">
      <table className="w-full text-left text-xs" data-testid="llm-model-stats">
        <thead className="bg-muted/50 text-muted-foreground">
          <tr>
            <th className="px-2 py-1.5 font-medium">Provider</th>
            <th className="px-2 py-1.5 font-medium">Model</th>
            {byStage && <th className="px-2 py-1.5 font-medium">Stage</th>}
            <th className="px-2 py-1.5 text-right font-medium">Runs</th>
            <th className="px-2 py-1.5 text-right font-medium">Calls</th>
            <th className="px-2 py-1.5 text-right font-medium">Avg</th>
            <th className="px-2 py-1.5 text-right font-medium">p50</th>
            <th className="px-2 py-1.5 text-right font-medium">p95</th>
            <th className="px-2 py-1.5 text-right font-medium">Min</th>
            <th className="px-2 py-1.5 text-right font-medium">Max</th>
            <th className="px-2 py-1.5 text-right font-medium">In tok</th>
            <th className="px-2 py-1.5 text-right font-medium">Out tok</th>
            <th className="px-2 py-1.5 text-right font-medium">Total tok</th>
            <th className="px-2 py-1.5 text-right font-medium">Retries</th>
            <th className="px-2 py-1.5 text-right font-medium">Failed</th>
          </tr>
        </thead>
        <tbody className="font-mono">
          {stats.map((s) => (
            <tr key={`${s.provider}/${s.model}/${s.stage ?? ""}`} className="border-t border-border">
              <td className="px-2 py-1.5">{s.provider}</td>
              <td className="px-2 py-1.5">{s.model || "—"}</td>
              {byStage && <td className="px-2 py-1.5">{s.stage || "—"}</td>}
              <td className="px-2 py-1.5 text-right">{s.runs}</td>
              <td className="px-2 py-1.5 text-right">{s.calls}</td>
              <td className="px-2 py-1.5 text-right">{fmtMs(s.avg_duration_ms)}</td>
              <td className="px-2 py-1.5 text-right">{fmtMs(s.p50_duration_ms)}</td>
              <td className="px-2 py-1.5 text-right">{fmtMs(s.p95_duration_ms)}</td>
              <td className="px-2 py-1.5 text-right">{fmtMs(s.min_duration_ms)}</td>
              <td className="px-2 py-1.5 text-right">{fmtMs(s.max_duration_ms)}</td>
              <td className="px-2 py-1.5 text-right">{fmtTokens(s.input_tokens)}</td>
              <td className="px-2 py-1.5 text-right">{fmtTokens(s.output_tokens)}</td>
              <td className="px-2 py-1.5 text-right" title={`${s.calls_with_usage} of ${s.calls} calls reported usage`}>
                {fmtTokens(s.total_tokens)}
              </td>
              <td className="px-2 py-1.5 text-right">{s.retries}</td>
              <td className={"px-2 py-1.5 text-right " + (s.failures > 0 ? "text-signal-error" : "")}>
                {s.failures}
                {s.cancelled > 0 && <span className="text-muted-foreground"> (+{s.cancelled} cancelled)</span>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function RunsTable({
  runs,
  selected,
  onSelect,
}: {
  runs: LLMRunSummary[];
  selected: { kind: string; id: string } | null;
  onSelect: (r: LLMRunSummary) => void;
}) {
  if (runs.length === 0) {
    return <p className="text-sm text-muted-foreground">No runs recorded in this range.</p>;
  }
  return (
    <div className="overflow-x-auto rounded-md border border-border scrollbar-thin">
      <table className="w-full text-left text-xs" data-testid="llm-runs">
        <thead className="bg-muted/50 text-muted-foreground">
          <tr>
            <th className="px-2 py-1.5 font-medium">Started</th>
            <th className="px-2 py-1.5 font-medium">Kind</th>
            <th className="px-2 py-1.5 font-medium">Agent</th>
            <th className="px-2 py-1.5 font-medium">Provider / model</th>
            <th className="px-2 py-1.5 text-right font-medium">Calls</th>
            <th className="px-2 py-1.5 text-right font-medium">LLM time</th>
            <th className="px-2 py-1.5 text-right font-medium">Run duration</th>
            <th className="px-2 py-1.5 text-right font-medium">In / out tok</th>
            <th className="px-2 py-1.5 text-right font-medium">Retries</th>
            <th className="px-2 py-1.5 text-right font-medium">Failed</th>
            <th className="px-2 py-1.5 font-medium">Status</th>
          </tr>
        </thead>
        <tbody>
          {runs.map((r) => {
            const active = selected?.kind === r.run_kind && selected?.id === r.run_id;
            return (
              <tr
                key={`${r.run_kind}/${r.run_id}`}
                onClick={() => onSelect(r)}
                className={"cursor-pointer border-t border-border hover:bg-muted/40 " + (active ? "bg-muted/60" : "")}
              >
                <td className="px-2 py-1.5">{fmtTime(r.first_call_at)}</td>
                <td className="px-2 py-1.5">{r.run_kind}</td>
                <td className="px-2 py-1.5">{r.agent_name || "—"}</td>
                <td className="px-2 py-1.5 font-mono">{r.models.join(", ")}</td>
                <td className="px-2 py-1.5 text-right font-mono">{r.call_count}</td>
                <td className="px-2 py-1.5 text-right font-mono">{fmtMs(r.llm_time_ms)}</td>
                <td className="px-2 py-1.5 text-right font-mono">{fmtMs(r.run_duration_ms)}</td>
                <td className="px-2 py-1.5 text-right font-mono">
                  {fmtTokens(r.input_tokens)} / {fmtTokens(r.output_tokens)}
                </td>
                <td className="px-2 py-1.5 text-right font-mono">{r.retries}</td>
                <td className={"px-2 py-1.5 text-right font-mono " + (r.failures > 0 ? "text-signal-error" : "")}>{r.failures}</td>
                <td className="px-2 py-1.5">{r.run_status || "—"}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function RunCalls({ kind, id }: { kind: string; id: string }) {
  const run = useLLMRun(kind, id);
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium uppercase tracking-wide text-muted-foreground">
        Calls in {kind} run <span className="font-mono normal-case">{id}</span>
      </h2>
      {run.isPending ? <Loading /> : run.isError ? <ErrorBox what="this run" /> : <LLMCallsTable calls={run.data.calls} />}
    </section>
  );
}

function Loading() {
  return (
    <div className="flex items-center gap-2 text-sm text-muted-foreground">
      <Loader2 className="h-4 w-4 animate-spin" /> Loading…
    </div>
  );
}

function ErrorBox({ what }: { what: string }) {
  return (
    <div className="rounded-md border border-signal-error/40 bg-signal-error/10 p-3 text-sm text-signal-error">
      Couldn’t load {what}.
    </div>
  );
}
