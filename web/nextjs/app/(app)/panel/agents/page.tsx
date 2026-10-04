"use client";

import { Suspense, useMemo, useState } from "react";
import Link from "next/link";
import { AlertTriangle, Bot, CheckCircle2, Cpu, Download, Loader2, Plus, Search } from "lucide-react";
import { CreateAgentDialog } from "@/components/agent/CreateAgentDialog";
import { ImportAgentPackageDialog } from "@/components/agent/ImportAgentPackageDialog";
import { providerName } from "@/components/agent/ModelPicker";
import { agentHref } from "@/lib/agents/links";
import { useAgents } from "@/lib/hooks/useAgents";
import { useBlogConfig } from "@/lib/hooks/useBlog";
import { useRunFailures, type AgentRunStats } from "@/lib/hooks/useRunFailures";
import type { Agent } from "@/lib/types/agent";
import { cn } from "@/lib/utils/cn";
import { useRouter, useSearchParams } from "next/navigation";

/** The engine_config key the server reads the structured model from. */
const STRUCTURED_MODEL_KEY = "structured_model";

function isLive(agent: Agent): boolean {
  return agent.status === "active" || agent.status === "idle";
}

function modelLabel(provider: string | null, model: string | null, inherited?: string): string {
  if (provider === "auto") return "Automatic";
  if (!provider && !model) return inherited ? `Platform default → ${inherited}` : "Platform default";
  return [providerName(provider ?? ""), model || "provider default"].filter(Boolean).join(" · ");
}

