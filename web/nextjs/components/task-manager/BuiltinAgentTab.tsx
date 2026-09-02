"use client";

import { useEffect, useMemo, useState, type ComponentType } from "react";
import { Loader2, Rocket } from "lucide-react";
import { toast } from "sonner";

import { AgentInputFields } from "@/components/task-manager/AgentInputFields";
import {
  defaultValuesForSchema,
  schemaFromWire,
  schemaValuesValid,
  validateSchemaValues,
  type WireSchema,
} from "@/lib/agents/input-schemas";
import { launchAgent, type LaunchResult } from "@/lib/agents/launch";
import { fetchMailFormValues, mailFormIsBlank } from "@/lib/agents/mail-playbook";
import { apiErrorMessage } from "@/lib/api/client";
import type { Agent } from "@/lib/types/agent";
import type { Project } from "@/lib/types/project";

const inputCls =
  "flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

/**
 * Generic Task Manager specialist tab.
 *
 * If AGENT_CLIENTS registered a client, that UI is the tab (schema still used
 * in New task / Run task / chat). Otherwise: schema form + Run.
 */
export function BuiltinAgentTab({
  wire,
  agent,
  projects,
  Client,
  onLaunched,
}: {
  wire: WireSchema;
  agent: Agent | undefined;
  projects: Project[];
  Client?: ComponentType;
  onLaunched: (result: LaunchResult) => void;
}) {
  const schema = useMemo(() => schemaFromWire(wire), [wire]);
  const ownsTab = Boolean(Client);
  const [values, setValues] = useState(() => defaultValuesForSchema(schema));
  const [projectId, setProjectId] = useState(projects[0]?.id ?? "");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [launching, setLaunching] = useState(false);
  const [mailboxLoad, setMailboxLoad] = useState<"idle" | "loading" | "ready">(
    "idle"
  );

  useEffect(() => {
    if (ownsTab) return;
    setValues(defaultValuesForSchema(schema));
    setFieldErrors({});
    if (schema.prefill !== "mailbox") {
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
  }, [schema, ownsTab]);

  useEffect(() => {
    if (!projectId && projects[0]) setProjectId(projects[0].id);
  }, [projectId, projects]);

  const mailReady = schema.prefill !== "mailbox" || mailboxLoad === "ready";
  const ready =
    Boolean(agent) && Boolean(projectId) && mailReady && schemaValuesValid(schema, values);

  async function handleRun() {
    if (!agent || launching) return;
    const errs = validateSchemaValues(schema, values);
    setFieldErrors(errs);
    if (Object.keys(errs).length > 0 || !projectId) return;
    setLaunching(true);
    try {
      const result = await launchAgent({
        agent,
        projectId,
        values,
      });
      toast.success(result.message || "Agent run started");
      onLaunched(result);
    } catch (err) {
      toast.error(apiErrorMessage(err, "Failed to launch agent"));
    } finally {
      setLaunching(false);
    }
  }

  return (
    <div className="space-y-6">
      <h2 className="text-lg font-semibold tracking-tight">
        {wire.label || agent?.name || "Agent"}
      </h2>

      {ownsTab && Client ? (
        !agent ? (
          <p className="text-sm text-muted-foreground">
            This agent is not in the organisation yet.
          </p>
        ) : (
          <Client />
        )
      ) : (
        <>
          {schema.hint ? (
            <p className="text-sm text-muted-foreground">{schema.hint}</p>
          ) : null}

          {!agent ? (
            <p className="text-sm text-muted-foreground">
              This agent is not in the organisation yet.
            </p>
          ) : (
            <div className="space-y-4 rounded-lg border border-border bg-card p-5">
              {schema.prefill === "mailbox" && mailboxLoad === "loading" ? (
                <p className="text-xs text-muted-foreground">
                  Loading saved mailbox settings…
                </p>
              ) : null}
              <AgentInputFields
                fields={schema.fields}
                values={values}
                onChange={(key, value) => {
                  const next = { ...values, [key]: value };
                  setValues(next);
                  if (fieldErrors[key]) {
                    setFieldErrors(validateSchemaValues(schema, next));
                  }
                }}
                errors={fieldErrors}
                disabled={launching || !mailReady}
                autoFocusFirst
              />
              <div className="flex flex-wrap items-end gap-3 border-t border-border pt-4">
                <div className="min-w-[180px] flex-1 space-y-1.5">
                  <label className="text-sm font-medium" htmlFor="tab-project">
                    Project
                  </label>
                  <select
                    id="tab-project"
                    value={projectId}
                    onChange={(e) => setProjectId(e.target.value)}
                    className={inputCls}
                    disabled={launching}
                  >
                    {projects.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                </div>
                <button
                  type="button"
                  onClick={() => void handleRun()}
                  disabled={!ready || launching}
                  className="inline-flex h-10 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
                >
                  {launching ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Rocket className="h-4 w-4" />
                  )}
                  Run
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
