"use client";

import { useState } from "react";
import {
  useScheduledTasks,
  useCreateScheduledTask,
  useUpdateScheduledTask,
  useDeleteScheduledTask,
} from "@/lib/hooks/useScheduler";
import { useAgents } from "@/lib/hooks/useAgents";
import { useLLMProviders } from "@/lib/hooks/useLLMProviders";
import type { CreateScheduledTaskRequest } from "@/lib/types/scheduler";

const CRON_PRESETS = [
  { label: "Every 5 minutes", value: "*/5 * * * *" },
  { label: "Every 15 minutes", value: "*/15 * * * *" },
  { label: "Every hour", value: "0 * * * *" },
  { label: "Every 6 hours", value: "0 */6 * * *" },
  { label: "Daily at midnight", value: "0 0 * * *" },
  { label: "Daily at 9 AM", value: "0 9 * * *" },
  { label: "Weekly (Monday 9 AM)", value: "0 9 * * 1" },
  { label: "Custom", value: "" },
];

const PRIORITY_COLORS: Record<string, string> = {
  low: "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300",
  medium: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  high: "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400",
  critical: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
};

const STATUS_COLORS: Record<string, string> = {
  active: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  paused: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  completed: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  failed: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
};

export default function SchedulerPage() {
  const { data: tasksResponse, isLoading } = useScheduledTasks();
  const { data: agentsResponse } = useAgents();
  const { data: providers } = useLLMProviders();
  const createMutation = useCreateScheduledTask();
  const updateMutation = useUpdateScheduledTask();
  const deleteMutation = useDeleteScheduledTask();

  const [showForm, setShowForm] = useState(false);
  const [cronPreset, setCronPreset] = useState("0 * * * *");
  const [form, setForm] = useState<CreateScheduledTaskRequest>({
    name: "",
    task_type: "agent",
    input_prompt: "",
    schedule_type: "cron",
    cron_expression: "0 * * * *",
    priority: "medium",
    retry_on_failure: false,
    max_retries: 3,
    tags: [],
  });

  const agents = agentsResponse?.data ?? [];
  const tasks = tasksResponse?.data ?? [];

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    await createMutation.mutateAsync(form);
    setShowForm(false);
    setForm({
      name: "",
      task_type: "agent",
      input_prompt: "",
      schedule_type: "cron",
      cron_expression: "0 * * * *",
      priority: "medium",
      retry_on_failure: false,
      max_retries: 3,
      tags: [],
    });
  }

  function formatSchedule(task: (typeof tasks)[0]): string {
    if (task.schedule_type === "cron" && task.cron_expression) {
      const preset = CRON_PRESETS.find((p) => p.value === task.cron_expression);
      return preset ? preset.label : `Cron: ${task.cron_expression}`;
    }
    if (task.schedule_type === "interval" && task.interval_seconds) {
      const h = Math.floor(task.interval_seconds / 3600);
      const m = Math.floor((task.interval_seconds % 3600) / 60);
      return h > 0 ? `Every ${h}h ${m}m` : `Every ${m}m`;
    }
    if (task.schedule_type === "once" && task.run_at) {
      return `Once: ${new Date(task.run_at).toLocaleString()}`;
    }
    return task.schedule_type;
  }

  return (
    <div className="mx-auto max-w-5xl space-y-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Task Scheduler</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Schedule agent and workflow executions with cron, interval, or one-time triggers.
          </p>
        </div>
        <button
          onClick={() => setShowForm(!showForm)}
          className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
        >
          {showForm ? "Cancel" : "Schedule Task"}
        </button>
      </div>

      {/* Create form */}
      {showForm && (
        <section className="rounded-xl border border-border bg-card p-6">
          <h2 className="text-base font-semibold">New Scheduled Task</h2>
          <form onSubmit={handleSubmit} className="mt-4 space-y-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <label className="text-sm font-medium">Name</label>
                <input
                  type="text"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="Daily report generation"
                  required
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium">Task Type</label>
                <select
                  value={form.task_type}
                  onChange={(e) =>
                    setForm({ ...form, task_type: e.target.value as "agent" | "workflow" })
                  }
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="agent">Agent Execution</option>
                  <option value="workflow">Workflow Execution</option>
                </select>
              </div>

              {form.task_type === "agent" && (
                <div className="space-y-2">
                  <label className="text-sm font-medium">Agent</label>
                  <select
                    value={form.agent_id ?? ""}
                    onChange={(e) => setForm({ ...form, agent_id: e.target.value || undefined })}
                    className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <option value="">Select an agent...</option>
                    {agents.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.name} ({a.role})
                      </option>
                    ))}
                  </select>
                </div>
              )}

              <div className="space-y-2">
                <label className="text-sm font-medium">Schedule Type</label>
                <select
                  value={form.schedule_type}
                  onChange={(e) =>
                    setForm({
                      ...form,
                      schedule_type: e.target.value as "cron" | "interval" | "once",
                    })
                  }
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="cron">Cron Expression</option>
                  <option value="interval">Fixed Interval</option>
                  <option value="once">One-Time</option>
                </select>
              </div>

              {form.schedule_type === "cron" && (
                <>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Cron Preset</label>
                    <select
                      value={cronPreset}
                      onChange={(e) => {
                        setCronPreset(e.target.value);
                        if (e.target.value) {
                          setForm({ ...form, cron_expression: e.target.value });
                        }
                      }}
                      className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      {CRON_PRESETS.map((p) => (
                        <option key={p.label} value={p.value}>
                          {p.label}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Cron Expression</label>
                    <input
                      type="text"
                      value={form.cron_expression ?? ""}
                      onChange={(e) => setForm({ ...form, cron_expression: e.target.value })}
                      placeholder="*/5 * * * *"
                      className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm font-mono focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    />
                  </div>
                </>
              )}

              {form.schedule_type === "interval" && (
                <div className="space-y-2">
                  <label className="text-sm font-medium">Interval (seconds)</label>
                  <input
                    type="number"
                    value={form.interval_seconds ?? 3600}
                    onChange={(e) =>
                      setForm({ ...form, interval_seconds: parseInt(e.target.value) })
                    }
                    min={60}
                    className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                </div>
              )}

              {form.schedule_type === "once" && (
                <div className="space-y-2">
                  <label className="text-sm font-medium">Run At</label>
                  <input
                    type="datetime-local"
                    value={form.run_at ?? ""}
                    onChange={(e) => setForm({ ...form, run_at: e.target.value })}
                    className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                </div>
              )}

              <div className="space-y-2">
                <label className="text-sm font-medium">LLM Provider Override</label>
                <select
                  value={form.provider_config_id ?? ""}
                  onChange={(e) =>
                    setForm({ ...form, provider_config_id: e.target.value || undefined })
                  }
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="">Use agent default</option>
                  {providers?.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name} ({p.provider_type} / {p.default_model})
                    </option>
                  ))}
                </select>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium">Priority</label>
                <select
                  value={form.priority}
                  onChange={(e) =>
                    setForm({
                      ...form,
                      priority: e.target.value as "low" | "medium" | "high" | "critical",
                    })
                  }
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="low">Low</option>
                  <option value="medium">Medium</option>
                  <option value="high">High</option>
                  <option value="critical">Critical</option>
                </select>
              </div>
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium">Prompt / Input</label>
              <textarea
                value={form.input_prompt}
                onChange={(e) => setForm({ ...form, input_prompt: e.target.value })}
                placeholder="Describe the task for the agent..."
                rows={3}
                className="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </div>

            <div className="flex items-center gap-4">
              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="retry"
                  checked={form.retry_on_failure}
                  onChange={(e) => setForm({ ...form, retry_on_failure: e.target.checked })}
                  className="h-4 w-4 rounded border-input"
                />
                <label htmlFor="retry" className="text-sm">
                  Retry on failure
                </label>
              </div>
              {form.retry_on_failure && (
                <div className="flex items-center gap-2">
                  <label className="text-sm">Max retries:</label>
                  <input
                    type="number"
                    value={form.max_retries}
                    onChange={(e) => setForm({ ...form, max_retries: parseInt(e.target.value) })}
                    min={1}
                    max={10}
                    className="flex h-8 w-16 rounded-md border border-input bg-background px-2 text-sm"
                  />
                </div>
              )}
            </div>

            <button
              type="submit"
              disabled={createMutation.isPending}
              className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {createMutation.isPending ? "Creating..." : "Create Scheduled Task"}
            </button>
          </form>
        </section>
      )}

      {/* Task list */}
      {isLoading ? (
        <div className="flex items-center justify-center py-12">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        </div>
      ) : tasks.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border p-12 text-center">
          <p className="text-sm text-muted-foreground">No scheduled tasks yet.</p>
        </div>
      ) : (
        <div className="space-y-3">
          {tasks.map((task) => (
            <div
              key={task.id}
              className="rounded-xl border border-border bg-card p-5"
            >
              <div className="flex items-start justify-between">
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <h3 className="text-sm font-semibold">{task.name}</h3>
                    <span
                      className={`rounded-full px-2 py-0.5 text-xs font-medium ${
                        STATUS_COLORS[task.status] ?? ""
                      }`}
                    >
                      {task.status}
                    </span>
                    <span
                      className={`rounded-full px-2 py-0.5 text-xs font-medium ${
                        PRIORITY_COLORS[task.priority] ?? ""
                      }`}
                    >
                      {task.priority}
                    </span>
                    <span className="rounded-full bg-accent px-2 py-0.5 text-xs">
                      {task.task_type}
                    </span>
                  </div>
                  {task.description && (
                    <p className="text-xs text-muted-foreground">{task.description}</p>
                  )}
                  <div className="flex items-center gap-4 text-xs text-muted-foreground">
                    <span>Schedule: {formatSchedule(task)}</span>
                    <span>Runs: {task.run_count}{task.max_runs ? ` / ${task.max_runs}` : ""}</span>
                    {task.last_run_at && (
                      <span>Last: {new Date(task.last_run_at).toLocaleString()}</span>
                    )}
                    {task.next_run_at && (
                      <span>Next: {new Date(task.next_run_at).toLocaleString()}</span>
                    )}
                  </div>
                  {task.input_prompt && (
                    <p className="mt-1 max-w-xl truncate text-xs text-muted-foreground">
                      Prompt: {task.input_prompt}
                    </p>
                  )}
                </div>

                <div className="flex items-center gap-2">
                  {task.status === "active" ? (
                    <button
                      onClick={() =>
                        updateMutation.mutate({
                          id: task.id,
                          payload: { status: "paused" },
                        })
                      }
                      className="inline-flex h-8 items-center rounded-md border border-input bg-background px-3 text-xs font-medium hover:bg-accent"
                    >
                      Pause
                    </button>
                  ) : task.status === "paused" ? (
                    <button
                      onClick={() =>
                        updateMutation.mutate({
                          id: task.id,
                          payload: { status: "active" },
                        })
                      }
                      className="inline-flex h-8 items-center rounded-md bg-green-600 px-3 text-xs font-medium text-white hover:bg-green-700"
                    >
                      Resume
                    </button>
                  ) : null}
                  <button
                    onClick={() => {
                      if (confirm("Delete this scheduled task?")) {
                        deleteMutation.mutate(task.id);
                      }
                    }}
                    className="inline-flex h-8 items-center rounded-md border border-red-200 bg-background px-3 text-xs font-medium text-red-600 hover:bg-red-50 dark:border-red-800 dark:hover:bg-red-900/20"
                  >
                    Delete
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