function relative(at: string): string {
  const mins = Math.round((Date.now() - Date.parse(at)) / 60000);
  if (Number.isNaN(mins)) return "";
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

function LastRunCell({ stats }: { stats?: AgentRunStats }) {
  const last = stats?.last;
  if (!last) return <span className="text-muted-foreground">No runs yet</span>;
  const when = relative(last.at);
  if (last.status === "failed") {
    return (
      <span className="inline-flex items-center gap-1 text-destructive" title={last.error}>
        <AlertTriangle className="h-3.5 w-3.5" /> Failed {when}
      </span>
    );
  }
  if (last.status === "running") {
    return (
      <span className="inline-flex items-center gap-1 text-signal-live">
        <Loader2 className="h-3.5 w-3.5 animate-spin" /> Running
      </span>
    );
  }
  if (last.status === "completed") {
    return (
      <span className="inline-flex items-center gap-1 text-status-done">
        <CheckCircle2 className="h-3.5 w-3.5" /> Succeeded {when}
      </span>
    );
  }
  return <span className="text-muted-foreground">{when}</span>;
}

/**
 * All agents: one searchable list, so an agent is never more than one click
 * from the sidebar. Each row opens the agent's profile; the model cell opens
 * its Models tab.
 */
export default function AgentsPage() {
  // useSearchParams on a static route needs a Suspense boundary to build.
  return (
    <Suspense fallback={null}>
      <AgentsList />
    </Suspense>
  );
}

function AgentsList() {
  const router = useRouter();
  const params = useSearchParams();
  const { data, isLoading } = useAgents({ per_page: 100 });
  const { data: blogConfig } = useBlogConfig();
  const { byAgent } = useRunFailures();
  const [query, setQuery] = useState("");
  // ?new=1 (the palette's "New agent") opens the create dialog straight away.
  const [createOpen, setCreateOpen] = useState(params.get("new") === "1");
  const [importOpen, setImportOpen] = useState(params.get("import") === "1");

  const agents = useMemo(() => {
    const q = query.trim().toLowerCase();
    const all = [...(data?.data ?? [])].sort((a, b) => a.name.localeCompare(b.name));
    if (!q) return all;
    return all.filter((a) =>
      [a.name, a.role, a.model_name ?? "", a.model_provider ?? "", a.metadata?.builtin ?? ""]
        .join(" ")
        .toLowerCase()
        .includes(q)
    );
  }, [data, query]);

  return (
    <div className="space-y-5 p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl">All agents</h1>
          <p className="text-sm text-muted-foreground">
            Every agent in this workspace. Open one to change its model, run it, or see its tasks.
          </p>
        </div>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => setImportOpen(true)}
            className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card px-3 py-2 text-sm font-medium hover:bg-muted"
          >
            <Download className="h-4 w-4" /> Import
          </button>
          <button
            type="button"
            onClick={() => setCreateOpen(true)}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-2 text-sm font-semibold text-primary-foreground hover:opacity-90"
          >
            <Plus className="h-4 w-4" /> New agent
          </button>
        </div>
      </div>

      <label className="relative block max-w-md">
        <span className="sr-only">Search agents</span>
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search by name, role or model"
          className="h-10 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>

      {isLoading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : agents.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border p-10 text-center">
          <Bot className="mx-auto mb-2 h-6 w-6 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">
            {query ? `No agent matches “${query}”.` : "No agents yet."}
          </p>
          <button
            type="button"
            onClick={() => (query ? setQuery("") : setCreateOpen(true))}
            className="mt-3 inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-2 text-sm font-semibold text-primary-foreground hover:opacity-90"
          >
            {query ? "Clear search" : (<><Plus className="h-4 w-4" /> Create an agent</>)}
          </button>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border bg-card">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-border bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th scope="col" className="px-4 py-2.5 font-semibold">Agent</th>
                <th scope="col" className="px-4 py-2.5 font-semibold">Status</th>
                <th scope="col" className="px-4 py-2.5 font-semibold">Models</th>
                <th scope="col" className="px-4 py-2.5 font-semibold">Last run</th>
                <th scope="col" className="px-4 py-2.5 text-right font-semibold">Failed runs</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {agents.map((a) => {
                const writer = a.metadata?.builtin === "article_writer";
                const structured =
                  typeof a.engine_config?.[STRUCTURED_MODEL_KEY] === "string"
                    ? (a.engine_config[STRUCTURED_MODEL_KEY] as string)
                    : "";
                const stats = byAgent.get(a.id);
                return (
                  <tr
                    key={a.id}
                    onClick={() => router.push(agentHref(a.id))}
                    className="cursor-pointer hover:bg-muted/40"
                  >
                    <td className="px-4 py-3">
                      <Link
                        href={agentHref(a.id)}
                        onClick={(e) => e.stopPropagation()}
                        className="font-semibold text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      >
                        {a.name}
                      </Link>
                      <div className="text-xs text-muted-foreground">{a.role}</div>
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={cn(
                          "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium",
                          isLive(a) ? "bg-status-done/10 text-status-done" : "bg-muted text-muted-foreground"
                        )}
                      >
                        <span className={cn("h-1.5 w-1.5 rounded-full", isLive(a) ? "bg-status-done" : "bg-muted-foreground")} />
                        {isLive(a) ? "Live" : a.status === "paused" ? "Paused" : "Offline"}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <Link
                        href={agentHref(a.id, "models")}
                        onClick={(e) => e.stopPropagation()}
                        title="Change model"
                        className="group inline-flex flex-col text-xs hover:text-foreground"
                      >
                        <span className="inline-flex items-center gap-1 text-foreground group-hover:underline">
                          <Cpu className="h-3 w-3 text-muted-foreground" />
                          {writer && <span className="text-muted-foreground">Writing:</span>}
                          {modelLabel(a.model_provider, a.model_name, writer ? blogConfig?.effective_models?.prose : undefined)}
                        </span>
                        {writer && (
                          <span className="ml-4 text-muted-foreground group-hover:underline">
                            Structured:{" "}
                            {structured
                              ? `${providerName(blogConfig?.provider ?? "")} · ${structured}`
                              : modelLabel(null, null, blogConfig?.effective_models?.structured)}
                          </span>
                        )}
                      </Link>
                    </td>
                    <td className="px-4 py-3 text-xs">
                      <LastRunCell stats={stats} />
                    </td>
                    <td className="px-4 py-3 text-right">
                      {stats?.failed ? (
                        <span className="inline-flex min-w-6 justify-center rounded-full bg-destructive px-2 py-0.5 text-xs font-bold text-destructive-foreground">
                          {stats.failed}
                        </span>
                      ) : (
                        <span className="text-muted-foreground">0</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <CreateAgentDialog open={createOpen} onClose={() => setCreateOpen(false)} />
      <ImportAgentPackageDialog
        open={importOpen}
        onClose={() => setImportOpen(false)}
        onImported={(id) => router.push(agentHref(id))}
      />
    </div>
  );
}
