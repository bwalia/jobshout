"use client";

import { useEffect, useMemo, useState } from "react";
import { Bug, Loader2, Plus, Rocket, X } from "lucide-react";

import { AgentInputFields } from "@/components/task-manager/AgentInputFields";
import {
  defaultValuesForSchema,
  getAgentInputSchema,
  isSpecialistSchema,
  schemaValuesValid,
  validateSchemaValues,
} from "@/lib/agents/input-schemas";
import {
  hydrateLaunchValues,
  launchAgentForTask,
  type LaunchResult,
} from "@/lib/agents/launch";
import { fetchMailFormValues, mailFormIsBlank } from "@/lib/agents/mail-playbook";
import { apiErrorMessage } from "@/lib/api/client";
import { useSkills } from "@/lib/hooks/useSkills";
import { useCreateTaskRun } from "@/lib/hooks/useTaskRuns";
import type { Agent } from "@/lib/types/agent";
import type { Task } from "@/lib/types/project";
import type {
  CreateTaskRunRequest,
  TaskRun,
} from "@/lib/types/task-run";
import { toast } from "sonner";

const inputCls =
  "flex h-9 w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

interface RunTaskDialogProps {
  task: Task;
  agents: Agent[];
  onClose: () => void;
  onLaunched: (run: TaskRun) => void;
  /** When the launch hits a specialist API (pentest, review, articles, …). */
  onSpecialistLaunched?: (result: LaunchResult) => void;
}

/**
 * Run console: pick an agent first, then fill that agent's inputs (specialists
 * get their real fields; generic agents get skills/engine/prompt overrides).
 */
