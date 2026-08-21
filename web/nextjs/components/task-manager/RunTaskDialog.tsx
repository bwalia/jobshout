"use client";

import { useMemo, useState } from "react";
import { Bug, Plus, Rocket, Trash2, X } from "lucide-react";

import { useSkills } from "@/lib/hooks/useSkills";
import { useCreateTaskRun } from "@/lib/hooks/useTaskRuns";
import type { Agent } from "@/lib/types/agent";
import type { Task } from "@/lib/types/project";
import type {
  CreateTaskRunRequest,
  TaskRun,
} from "@/lib/types/task-run";

const inputCls =
  "flex h-9 w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

interface KVRow {
  key: string;
  value: string;
}

interface RunTaskDialogProps {
  task: Task;
  agents: Agent[];
  onClose: () => void;
  onLaunched: (run: TaskRun) => void;
}

/**
 * The run console: launch a task with an agent, right now, and override
 * everything for this run only — the agent, the skills it loads, its model and
 * engine, free-form inputs, and the prompt itself, with an optional debug
 * trace. Nothing here mutates the task or the agent; it is all per-run.
 */
export function RunTaskDialog({
  task,
  agents,
  onClose,
  onLaunched,
}: RunTaskDialogProps) {
  const createRun = useCreateTaskRun();
  const { data: skills } = useSkills();

  const derivedPrompt = useMemo(() => {
    const parts = [task.title.trim()];
    if (task.description && task.description.trim()) {
      parts.push(task.description.trim());
    }
    return parts.join("\n\n");
  }, [task]);

  const [agentId, setAgentId] = useState(task.assigned_agent_id ?? "");
  const [promptOverride, setPromptOverride] = useState("");
  const [engine, setEngine] = useState<"" | CreateTaskRunRequest["engine"]>("");
  const [modelProvider, setModelProvider] = useState("");
  const [modelName, setModelName] = useState("");
  const [selectedSlugs, setSelectedSlugs] = useState<string[]>([]);
  const [extraSlug, setExtraSlug] = useState("");
  const [inputs, setInputs] = useState<KVRow[]>([]);
  const [debug, setDebug] = useState(false);

  const availableSlugs = useMemo(
    () => (skills ?? []).map((s) => s.slug),
    [skills]
  );

  function toggleSlug(slug: string) {
    setSelectedSlugs((prev) =>
      prev.includes(slug) ? prev.filter((s) => s !== slug) : [...prev, slug]
    );
  }

  function addExtraSlug() {
    const slug = extraSlug.trim().toLowerCase();
    if (slug && !selectedSlugs.includes(slug)) {
      setSelectedSlugs((prev) => [...prev, slug]);
    }
    setExtraSlug("");
  }

  const canRun = Boolean(agentId) && !createRun.isPending;

  async function handleRun() {
    if (!agentId) return;

    const inputMap: Record<string, unknown> = {};
    for (const { key, value } of inputs) {
      if (key.trim()) inputMap[key.trim()] = value;
    }

    const payload: CreateTaskRunRequest = {
      agent_id: agentId,
      debug,
    };
    if (promptOverride.trim()) payload.prompt = promptOverride.trim();
    if (engine) payload.engine = engine;
    if (modelProvider.trim()) payload.model_provider = modelProvider.trim();
    if (modelName.trim()) payload.model_name = modelName.trim();
    if (selectedSlugs.length) payload.skill_slugs = selectedSlugs;
    if (Object.keys(inputMap).length) payload.inputs = inputMap;

    try {
      const run = await createRun.mutateAsync({ taskId: task.id, payload });
      onLaunched(run);
      onClose();
    } catch {
      // toast surfaced by the hook
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <div
        className="flex max-h-[90vh] w-full max-w-2xl flex-col rounded-xl border border-border bg-card shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-6 py-4">
          <div>
            <h2 className="font-display text-lg font-semibold">Run task with an agent</h2>
            <p className="mt-0.5 text-sm text-muted-foreground line-clamp-1">
              {task.title}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="space-y-5 overflow-y-auto px-6 py-5 scrollbar-thin">
          {/* Agent */}
          <div className="space-y-1.5">
            <label className="text-sm font-medium">Agent</label>
            <select
              value={agentId}
              onChange={(e) => setAgentId(e.target.value)}
              className={inputCls}
            >
              <option value="">— Choose an agent —</option>
              {agents.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name} · {a.role}
                </option>
              ))}
            </select>
            {!task.assigned_agent_id && !agentId && (
              <p className="text-xs text-muted-foreground">
                This task has no assigned agent — pick one for this run.
              </p>
            )}
          </div>

          {/* Skills */}
          <div className="space-y-2">
            <label className="text-sm font-medium">Skills to load for this run</label>
            {availableSlugs.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {availableSlugs.map((slug) => {
                  const on = selectedSlugs.includes(slug);
                  return (
                    <button
                      key={slug}
                      type="button"
                      onClick={() => toggleSlug(slug)}
                      className={
                        "rounded-full border px-3 py-1 font-mono text-xs transition-colors " +
                        (on
                          ? "border-primary bg-primary/15 text-primary"
                          : "border-border bg-background text-muted-foreground hover:border-primary/50")
                      }
                    >
                      {slug}
                    </button>
                  );
                })}
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">
                No skills in the registry yet — add one under Skills, or type a slug below.
              </p>
            )}
            <div className="flex gap-2">
              <input
                type="text"
                value={extraSlug}
                onChange={(e) => setExtraSlug(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addExtraSlug();
                  }
                }}
                placeholder="add a skill by slug, e.g. web-search"
                className={inputCls + " font-mono"}
              />
              <button
                type="button"
                onClick={addExtraSlug}
                className="inline-flex h-9 shrink-0 items-center gap-1 rounded-md border border-border bg-background px-3 text-sm hover:bg-accent"
              >
                <Plus className="h-4 w-4" /> Add
              </button>
            </div>
            {selectedSlugs.filter((s) => !availableSlugs.includes(s)).length > 0 && (
              <div className="flex flex-wrap gap-2">
                {selectedSlugs
                  .filter((s) => !availableSlugs.includes(s))
                  .map((slug) => (
                    <span
                      key={slug}
                      className="inline-flex items-center gap-1 rounded-full border border-primary bg-primary/15 px-3 py-1 font-mono text-xs text-primary"
                    >
                      {slug}
                      <button type="button" onClick={() => toggleSlug(slug)} aria-label={`Remove ${slug}`}>
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
              </div>
            )}
          </div>

          {/* Engine + model overrides */}
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div className="space-y-1.5">
              <label className="text-sm font-medium">Engine</label>
              <select
                value={engine ?? ""}
                onChange={(e) =>
                  setEngine(e.target.value as CreateTaskRunRequest["engine"] | "")
                }
                className={inputCls}
              >
                <option value="">Agent default</option>
                <option value="go_native">go_native</option>
                <option value="langchain">langchain</option>
                <option value="langgraph">langgraph</option>
              </select>
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium">Model provider</label>
              <input
                type="text"
                value={modelProvider}
                onChange={(e) => setModelProvider(e.target.value)}
                placeholder="agent default"
                className={inputCls + " font-mono"}
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium">Model name</label>
              <input
                type="text"
                value={modelName}
                onChange={(e) => setModelName(e.target.value)}
                placeholder="agent default"
                className={inputCls + " font-mono"}
              />
            </div>
          </div>

          {/* Inputs (key/value) */}
          <div className="space-y-2">
            <label className="text-sm font-medium">Inputs (key / value)</label>
            {inputs.map((row, i) => (
              <div key={i} className="flex gap-2">
                <input
                  type="text"
                  value={row.key}
                  onChange={(e) =>
                    setInputs((prev) =>
                      prev.map((r, j) => (j === i ? { ...r, key: e.target.value } : r))
                    )
                  }
                  placeholder="key"
                  className={inputCls + " font-mono"}
                />
                <input
                  type="text"
                  value={row.value}
                  onChange={(e) =>
                    setInputs((prev) =>
                      prev.map((r, j) => (j === i ? { ...r, value: e.target.value } : r))
                    )
                  }
                  placeholder="value"
                  className={inputCls}
                />
                <button
                  type="button"
                  onClick={() => setInputs((prev) => prev.filter((_, j) => j !== i))}
                  className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border text-muted-foreground hover:bg-accent"
                  aria-label="Remove input"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            ))}
            <button
              type="button"
              onClick={() => setInputs((prev) => [...prev, { key: "", value: "" }])}
              className="inline-flex h-9 items-center gap-1 rounded-md border border-border bg-background px-3 text-sm hover:bg-accent"
            >
              <Plus className="h-4 w-4" /> Add input
            </button>
          </div>

          {/* Prompt override */}
          <div className="space-y-1.5">
            <label className="text-sm font-medium">
              Prompt override{" "}
              <span className="font-normal text-muted-foreground">
                (leave blank to use the task&apos;s title + description)
              </span>
            </label>
            <textarea
              rows={3}
              value={promptOverride}
              onChange={(e) => setPromptOverride(e.target.value)}
              placeholder={derivedPrompt}
              className="w-full resize-none rounded-md border border-input bg-background px-3 py-2 font-mono text-xs placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          {/* Debug */}
          <label className="flex cursor-pointer items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={debug}
              onChange={(e) => setDebug(e.target.checked)}
              className="h-4 w-4 rounded border-input"
            />
            <Bug className="h-4 w-4 text-muted-foreground" />
            <span>Debug — capture the full engine trace (tool calls, iterations, tokens)</span>
          </label>
        </div>

        <div className="flex justify-end gap-2 border-t border-border px-6 py-4">
          <button
            type="button"
            onClick={onClose}
            className="inline-flex h-9 items-center rounded-md border border-border bg-background px-4 text-sm font-medium hover:bg-accent"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleRun}
            disabled={!canRun}
            className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
          >
            <Rocket className="h-4 w-4" />
            {createRun.isPending ? "Starting…" : "Run now"}
          </button>
        </div>
      </div>
    </div>
  );
}
