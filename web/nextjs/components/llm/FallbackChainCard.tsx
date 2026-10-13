"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Loader2, Plus, Save, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { getLLMRouting, putLLMRouting } from "@/lib/api/llm-providers";
import { apiErrorMessage } from "@/lib/api/client";
import { cn } from "@/lib/utils/cn";
import type { LLMProviderConfig, LLMRouteStep, LLMRoutingMode } from "@/lib/types/llm-provider";

const inputClass =
  "h-9 rounded-md border border-input bg-background px-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

/**
 * The org's fallback chain: which providers take over, in order, when an
 * agent's own model cannot answer — a self-hosted model that is busy or
 * unreachable, a cloud key over quota. Only active providers can be chained.
 */
export function FallbackChainCard({ providers }: { providers: LLMProviderConfig[] }) {
  const queryClient = useQueryClient();
  const { data: routing, isLoading } = useQuery({ queryKey: ["llm-routing"], queryFn: getLLMRouting });

  const [enabled, setEnabled] = useState(true);
  const [mode, setMode] = useState<LLMRoutingMode>("fallback");
  const [chain, setChain] = useState<LLMRouteStep[]>([]);
  const [adding, setAdding] = useState("");

  const active = useMemo(() => providers.filter((p) => p.is_active), [providers]);
  const byId = useMemo(() => new Map(providers.map((p) => [p.id, p])), [providers]);

  useEffect(() => {
    if (!routing) return;
    setEnabled(routing.enabled);
    // An org that never saved a chain gets the safe default: agents keep
    // their own models and the chain is only a fallback.
    setMode(routing.chain.length === 0 && routing.source === "env" ? "fallback" : routing.mode);
    setChain(
      routing.chain.length > 0
        ? routing.chain
        : routing.effective.map((s) => ({ provider_config_id: s.provider_config_id }))
    );
  }, [routing]);

  const save = useMutation({
    mutationFn: () => putLLMRouting({ enabled, mode, chain }),
    onSuccess: (r) => {
      queryClient.setQueryData(["llm-routing"], r);
      toast.success(
        r.source === "org"
          ? `Fallback chain saved: ${r.effective.map((s) => s.name).join(" → ")}`
          : "Saved. No active providers in the chain, so the server's own providers answer."
      );
    },
    onError: (err) => toast.error(`Could not save: ${apiErrorMessage(err, "unknown error")}`),
  });

  function move(i: number, d: -1 | 1) {
    const j = i + d;
    if (j < 0 || j >= chain.length) return;
    const next = [...chain];
    [next[i], next[j]] = [next[j], next[i]];
    setChain(next);
  }

  const addable = active.filter((p) => !chain.some((s) => s.provider_config_id === p.id));

  return (
    <section className="rounded-xl border border-border bg-card p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Fallback chain</h2>
          <p className="mt-1 max-w-2xl text-xs text-muted-foreground">
            When a model cannot answer — Ollama busy or unreachable, a key over quota — the next
            provider here takes over, in order. Each step is tried once; the first that answers wins.
          </p>
        </div>
        <label className="inline-flex cursor-pointer items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            className="h-4 w-4 rounded border-input"
          />
          Active
        </label>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-6">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <div className={cn("mt-4 space-y-4", !enabled && "opacity-50")}>
          <fieldset className="grid gap-2 sm:grid-cols-2">
            <legend className="sr-only">When the chain is used</legend>
            {([
              {
                id: "fallback",
                title: "Only when an agent's model fails (recommended)",
                body: "Each agent keeps the model set on its Models tab. The chain steps in only when that model cannot answer.",
              },
              {
                id: "replace",
                title: "For every call",
                body: "Every agent call goes down this chain, and the chain's models replace each agent's own.",
              },
            ] as const).map((m) => (
              <label
                key={m.id}
                className={cn(
                  "cursor-pointer rounded-lg border p-3 text-sm",
                  mode === m.id ? "border-primary bg-primary/5" : "border-border hover:bg-muted/40"
                )}
              >
                <span className="flex items-center gap-2 font-medium">
                  <input
                    type="radio"
                    name="routing-mode"
                    checked={mode === m.id}
                    onChange={() => setMode(m.id)}
                    disabled={!enabled}
                  />
                  {m.title}
                </span>
                <span className="mt-1 block text-xs text-muted-foreground">{m.body}</span>
              </label>
            ))}
          </fieldset>

          {chain.length === 0 ? (
            <p className="rounded-lg border border-dashed border-border p-4 text-center text-sm text-muted-foreground">
              No providers in the chain yet. Add an active provider below.
            </p>
          ) : (
            <ol className="space-y-2">
              {chain.map((step, i) => {
                const p = byId.get(step.provider_config_id);
                const missing = !p || !p.is_active;
                return (
                  <li
                    key={`${step.provider_config_id}-${i}`}
                    className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-background p-2"
                  >
                    <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold">
                      {i + 1}
                    </span>
                    <span className="min-w-0 flex-1 text-sm">
                      <span className="font-medium">{p?.name ?? "Deleted provider"}</span>
                      <span className="text-muted-foreground"> · {p?.provider_type ?? "?"}</span>
                      {missing && <span className="ml-2 text-xs text-destructive">inactive — skipped</span>}
                    </span>
                    <input
                      value={step.model ?? ""}
                      onChange={(e) => {
                        const next = [...chain];
                        next[i] = { ...step, model: e.target.value || undefined };
                        setChain(next);
                      }}
                      placeholder={p?.default_model ?? "model"}
                      aria-label={`Model for step ${i + 1}`}
                      className={cn(inputClass, "w-48")}
                      disabled={!enabled}
                    />
                    <button type="button" onClick={() => move(i, -1)} disabled={i === 0 || !enabled} aria-label="Move up" className="rounded p-1.5 hover:bg-muted disabled:opacity-30">
                      <ArrowUp className="h-4 w-4" />
                    </button>
                    <button type="button" onClick={() => move(i, 1)} disabled={i === chain.length - 1 || !enabled} aria-label="Move down" className="rounded p-1.5 hover:bg-muted disabled:opacity-30">
                      <ArrowDown className="h-4 w-4" />
                    </button>
                    <button type="button" onClick={() => setChain(chain.filter((_, k) => k !== i))} disabled={!enabled} aria-label="Remove" className="rounded p-1.5 text-destructive hover:bg-destructive/10 disabled:opacity-30">
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </li>
                );
              })}
            </ol>
          )}

          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <select value={adding} onChange={(e) => setAdding(e.target.value)} className={inputClass} disabled={!enabled || addable.length === 0}>
                <option value="">{addable.length === 0 ? "No more active providers" : "Add a provider…"}</option>
                {addable.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name} ({p.provider_type})
                  </option>
                ))}
              </select>
              <button
                type="button"
                onClick={() => {
                  if (!adding) return;
                  setChain([...chain, { provider_config_id: adding }]);
                  setAdding("");
                }}
                disabled={!adding || !enabled}
                className="inline-flex h-9 items-center gap-1 rounded-md border border-input px-3 text-sm hover:bg-accent disabled:opacity-50"
              >
                <Plus className="h-4 w-4" /> Add
              </button>
            </div>
            <button
              type="button"
              onClick={() => save.mutate()}
              disabled={save.isPending}
              className="inline-flex h-9 items-center gap-2 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {save.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
              Save chain
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