export function RunTaskDialog({
  task,
  agents,
  onClose,
  onLaunched,
  onSpecialistLaunched,
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
  const selectedAgent = useMemo(
    () => agents.find((a) => a.id === agentId) ?? null,
    [agents, agentId]
  );
  const schema = useMemo(
    () => getAgentInputSchema(selectedAgent),
    [selectedAgent]
  );
  const isSpecialist = isSpecialistSchema(schema);

  const [values, setValues] = useState<Record<string, string>>(() =>
    defaultValuesForSchema(getAgentInputSchema(null))
  );
  const [promptOverride, setPromptOverride] = useState("");
  const [engine, setEngine] = useState<"" | CreateTaskRunRequest["engine"]>("");
  const [modelProvider, setModelProvider] = useState("");
  const [modelName, setModelName] = useState("");
  const [selectedSlugs, setSelectedSlugs] = useState<string[]>([]);
  const [extraSlug, setExtraSlug] = useState("");
  const [debug, setDebug] = useState(false);
  const [launching, setLaunching] = useState(false);
  const [launchError, setLaunchError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  /** After a failed submit, keep live-validating so errors clear as the user fixes fields. */
  const [touchedSubmit, setTouchedSubmit] = useState(false);
  const [mailboxLoad, setMailboxLoad] = useState<"idle" | "loading" | "ready">(
    "idle"
  );

  useEffect(() => {
    setValues(hydrateLaunchValues(task, schema));
    setFieldErrors({});
    setTouchedSubmit(false);
    setLaunchError(null);
    // The generic-agent overrides are per-agent choices: switching agents
    // must not silently carry an engine/model/skills/debug config into the
    // payload of a different agent (or resurface it after a specialist
    // round-trip).
    setPromptOverride("");
    setEngine("");
    setModelProvider("");
    setModelName("");
    setSelectedSlugs([]);
    setExtraSlug("");
    setDebug(false);
    if (schema.kind !== "mail") {
      setMailboxLoad("idle");
      return;
    }
    setMailboxLoad("loading");
    let cancelled = false;
    void fetchMailFormValues()
      .then((saved) => {
        if (cancelled || !saved) return;
        setValues((prev) => (mailFormIsBlank(prev) ? { ...prev, ...saved } : prev));
      })
      .finally(() => {
        if (!cancelled) setMailboxLoad("ready");
      });
    return () => {
      cancelled = true;
    };
  }, [schema, task]);

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

  function onSpecialistFieldChange(key: string, value: string) {
    const next = { ...values, [key]: value };
    setValues(next);
    // Always refresh this field's error once the user has attempted to run,
    // or whenever that field already showed an error — so fixing a blank
    // required field clears the message immediately.
    if (touchedSubmit || fieldErrors[key]) {
      setFieldErrors(validateSchemaValues(schema, next));
    }
  }

  const busy = launching || createRun.isPending;
  const mailReady = schema.kind !== "mail" || mailboxLoad === "ready";
  const canRun =
    Boolean(selectedAgent) &&
    !busy &&
    mailReady &&
    (!isSpecialist || schemaValuesValid(schema, values));

  async function handleRun() {
    if (busy) return;
    if (!agentId || !selectedAgent) {
      setLaunchError("Choose an agent first");
      return;
    }

    setLaunchError(null);

    // Specialist path: schema fields only — never mix with generic overrides.
    if (isSpecialist) {
      if (!mailReady) {
        setLaunchError("Loading saved mailbox settings…");
        return;
      }
      setTouchedSubmit(true);
      const errs = validateSchemaValues(schema, values);
      setFieldErrors(errs);
      if (Object.keys(errs).length > 0) {
        setLaunchError("Fix the highlighted fields before running");
        return;
      }

      setLaunching(true);
      try {
        const result = await launchAgentForTask({
          agent: selectedAgent,
          task,
          values,
        });
        toast.success(
          result.kind === "researcher"
            ? "Research complete"
            : result.kind === "mail"
              ? result.sync_queued
                ? "Mailbox sync queued"
                : "Playbook saved. Connect Gmail on Mail Agent to sync."
              : result.kind === "images"
                ? "Image generated"
                : "Agent run started"
        );
        onSpecialistLaunched?.(result);
        onClose();
      } catch (err) {
        const msg = apiErrorMessage(err, "Failed to launch agent");
        setLaunchError(msg);
        toast.error(msg);
      } finally {
        setLaunching(false);
      }
      return;
    }

    // Generic path: task-run only — no schema field validation.
    setFieldErrors({});
    const payload: CreateTaskRunRequest = {
      agent_id: agentId,
      debug,
    };
    if (promptOverride.trim()) payload.prompt = promptOverride.trim();
    if (engine) payload.engine = engine;
    if (modelProvider.trim()) payload.model_provider = modelProvider.trim();
    if (modelName.trim()) payload.model_name = modelName.trim();
    if (selectedSlugs.length) payload.skill_slugs = selectedSlugs;

    setLaunching(true);
    try {
      const run = await createRun.mutateAsync({ taskId: task.id, payload });
      toast.success("Agent run started");
      onLaunched(run);
      onClose();
    } catch (err) {
      const msg = apiErrorMessage(err, "Failed to start run");
      setLaunchError(msg);
      // Hook may also toast; keep inline error for the dialog.
    } finally {
      setLaunching(false);
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={() => {
        if (!busy) onClose();
      }}
    >
      <div
        className="flex max-h-[90vh] w-full max-w-2xl flex-col rounded-xl border border-border bg-card shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-6 py-4">
          <div>
            <h2 className="font-display text-lg font-semibold">
              Run task with an agent
            </h2>
            <p className="mt-0.5 line-clamp-1 text-sm text-muted-foreground">
              {task.title}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="space-y-5 overflow-y-auto px-6 py-5 scrollbar-thin">
          <div className="space-y-1.5">
            <label className="text-sm font-medium">
              Agent <span className="text-destructive">*</span>
            </label>
            <select
              value={agentId}
              onChange={(e) => {
                setAgentId(e.target.value);
                setLaunchError(null);
              }}
              disabled={busy}
              className={inputCls}
            >
              <option value="">— Choose an agent —</option>
              {agents.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                  {a.metadata?.builtin
                    ? ` · ${a.metadata.builtin}`
                    : ` · ${a.role}`}
                </option>
              ))}
            </select>
            {selectedAgent && (
              <p className="text-xs text-muted-foreground">{schema.hint}</p>
            )}
          </div>

          {selectedAgent && schema.kind === "mail" && mailboxLoad === "loading" ? (
            <p className="text-xs text-muted-foreground">
              Loading saved mailbox settings…
            </p>
          ) : null}

          {selectedAgent && isSpecialist ? (
            <AgentInputFields
              fields={schema.fields}
              values={values}
              onChange={onSpecialistFieldChange}
              errors={fieldErrors}
              disabled={busy || !mailReady}
              autoFocusFirst
            />
          ) : null}

          {selectedAgent && !isSpecialist ? (
            <>
              <div className="space-y-2">
                <label className="text-sm font-medium">
                  Skills to load for this run
                </label>
                {availableSlugs.length > 0 ? (
                  <div className="flex flex-wrap gap-2">
                    {availableSlugs.map((slug) => {
                      const on = selectedSlugs.includes(slug);
                      return (
                        <button
                          key={slug}
                          type="button"
                          onClick={() => toggleSlug(slug)}
                          disabled={busy}
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
                    No skills in the registry yet — add one under Skills, or type
                    a slug below.
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
                    disabled={busy}
                    placeholder="add a skill by slug, e.g. web-search"
                    className={inputCls + " font-mono"}
                  />
                  <button
                    type="button"
                    onClick={addExtraSlug}
                    disabled={busy}
                    className="inline-flex h-9 shrink-0 items-center gap-1 rounded-md border border-border bg-background px-3 text-sm hover:bg-accent disabled:opacity-50"
                  >
                    <Plus className="h-4 w-4" /> Add
                  </button>
                </div>
              </div>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">Engine</label>
                  <select
                    value={engine ?? ""}
                    onChange={(e) =>
                      setEngine(
                        e.target.value as CreateTaskRunRequest["engine"] | ""
                      )
                    }
                    disabled={busy}
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
                    disabled={busy}
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
                    disabled={busy}
                    className={inputCls + " font-mono"}
                  />
                </div>
              </div>

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
                  disabled={busy}
                  className="w-full resize-none rounded-md border border-input bg-background px-3 py-2 font-mono text-xs placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                />
              </div>

              <label className="flex cursor-pointer items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={debug}
                  onChange={(e) => setDebug(e.target.checked)}
                  disabled={busy}
                  className="h-4 w-4 rounded border-input"
                />
                <Bug className="h-4 w-4 text-muted-foreground" />
                <span>
                  Debug — capture the full engine trace (tool calls, iterations,
                  tokens)
                </span>
              </label>
            </>
          ) : null}

          {launchError && (
            <div
              className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive"
              role="alert"
            >
              {launchError}
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 border-t border-border px-6 py-4">
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            className="inline-flex h-9 items-center rounded-md border border-border bg-background px-4 text-sm font-medium hover:bg-accent disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void handleRun()}
            disabled={!canRun}
            className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
          >
            {busy ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Rocket className="h-4 w-4" />
            )}
            {busy ? "Starting…" : "Run now"}
          </button>
        </div>
      </div>
    </div>
  );
}
