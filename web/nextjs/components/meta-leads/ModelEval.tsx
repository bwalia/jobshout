"use client";

import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { apiErrorMessage } from "@/lib/api/client";
import { getAvailableProviderModels } from "@/lib/api/llm-providers";
import { listMetaEvals, startMetaEval, type MetaLeadEval, type MetaLeadEvalRow } from "@/lib/api/meta-leads";

const MAX_MODELS = 4;

const pct = (x: number) => `${Math.round(x * 100)}%`;

/** Compares models on the messages people labelled in the Leads tab. */
export function ModelEval() {
  const [evals, setEvals] = useState<MetaLeadEval[]>([]);
  const [labelled, setLabelled] = useState(0);
  const [options, setOptions] = useState<string[]>([]);
  const [picked, setPicked] = useState<string[]>([]);
  const [runs, setRuns] = useState(3);
  const [starting, setStarting] = useState(false);

  async function load() {
    try {
      const res = await listMetaEvals();
      setEvals(res.evals);
      setLabelled(res.labelled);
    } catch (e) {
      toast.error(apiErrorMessage(e, "Failed to load model comparisons"));
    }
  }

  useEffect(() => {
    void load();
    void getAvailableProviderModels()
      .then((res) => setOptions(res.providers.flatMap((p) => p.models.map((m) => `${p.provider}:${m.name}`))))
      .catch(() => setOptions([]));
  }, []);

  const running = evals.some((e) => e.status === "running");
  useEffect(() => {
    if (!running) return;
    const t = setInterval(() => void load(), 5000);
    return () => clearInterval(t);
  }, [running]);

  function toggle(m: string) {
    setPicked((cur) => (cur.includes(m) ? cur.filter((x) => x !== m) : cur.length < MAX_MODELS ? [...cur, m] : cur));
  }

  async function onStart() {
    setStarting(true);
    try {
      await startMetaEval(picked, runs);
      toast.success("Comparison started");
      await load();
    } catch (e) {
      toast.error(apiErrorMessage(e, "Could not start the comparison"));
    } finally {
      setStarting(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-border p-4 text-sm">
        <h3 className="font-semibold">Compare models on your labelled messages</h3>
        <p className="mt-1 text-muted-foreground">
          {labelled} labelled so far. Mark DMs, comments and mentions as lead or not a lead in the Leads tab;
          200–300 gives a reliable comparison. Each model qualifies every labelled message {runs} times with the
          live prompt. To switch, set <code>AGENT_PROVIDER__META_LEAD_ANALYST</code> and{" "}
          <code>AGENT_MODEL__META_LEAD_ANALYST</code> in the ring&apos;s config.
        </p>
        {options.length === 0 ? (
          <p className="mt-3 text-muted-foreground">No models available from the configured providers.</p>
        ) : (
          <div className="mt-3 flex max-h-48 flex-wrap gap-2 overflow-y-auto">
            {options.map((m) => (
              <label
                key={m}
                className={`flex cursor-pointer items-center gap-1.5 rounded-md border px-2 py-1 text-xs ${
                  picked.includes(m) ? "border-primary bg-muted" : "border-border"
                }`}
              >
                <input type="checkbox" checked={picked.includes(m)} onChange={() => toggle(m)} />
                {m}
              </label>
            ))}
          </div>
        )}
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-2 text-xs">
            Runs per message
            <select
              value={runs}
              onChange={(e) => setRuns(Number(e.target.value))}
              className="rounded-md border border-input bg-background px-2 py-1 text-xs"
            >
              {[1, 3, 5].map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </label>
          <button
            type="button"
            onClick={() => void onStart()}
            disabled={starting || picked.length === 0 || labelled === 0}
            className="inline-flex items-center rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
          >
            {starting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
            Compare {picked.length || ""} model{picked.length === 1 ? "" : "s"}
          </button>
          <span className="text-xs text-muted-foreground">Up to {MAX_MODELS}.</span>
        </div>
      </div>

      {evals.length === 0 ? (
        <p className="text-sm text-muted-foreground">No comparisons yet.</p>
      ) : (
        evals.map((e) => <EvalCard key={e.id} e={e} />)
      )}
    </div>
  );
}

function EvalCard({ e }: { e: MetaLeadEval }) {
  // Best first: accuracy, then consistency.
  const rows = [...(e.report ?? [])].sort(
    (a, b) => Number(!!a.error) - Number(!!b.error) || b.accuracy - a.accuracy || b.consistency - a.consistency,
  );
  return (
    <div className="rounded-lg border border-border p-4 text-sm">
      <div className="flex flex-wrap justify-between gap-2">
        <span className="font-medium">
          {e.case_count} messages × {e.runs} runs
        </span>
        <span className="text-xs text-muted-foreground">
          {e.status === "running" ? (
            <span className="inline-flex items-center gap-1">
              <Loader2 className="h-3 w-3 animate-spin" /> running
            </span>
          ) : (
            e.status
          )}{" "}
          · {new Date(e.created_at).toLocaleString()}
        </span>
      </div>
      {e.error_message ? <p className="mt-1 text-xs text-destructive">{e.error_message}</p> : null}
      {e.status === "running" && rows.length === 0 ? (
        <p className="mt-2 text-muted-foreground">{e.models.join(", ")}</p>
      ) : (
        <div className="mt-3 overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="text-muted-foreground">
              <tr>
                <th className="py-1 pr-3 font-medium">Model</th>
                <th className="py-1 pr-3 font-medium" title="Majority verdict matches the label">Accuracy</th>
                <th className="py-1 pr-3 font-medium" title="Of the messages it called leads, how many were">Precision</th>
                <th className="py-1 pr-3 font-medium" title="Of the real leads, how many it found">Recall</th>
                <th className="py-1 pr-3 font-medium" title="Every run gave the same verdict">Consistency</th>
                <th className="py-1 pr-3 font-medium" title="Lead verdicts whose quote is really in the message">Grounded</th>
                <th className="py-1 pr-3 font-medium">Failed calls</th>
                <th className="py-1 font-medium">Avg time</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <ResultRow key={r.model} r={r} best={i === 0 && !r.error && rows.length > 1} />
              ))}
            </tbody>
          </table>
        </div>
      )}
      {rows.some((r) => r.misses?.length) ? (
        <details className="mt-3">
          <summary className="cursor-pointer text-xs text-muted-foreground">Messages a model got wrong</summary>
          {rows
            .filter((r) => r.misses?.length)
            .map((r) => (
              <div key={r.model} className="mt-2">
                <p className="text-xs font-medium">{r.model}</p>
                <ul className="mt-1 space-y-1 text-xs text-muted-foreground">
                  {r.misses!.map((m) => (
                    <li key={m.lead_id}>
                      <span className="text-foreground">{m.label ? "lead" : "not a lead"}</span> · voted {m.votes} · “
                      {m.body}”
                    </li>
                  ))}
                </ul>
              </div>
            ))}
        </details>
      ) : null}
    </div>
  );
}

function ResultRow({ r, best }: { r: MetaLeadEvalRow; best: boolean }) {
  if (r.error) {
    return (
      <tr className="border-t border-border">
        <td className="py-1 pr-3">{r.model}</td>
        <td colSpan={7} className="py-1 text-destructive">
          {r.error}
        </td>
      </tr>
    );
  }
  return (
    <tr className={`border-t border-border ${best ? "font-medium text-foreground" : ""}`}>
      <td className="py-1 pr-3">
        {r.model}
        {best ? " ★" : ""}
      </td>
      <td className="py-1 pr-3">{pct(r.accuracy)}</td>
      <td className="py-1 pr-3">{pct(r.precision)}</td>
      <td className="py-1 pr-3">{pct(r.recall)}</td>
      <td className="py-1 pr-3">{pct(r.consistency)}</td>
      <td className="py-1 pr-3">{pct(r.grounded)}</td>
      <td className="py-1 pr-3">{r.failures}</td>
      <td className="py-1">{(r.avg_latency_ms / 1000).toFixed(1)}s</td>
    </tr>
  );
}
