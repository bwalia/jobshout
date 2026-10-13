"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Bot, Loader2, Play, Search, Sparkles, X } from "lucide-react";
import { toast } from "sonner";

import { AgentInputFields } from "@/components/task-manager/AgentInputFields";
import {
  defaultValuesForSchema,
  getAgentInputSchema,
  schemaValuesValid,
  validateSchemaValues,
} from "@/lib/agents/input-schemas";
import { launchAgent } from "@/lib/agents/launch";
import { apiErrorMessage } from "@/lib/api/client";
import { useAgents } from "@/lib/hooks/useAgents";
import { useAgentSchemas } from "@/lib/hooks/useAgentSchemas";
import { useProjects } from "@/lib/hooks/useProjects";
import { taskKeys } from "@/lib/hooks/useTasks";
import { useUiStore } from "@/lib/store/ui-store";
import { cn } from "@/lib/utils/cn";
import type { Agent } from "@/lib/types/agent";

const inputCls =
  "flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

/**
 * The one place to run any agent: pick it, fill its form, Run. Opened from the
 * header's "Run a task" button on every screen, ⌘K → "Run an agent", or an
 * agent's own Run button (which skips the picker).
 *
 * It launches through POST /tasks/launch like New task and chat, so the run
 * gets a board task and always lands in the same place: that task, with the
 * run open.
 */
export function RunAgentDialog() {
  const { runDialog, closeRunDialog } = useUiStore();
  if (!runDialog.open) return null;
  return <RunAgentDialogBody initialAgentId={runDialog.agentId} onClose={closeRunDialog} />;
}

