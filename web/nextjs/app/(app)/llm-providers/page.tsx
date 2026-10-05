"use client";

import { useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { AlertTriangle, ChevronRight } from "lucide-react";
import { LLMBenchmarksPanel } from "@/components/panels/LLMBenchmarksPanel";
import { ProviderDrawer } from "@/components/llm/ProviderDrawer";
import { cn } from "@/lib/utils/cn";
import {
  useAvailableModels,
  useBuiltinProviders,
  useLLMProviders,
  useCreateLLMProvider,
  useUpdateLLMProvider,
  useDeleteLLMProvider,
} from "@/lib/hooks/useLLMProviders";
import type { CreateLLMProviderRequest, LLMProviderType } from "@/lib/types/llm-provider";

const PROVIDER_PRESETS: Record<LLMProviderType, { base_url: string; models: string[]; key_placeholder?: string }> = {
  ollama: {
    base_url: "http://localhost:11434",
    models: ["llama3", "llama3.1", "mistral", "codellama", "phi3", "gemma2", "qwen2"],
  },
  openai: {
    base_url: "https://api.openai.com",
    models: ["gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-3.5-turbo", "o1-preview", "o1-mini"],
  },
  claude: {
    base_url: "https://api.anthropic.com",
    models: [
      "claude-sonnet-4-20250514",
      "claude-opus-4-20250514",
      "claude-haiku-4-20250514",
      "claude-3-5-sonnet-20241022",
    ],
  },
  gemini: {
    base_url: "https://generativelanguage.googleapis.com",
    models: ["gemini-flash-latest", "gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite"],
    key_placeholder: "AIza...",
  },
};

/**
 * Defence in depth: the server masks stored keys on every read now, but this
 * page must never print a full secret even if it receives one (older cached
 * responses, future regressions).
 */
function maskedKey(key: string): string {
  if (key.includes("****")) return key;
  return key.length > 4 ? `••••${key.slice(-4)}` : "••••";
}

type PageTab = "providers" | "benchmarks";

/**
 * Models & providers: the providers agents can use, and (as a tab) the LLM
 * benchmarks of every call they made.
 */
export default function LLMProvidersPage() {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const tab: PageTab = params.get("tab") === "benchmarks" ? "benchmarks" : "providers";
  const setTab = (t: PageTab) => router.replace(t === "providers" ? pathname : `${pathname}?tab=${t}`, { scroll: false });

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Models &amp; providers</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Where agents get their models, and how each model has performed.
        </p>
      </div>
      <div className="border-b border-border" role="tablist" aria-label="Models and providers">
        {([
          { id: "providers", label: "Providers" },
          { id: "benchmarks", label: "Benchmarks" },
        ] as const).map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => setTab(t.id)}
            className={cn(
              "-mb-px border-b-2 px-4 py-2.5 text-sm font-medium",
              tab === t.id ? "border-primary text-foreground" : "border-transparent text-muted-foreground hover:text-foreground"
            )}
          >
            {t.label}
          </button>
        ))}
      </div>
      {tab === "benchmarks" ? <LLMBenchmarksPanel /> : <ProvidersTab />}
    </div>
  );
}

