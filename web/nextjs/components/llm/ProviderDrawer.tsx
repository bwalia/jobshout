"use client";

import { useEffect } from "react";
import { AlertTriangle, CheckCircle2, X } from "lucide-react";
import { providerName } from "@/components/agent/ModelPicker";
import { useAvailableModels } from "@/lib/hooks/useLLMProviders";
import type { ProviderModelGroup } from "@/lib/types/llm-provider";
import { cn } from "@/lib/utils/cn";

/** How a provider's model list was obtained, in plain words. */
const SOURCE_LABEL: Record<ProviderModelGroup["source"], string> = {
  discovered: "Live — asked the provider just now",
  static: "Built-in list — this provider is not asked for its models",
  stale: "Last known list — the provider did not answer this time",
};

/** The hint shown when a reachable provider reports no models. */
export function noModelsHint(provider: string): string {
  if (provider === "ollama") {
    return "Ollama is reachable but reports no models. Pull models on the host (e.g. `ollama pull muse-glimmer:latest`).";
  }
  return `${providerName(provider)} is reachable but reports no models. Check the account's model access.`;
}

/**
 * A system (environment-configured) provider: whether it is reachable, and
 * every model it currently offers to agents. Read-only — these providers are
 * set by the server's environment, not on this page.
 */
export function ProviderDrawer({ provider, onClose }: { provider: string; onClose: () => void }) {
  const { data, isLoading, isError, refetch, isFetching } = useAvailableModels();
  const group = data?.providers.find((p) => p.provider === provider);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const reachable = group && !group.error;
  const empty = group && !group.error && group.models.length === 0;

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/40" onClick={onClose}>
      <aside
        role="dialog"
        aria-modal="true"
        aria-labelledby="provider-drawer-title"
        onClick={(e) => e.stopPropagation()}
        className="flex h-full w-full max-w-md flex-col border-l border-border bg-card shadow-2xl"
      >
        <header className="flex shrink-0 items-center justify-between border-b border-border px-5 py-4">
          <h2 id="provider-drawer-title" className="text-base font-semibold">
            {providerName(provider)}
          </h2>
          <button type="button" onClick={onClose} aria-label="Close" className="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground">
            <X className="h-5 w-5" />
          </button>
        </header>

        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-5 py-4 scrollbar-thin">
          <dl className="space-y-3 text-sm">
            <div>
              <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Connection</dt>
              <dd className="mt-1">
                {isLoading ? (
                  "Checking…"
                ) : isError ? (
                  <span className="text-destructive">Could not load the model list from the server.</span>
                ) : !group ? (
                  <span className="text-muted-foreground">Not registered on this server, so agents cannot use it.</span>
                ) : reachable ? (
                  <span className="inline-flex items-center gap-1.5 text-status-done">
                    <CheckCircle2 className="h-4 w-4" /> Reachable{group.is_default ? " · the default provider" : ""}
                  </span>
                ) : (
                  <span className="inline-flex items-start gap-1.5 text-destructive">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" /> Not reachable: {group.error}
                  </span>
                )}
              </dd>
            </div>
            {group && (
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Model list</dt>
                <dd className="mt-1 text-muted-foreground">{SOURCE_LABEL[group.source]}</dd>
              </div>
            )}
            <div>
              <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Configuration</dt>
              <dd className="mt-1 text-muted-foreground">
                Set by the server&apos;s environment (base URL, keys). It cannot be edited here; add a custom
                provider below to use a different endpoint.
              </dd>
            </div>
          </dl>

          {empty && (
            <p role="alert" className="flex items-start gap-2 rounded-lg border border-signal-warn/40 bg-signal-warn/10 p-3 text-sm">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-signal-warn" />
              <span>{noModelsHint(provider)}</span>
            </p>
          )}

          {group && group.models.length > 0 && (
            <section>
              <h3 className="text-sm font-semibold">
                Models agents can use <span className="font-normal text-muted-foreground">({group.models.length})</span>
              </h3>
              <ul className="mt-2 divide-y divide-border rounded-lg border border-border">
                {group.models.map((m) => (
                  <li key={m.name} className="px-3 py-2">
                    <p className="font-mono text-sm">{m.name}</p>
                    <p className="mt-0.5 flex flex-wrap gap-1.5 text-xs text-muted-foreground">
                      {m.parameter_size && <span>{m.parameter_size}</span>}
                      {m.context_tokens > 0 && <span>{Math.round(m.context_tokens / 1024)}k context</span>}
                      {m.supports_tools && <span className={cn("rounded bg-muted px-1.5")}>tools</span>}
                      {m.supports_vision && <span className="rounded bg-muted px-1.5">vision</span>}
                    </p>
                  </li>
                ))}
              </ul>
            </section>
          )}
        </div>

        <footer className="shrink-0 border-t border-border px-5 py-3">
          <button
            type="button"
            onClick={() => void refetch()}
            disabled={isFetching}
            className="inline-flex h-9 items-center rounded-md border border-border px-3 text-sm font-medium hover:bg-muted disabled:opacity-50"
          >
            {isFetching ? "Checking…" : "Check again"}
          </button>
        </footer>
      </aside>
    </div>
  );
}