function RunAgentDialogBody({ initialAgentId, onClose }: { initialAgentId: string | null; onClose: () => void }) {
  const router = useRouter();
  const qc = useQueryClient();
  const { data: agentsResp, isLoading: agentsLoading } = useAgents({ per_page: 100 });
  const { data: catalog } = useAgentSchemas();
  const { data: projectsResp } = useProjects({ per_page: 100 });
  const agents = useMemo(() => agentsResp?.data ?? [], [agentsResp]);
  const projects = useMemo(() => projectsResp?.data ?? [], [projectsResp]);

  const [agentId, setAgentId] = useState<string | null>(initialAgentId);
  const [query, setQuery] = useState("");
  const agent = agents.find((a) => a.id === agentId) ?? null;

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = q
      ? agents.filter((a) => `${a.name} ${a.role} ${a.description ?? ""}`.toLowerCase().includes(q))
      : agents;
    // Specialists first: they are what most people come here to run.
    return [...list].sort((a, b) => Number(Boolean(b.metadata?.builtin)) - Number(Boolean(a.metadata?.builtin)) || a.name.localeCompare(b.name));
  }, [agents, query]);

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/60 p-4 pt-[10vh] backdrop-blur-sm" onMouseDown={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Run a task"
        onMouseDown={(e) => e.stopPropagation()}
        className="flex max-h-[80vh] w-full max-w-xl flex-col overflow-hidden rounded-xl border border-border bg-card shadow-2xl"
      >
        <div className="flex items-center gap-2 border-b border-border px-4 py-3">
          {agent && !initialAgentId && (
            <button type="button" onClick={() => setAgentId(null)} className="rounded p-1 hover:bg-muted" aria-label="Choose another agent">
              <ArrowLeft className="h-4 w-4" />
            </button>
          )}
          <Play className="h-4 w-4 text-primary" />
          <h2 className="flex-1 text-sm font-semibold">{agent ? `Run ${agent.name}` : "Run a task — choose an agent"}</h2>
          <button type="button" onClick={onClose} className="rounded p-1 hover:bg-muted" aria-label="Close">
            <X className="h-4 w-4" />
          </button>
        </div>

        {!agent ? (
          <>
            <div className="relative border-b border-border">
              <Search className="absolute left-4 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <input
                autoFocus
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search agents…"
                className="h-11 w-full bg-transparent pl-10 pr-4 text-sm focus:outline-none"
              />
            </div>
            <ul className="flex-1 overflow-y-auto p-2">
              {agentsLoading && (
                <li className="flex justify-center py-6">
                  <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                </li>
              )}
              {!agentsLoading && filtered.length === 0 && (
                <li className="py-6 text-center text-sm text-muted-foreground">No agents match.</li>
              )}
              {filtered.map((a) => (
                <li key={a.id}>
                  <button
                    type="button"
                    onClick={() => setAgentId(a.id)}
                    className="flex w-full items-start gap-3 rounded-lg px-3 py-2.5 text-left hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
                  >
                    <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                      {a.metadata?.builtin ? <Sparkles className="h-4 w-4" /> : <Bot className="h-4 w-4" />}
                    </span>
                    <span className="min-w-0">
                      <span className="block text-sm font-medium">{a.name}</span>
                      <span className="block truncate text-xs text-muted-foreground">{a.description || a.role}</span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          </>
        ) : (
          <RunForm
            agent={agent}
            catalog={catalog ?? []}
            projects={projects}
            onLaunched={(taskId, runId) => {
              void qc.invalidateQueries({ queryKey: taskKeys.all });
              onClose();
              const params = new URLSearchParams({ task: taskId });
              if (runId) params.set("run", runId);
              router.push(`/panel/tasks?${params.toString()}`);
            }}
          />
        )}
      </div>
    </div>
  );
}

function RunForm({
  agent,
  catalog,
  projects,
  onLaunched,
}: {
  agent: Agent;
  catalog: Parameters<typeof getAgentInputSchema>[1];
  projects: { id: string; name: string }[];
  onLaunched: (taskId: string, runId?: string) => void;
}) {
  const schema = useMemo(() => getAgentInputSchema(agent, catalog), [agent, catalog]);
  const [values, setValues] = useState(() => defaultValuesForSchema(schema));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [projectId, setProjectId] = useState(projects[0]?.id ?? "");
  const [launching, setLaunching] = useState(false);

  useEffect(() => {
    setValues(defaultValuesForSchema(schema));
    setErrors({});
  }, [schema]);
  useEffect(() => {
    if (!projectId && projects[0]) setProjectId(projects[0].id);
  }, [projectId, projects]);

  const ready = Boolean(projectId) && schemaValuesValid(schema, values);

  async function run(e: React.FormEvent) {
    e.preventDefault();
    const errs = validateSchemaValues(schema, values);
    setErrors(errs);
    if (Object.keys(errs).length > 0 || !projectId || launching) return;
    setLaunching(true);
    try {
      const result = await launchAgent({ agent, projectId, values });
      toast.success(result.message || `${agent.name} started`);
      if (result.task) onLaunched(result.task.id, result.run_id ?? undefined);
    } catch (err) {
      toast.error(apiErrorMessage(err, "Could not start the run"));
    } finally {
      setLaunching(false);
    }
  }

  return (
    <form onSubmit={run} className="flex min-h-0 flex-1 flex-col">
      <div className="flex-1 space-y-4 overflow-y-auto p-4">
        {schema.hint ? <p className="text-sm text-muted-foreground">{schema.hint}</p> : null}
        <AgentInputFields
          fields={schema.fields}
          values={values}
          onChange={(key, value) => {
            const next = { ...values, [key]: value };
            setValues(next);
            if (errors[key]) setErrors(validateSchemaValues(schema, next));
          }}
          errors={errors}
          disabled={launching}
          autoFocusFirst
        />
        <div className="space-y-1.5">
          <label htmlFor="run-project" className="text-sm font-medium">
            Project
          </label>
          {projects.length === 0 ? (
            <p className="text-sm text-muted-foreground">Create a project first — every run is filed under one.</p>
          ) : (
            <select id="run-project" value={projectId} onChange={(e) => setProjectId(e.target.value)} className={inputCls} disabled={launching}>
              {projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          )}
        </div>
      </div>
      <div className="flex items-center justify-between gap-3 border-t border-border px-4 py-3">
        <p className="text-xs text-muted-foreground">You&apos;ll see the run on its task as it works.</p>
        <button
          type="submit"
          disabled={!ready || launching}
          className={cn(
            "inline-flex h-10 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
          )}
        >
          {launching ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
          Run now
        </button>
      </div>
    </form>
  );
}