function ProvidersTab() {
  const { data: builtinProviders } = useBuiltinProviders();
  const { data: available } = useAvailableModels();
  const [openProvider, setOpenProvider] = useState<string | null>(null);
  const { data: providers, isLoading } = useLLMProviders();
  const createMutation = useCreateLLMProvider();
  const updateMutation = useUpdateLLMProvider();
  const deleteMutation = useDeleteLLMProvider();

  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<CreateLLMProviderRequest>({
    name: "",
    provider_type: "ollama",
    base_url: "http://localhost:11434",
    default_model: "llama3",
    api_key: "",
    is_default: false,
  });

  function handleProviderTypeChange(type: LLMProviderType) {
    const preset = PROVIDER_PRESETS[type];
    setForm({
      ...form,
      provider_type: type,
      base_url: preset.base_url,
      default_model: preset.models[0],
    });
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    await createMutation.mutateAsync(form);
    setShowForm(false);
    setForm({
      name: "",
      provider_type: "ollama",
      base_url: "http://localhost:11434",
      default_model: "llama3",
      api_key: "",
      is_default: false,
    });
  }

  return (
    <div className="max-w-4xl space-y-8">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          Configure LLM providers (local and cloud) for your agents and workflows. Click a system
          provider to see the models it offers.
        </p>
        <button
          onClick={() => setShowForm(!showForm)}
          className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
        >
          {showForm ? "Cancel" : "Add Provider"}
        </button>
      </div>

      {/* Builtin (env-based) providers */}
      {builtinProviders && builtinProviders.length > 0 && (
        <section className="rounded-xl border border-border bg-card p-6">
          <h2 className="text-base font-semibold">System Providers (Environment)</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            These providers are configured via environment variables and always available.
          </p>
          <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
            {builtinProviders.map((bp) => {
              const group = available?.providers.find((p) => p.provider === bp.name);
              const count = group?.models.length;
              const problem = group?.error ? "Not reachable" : group && count === 0 ? "No models" : null;
              return (
                <button
                  type="button"
                  key={bp.name}
                  onClick={() => setOpenProvider(bp.name)}
                  aria-label={`${bp.name}: show models`}
                  className="flex items-center justify-between rounded-lg border border-border bg-background p-4 text-left transition-colors hover:border-primary/40 hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-sm font-bold uppercase text-primary">
                      {bp.name.charAt(0)}
                    </div>
                    <div className="min-w-0">
                      <p className="text-sm font-medium capitalize">{bp.name}</p>
                      <p className={cn("text-xs", problem ? "text-destructive" : "text-muted-foreground")}>
                        {problem ? (
                          <span className="inline-flex items-center gap-1"><AlertTriangle className="h-3 w-3" /> {problem}</span>
                        ) : (
                          <>
                            {bp.is_default ? "Default" : "Available"}
                            {typeof count === "number" ? ` · ${count} model${count === 1 ? "" : "s"}` : ""}
                          </>
                        )}
                      </p>
                    </div>
                  </div>
                  <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                </button>
              );
            })}
          </div>
        </section>
      )}

      {/* Add new provider form */}
      {showForm && (
        <section className="rounded-xl border border-border bg-card p-6">
          <h2 className="text-base font-semibold">Add New Provider</h2>
          <form onSubmit={handleSubmit} className="mt-4 space-y-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <label className="text-sm font-medium">Name</label>
                <input
                  type="text"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="My Local Ollama"
                  required
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium">Provider Type</label>
                <select
                  value={form.provider_type}
                  onChange={(e) =>
                    handleProviderTypeChange(e.target.value as LLMProviderType)
                  }
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="ollama">Ollama (Local)</option>
                  <option value="openai">OpenAI / Compatible</option>
                  <option value="claude">Claude (Anthropic)</option>
                  <option value="gemini">Gemini (Google)</option>
                </select>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium">Base URL</label>
                <input
                  type="text"
                  value={form.base_url}
                  onChange={(e) => setForm({ ...form, base_url: e.target.value })}
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium">Default Model</label>
                <select
                  value={form.default_model}
                  onChange={(e) => setForm({ ...form, default_model: e.target.value })}
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  {PROVIDER_PRESETS[form.provider_type]?.models.map((m) => (
                    <option key={m} value={m}>
                      {m}
                    </option>
                  ))}
                </select>
              </div>

              {form.provider_type !== "ollama" && (
                <div className="space-y-2 sm:col-span-2">
                  <label className="text-sm font-medium">API Key</label>
                  <input
                    type="password"
                    value={form.api_key}
                    onChange={(e) => setForm({ ...form, api_key: e.target.value })}
                    placeholder={PROVIDER_PRESETS[form.provider_type]?.key_placeholder ?? "sk-..."}
                    className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                </div>
              )}
            </div>

            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="is-default"
                checked={form.is_default}
                onChange={(e) => setForm({ ...form, is_default: e.target.checked })}
                className="h-4 w-4 rounded border-input"
              />
              <label htmlFor="is-default" className="text-sm">
                Set as default provider
              </label>
            </div>

            <button
              type="submit"
              disabled={createMutation.isPending}
              className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {createMutation.isPending ? "Creating..." : "Create Provider"}
            </button>
          </form>
        </section>
      )}

      {/* User-configured providers */}
      <section className="space-y-4">
        <h2 className="text-base font-semibold">Custom Providers</h2>
        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        ) : !providers || providers.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border p-12 text-center">
            <p className="text-sm text-muted-foreground">
              No custom providers configured. Add one to get started.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4">
            {providers.map((p) => (
              <div
                key={p.id}
                className="flex items-center justify-between rounded-xl border border-border bg-card p-5"
              >
                <div className="flex items-center gap-4">
                  <div
                    className={`flex h-12 w-12 items-center justify-center rounded-lg text-lg font-bold uppercase ${
                      p.provider_type === "ollama"
                        ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
                        : p.provider_type === "claude"
                        ? "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400"
                        : p.provider_type === "gemini"
                        ? "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400"
                        : "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400"
                    }`}
                  >
                    {p.provider_type.charAt(0)}
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-semibold">{p.name}</p>
                      {p.is_default && (
                        <span className="rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700 dark:bg-green-900/30 dark:text-green-400">
                          Default
                        </span>
                      )}
                      {!p.is_active && (
                        <span className="rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700 dark:bg-red-900/30 dark:text-red-400">
                          Inactive
                        </span>
                      )}
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {p.provider_type} &middot; {p.default_model} &middot; {p.base_url}
                    </p>
                    {p.api_key && (
                      <p className="text-xs text-muted-foreground">
                        Key: {maskedKey(p.api_key)}
                      </p>
                    )}
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  {!p.is_default && (
                    <button
                      onClick={() =>
                        updateMutation.mutate({
                          id: p.id,
                          payload: { is_default: true },
                        })
                      }
                      className="inline-flex h-8 items-center rounded-md border border-input bg-background px-3 text-xs font-medium hover:bg-accent"
                    >
                      Set Default
                    </button>
                  )}
                  <button
                    onClick={() =>
                      updateMutation.mutate({
                        id: p.id,
                        payload: { is_active: !p.is_active },
                      })
                    }
                    className="inline-flex h-8 items-center rounded-md border border-input bg-background px-3 text-xs font-medium hover:bg-accent"
                  >
                    {p.is_active ? "Disable" : "Enable"}
                  </button>
                  <button
                    onClick={() => {
                      if (confirm("Delete this provider?")) {
                        deleteMutation.mutate(p.id);
                      }
                    }}
                    className="inline-flex h-8 items-center rounded-md border border-red-200 bg-background px-3 text-xs font-medium text-red-600 hover:bg-red-50 dark:border-red-800 dark:hover:bg-red-900/20"
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
      {openProvider && <ProviderDrawer provider={openProvider} onClose={() => setOpenProvider(null)} />}
    </div>
  );
}
