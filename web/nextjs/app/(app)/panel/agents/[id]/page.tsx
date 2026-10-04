"use client";

import { useMemo, useState } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Cpu,
  StickyNote,
  Activity,
  BookOpen,
  Sparkles,
  Wrench,
  MessageSquareText,
  Boxes,
  LayoutGrid,
  AlertTriangle,
} from "lucide-react";
import { useRunFailures } from "@/lib/hooks/useRunFailures";
import { BuiltinAgentTab } from "@/components/task-manager/BuiltinAgentTab";
import { AGENT_CLIENTS } from "@/lib/agents/tab-clients";
import { useAgentSchemas } from "@/lib/hooks/useAgentSchemas";
import { useProjects } from "@/lib/hooks/useProjects";
import { taskKeys } from "@/lib/hooks/useTasks";
import type { LaunchResult } from "@/lib/agents/launch";
import { providerName } from "@/components/agent/ModelPicker";
import { useBreadcrumbs } from "@/lib/store/breadcrumb-store";
import { agentHref, type AgentTab } from "@/lib/agents/links";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { AgentStatusBadge } from "@/components/agent/AgentStatusBadge";
import { ExportAgentButton } from "@/components/agent/ExportAgentButton";
import { RemoveAgentButton } from "@/components/agent/RemoveAgentButton";
import { useAgent, useUpdateAgent } from "@/lib/hooks/useAgents";
import { ModelPicker } from "@/components/agent/ModelPicker";
import { useBlogConfig } from "@/lib/hooks/useBlog";
import type { ModelRecommendation, ModelRole } from "@/lib/types/blog";
import { useTasks } from "@/lib/hooks/useTasks";
import { KnowledgeFileList } from "@/components/agent/KnowledgeFileList";
import { KnowledgeEditor } from "@/components/agent/KnowledgeEditor";
import {
  getKnowledgeFiles,
  createKnowledgeFile,
  updateKnowledgeFile,
  deleteKnowledgeFile,
} from "@/lib/api/knowledge";
import type { KnowledgeFile } from "@/lib/api/knowledge";
import {
  useSkills,
  useAgentSkills,
  useEnableAgentSkill,
  useDisableAgentSkill,
} from "@/lib/hooks/useSkills";
import type { Skill, SkillKind } from "@/lib/api/skills";
import type { Task } from "@/lib/types/project";

// ---------------------------------------------------------------------------
// Tab types
// ---------------------------------------------------------------------------
type Tab = AgentTab;

const TABS: { id: Tab; label: string }[] = [
  { id: "overview", label: "Overview" },
  { id: "models", label: "Models" },
  { id: "workspace", label: "Workspace" },
  { id: "tasks", label: "Tasks" },
  { id: "metrics", label: "Metrics" },
  { id: "knowledge", label: "Knowledge" },
  { id: "skills", label: "Skills" },
];


// ---------------------------------------------------------------------------
// Avatar helpers (mirrored from AgentCard to keep consistency)
// ---------------------------------------------------------------------------
const AVATAR_COLOURS = [
  "bg-violet-600",
  "bg-blue-600",
  "bg-emerald-600",
  "bg-orange-600",
  "bg-rose-600",
  "bg-cyan-600",
  "bg-indigo-600",
  "bg-pink-600",
];

function getAvatarColour(name: string): string {
  return AVATAR_COLOURS[name.charCodeAt(0) % AVATAR_COLOURS.length];
}

function getInitials(name: string): string {
  return name
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0].toUpperCase())
    .join("");
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/** Displays a labelled detail row. */
function DetailRow({
  label,
  value,
}: {
  label: string;
  value: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-0.5 sm:flex-row sm:gap-6">
      <dt className="w-36 flex-shrink-0 text-sm font-medium text-muted-foreground">
        {label}
      </dt>
      <dd className="text-sm text-foreground">{value ?? "—"}</dd>
    </div>
  );
}

const PICKER_CLASS =
  "flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50";

/** The engine_config key the server reads the structured model from. */
const STRUCTURED_MODEL_KEY = "structured_model";

/**
 * Editable model row. Saves on change — useUpdateAgent already invalidates the
 * detail and list queries and raises a toast either way, so a separate Save
 * button would add a step without adding feedback.
 *
 * The Article Writer gets a second picker, because it makes two kinds of call
 * and a benchmark found different models are better at each: one writes the
 * article, the other returns the title, outline and review as JSON. Every other
 * agent makes one kind of call and keeps the single row.
 */
function ModelRow({
  agent,
}: {
  agent: NonNullable<ReturnType<typeof useAgent>["data"]>;
}) {
  const { mutate: updateAgent, isPending } = useUpdateAgent();
  const { data: blogConfig } = useBlogConfig();

  const isArticleWriter = agent.metadata?.builtin === "article_writer";
  const advice = (role: ModelRole) =>
    blogConfig?.recommended_models?.find((r) => r.role === role);

  if (!isArticleWriter) {
    return (
      <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
        <dt className="text-sm text-muted-foreground sm:w-40 sm:shrink-0">
          Model
        </dt>
        <dd className="w-full sm:max-w-sm">
          <ModelPicker
            value={{
              provider: agent.model_provider ?? "",
              model: agent.model_name ?? "",
            }}
            disabled={isPending}
            onChange={(v) =>
              updateAgent({
                id: agent.id,
                payload: { model_provider: v.provider, model_name: v.model },
              })
            }
            className={PICKER_CLASS}
          />
        </dd>
      </div>
    );
  }

  const prose = advice("prose");
  const structured = advice("structured");
  const structuredModel =
    typeof agent.engine_config?.[STRUCTURED_MODEL_KEY] === "string"
      ? (agent.engine_config[STRUCTURED_MODEL_KEY] as string)
      : "";

  return (
    <div className="flex flex-col gap-5">
      <ModelSetting
        label={prose?.label ?? "Writing model"}
        covers={prose?.covers}
        advice={prose}
        picker={
          <ModelPicker
            value={{
              provider: agent.model_provider ?? "",
              model: agent.model_name ?? "",
            }}
            disabled={isPending}
            providerFilter={blogConfig?.provider}
            recommended={prose?.model}
            inheritedModel={blogConfig?.effective_models?.prose}
            includeAuto={false}
            onChange={(v) =>
              updateAgent({
                id: agent.id,
                payload: { model_provider: v.provider, model_name: v.model },
              })
            }
            className={PICKER_CLASS}
          />
        }
      />

      <ModelSetting
        label={structured?.label ?? "Structured model"}
        covers={structured?.covers}
        advice={structured}
        picker={
          <ModelPicker
            // Only the model name is stored: the pipeline talks to one
            // provider, so a second provider field would be a setting that
            // never applies.
            value={{
              provider: structuredModel ? (blogConfig?.provider ?? "") : "",
              model: structuredModel,
            }}
            disabled={isPending}
            providerFilter={blogConfig?.provider}
            recommended={structured?.model}
            inheritedModel={blogConfig?.effective_models?.structured}
            includeAuto={false}
            onChange={(v) =>
              updateAgent({
                id: agent.id,
                payload: {
                  engine_config: {
                    ...(agent.engine_config ?? {}),
                    [STRUCTURED_MODEL_KEY]: v.model,
                  },
                },
              })
            }
            className={PICKER_CLASS}
          />
        }
      />
    </div>
  );
}

/** One labelled model setting, with what it governs and the measured advice. */
function ModelSetting({
  label,
  covers,
  advice,
  picker,
}: {
  label: string;
  covers?: string;
  advice?: ModelRecommendation;
  picker: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium text-foreground">{label}</span>
        {covers && (
          <span className="text-xs text-muted-foreground">{covers}</span>
        )}
      </div>
      <div className="sm:max-w-sm">{picker}</div>
      {advice?.model && (
        <p className="text-xs text-muted-foreground sm:max-w-sm">
          <span className="font-medium text-foreground">
            Recommended: {advice.model}
          </span>{" "}
          — {advice.reason}
          {advice.caveat && (
            <span className="text-muted-foreground"> {advice.caveat}</span>
          )}
        </p>
      )}
    </div>
  );
}

/** What this agent's models resolve to, in words a person can check. */
function modelSummary(
  provider: string | null | undefined,
  model: string | null | undefined,
  inherited?: string
): string {
  if (provider === "auto") return "Automatic (chosen per request)";
  if (!provider && !model) {
    return inherited ? `Platform default → ${inherited}` : "Platform default";
  }
  return [providerName(provider ?? ""), model || "provider default"].filter(Boolean).join(" · ");
}

/** The top-of-Overview card: which models run this agent, and a way to change them. */
function ModelsSummary({ agent }: { agent: NonNullable<ReturnType<typeof useAgent>["data"]> }) {
  const { data: blogConfig } = useBlogConfig();
  const isArticleWriter = agent.metadata?.builtin === "article_writer";
  const structured =
    typeof agent.engine_config?.[STRUCTURED_MODEL_KEY] === "string"
      ? (agent.engine_config[STRUCTURED_MODEL_KEY] as string)
      : "";
  return (
    <section className="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0 space-y-1">
        <h3 className="text-sm font-semibold text-foreground">Models</h3>
        <p className="text-sm text-muted-foreground">
          <span className="text-foreground">{isArticleWriter ? "Writing: " : ""}</span>
          {modelSummary(agent.model_provider, agent.model_name, isArticleWriter ? blogConfig?.effective_models?.prose : undefined)}
        </p>
        {isArticleWriter && (
          <p className="text-sm text-muted-foreground">
            <span className="text-foreground">Structured: </span>
            {structured
              ? `${providerName(blogConfig?.provider ?? "")} · ${structured}`
              : modelSummary(null, null, blogConfig?.effective_models?.structured)}
          </p>
        )}
      </div>
      <Link
        href={agentHref(agent.id, "models")}
        scroll={false}
        className="inline-flex shrink-0 items-center gap-1.5 self-start rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:opacity-90 sm:self-center"
      >
        <Cpu className="h-4 w-4" />
        Change model
      </Link>
    </section>
  );
}

/** Models tab: the model selectors, with what each one governs. */
function ModelsTab({ agent }: { agent: NonNullable<ReturnType<typeof useAgent>["data"]> }) {
  return (
    <section className="space-y-3">
      <div>
        <h3 className="text-sm font-semibold text-foreground">Which models this agent uses</h3>
        <p className="text-sm text-muted-foreground">
          Changes save straight away and apply to the agent&apos;s next run. A single task can still
          override the model from its task panel.
        </p>
      </div>
      <dl className="space-y-3 rounded-lg border border-border bg-card p-5">
        <ModelRow agent={agent} />
      </dl>
    </section>
  );
}

/**
 * Workspace tab: the specialist's own UI (its registered tab client), or its
 * launch form and Run. What Task Manager used to show for this agent.
 */
function WorkspaceTab({ agent }: { agent: NonNullable<ReturnType<typeof useAgent>["data"]> }) {
  const router = useRouter();
  const qc = useQueryClient();
  const { data: catalog } = useAgentSchemas();
  const { data: projectsResp } = useProjects({ per_page: 100 });
  const builtin = agent.metadata?.builtin;
  const wire = catalog?.find((w) => w.builtin === builtin);

  function onLaunched(result: LaunchResult) {
    void qc.invalidateQueries({ queryKey: taskKeys.all });
    if (!result.task || wire?.stay_on_tab) return;
    const params = new URLSearchParams({ project: result.task.project_id, task: result.task.id });
    if (result.run_id) params.set("run", result.run_id);
    router.push(`/panel/projects?${params.toString()}`);
  }

  if (!wire) {
    return <p className="text-sm text-muted-foreground">Loading this agent&apos;s workspace…</p>;
  }
  return (
    <BuiltinAgentTab
      wire={wire}
      agent={agent}
      projects={projectsResp?.data ?? []}
      Client={builtin ? AGENT_CLIENTS[builtin] : undefined}
      onLaunched={onLaunched}
    />
  );
}

/** Overview tab: agent details, description, system prompt. */
function OverviewTab({ agent }: { agent: NonNullable<ReturnType<typeof useAgent>["data"]> }) {
  return (
    <div className="space-y-6">
      <ModelsSummary agent={agent} />

      {/* Core details */}
      <section>
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-widest text-muted-foreground">
          Details
        </h3>
        <dl className="space-y-3 rounded-lg border border-border bg-card p-5">
          <DetailRow label="Role" value={agent.role} />
          <DetailRow
            label="Created"
            value={new Date(agent.created_at).toLocaleDateString()}
          />
          <DetailRow
            label="Last Updated"
            value={new Date(agent.updated_at).toLocaleDateString()}
          />
        </dl>
      </section>

      {/* Description */}
      {agent.description && (
        <section>
          <h3 className="mb-3 text-sm font-semibold uppercase tracking-widest text-muted-foreground">
            Description
          </h3>
          <div className="rounded-lg border border-border bg-card p-5">
            <p className="whitespace-pre-wrap text-sm text-foreground leading-relaxed">
              {agent.description}
            </p>
          </div>
        </section>
      )}

      {/* System Prompt */}
      {agent.system_prompt && (
        <section>
          <h3 className="mb-3 text-sm font-semibold uppercase tracking-widest text-muted-foreground">
            System Prompt
          </h3>
          <div className="rounded-lg border border-border bg-card p-5">
            <pre className="whitespace-pre-wrap font-mono text-xs text-foreground leading-relaxed overflow-x-auto">
              {agent.system_prompt}
            </pre>
          </div>
        </section>
      )}
    </div>
  );
}

const PRIORITY_CLASSES: Record<string, string> = {
  critical: "bg-red-500/20 text-red-400",
  high: "bg-orange-500/20 text-orange-400",
  medium: "bg-orange-500/20 text-orange-400",
  low: "bg-blue-500/20 text-blue-400",
};

const STATUS_CLASSES: Record<string, string> = {
  done: "bg-emerald-500/20 text-emerald-400",
  in_progress: "bg-blue-500/20 text-blue-400",
  review: "bg-violet-500/20 text-violet-400",
  todo: "bg-muted text-muted-foreground",
  backlog: "bg-muted text-muted-foreground",
};

function formatStatus(status: string) {
  return status.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

/** Placeholder tasks tab – fetches tasks assigned to this agent. */
function TasksTab({ agentId }: { agentId: string }) {
  // Fetch all org tasks and filter by agent id on the client side since the
  // API does not expose a dedicated /agents/:id/tasks endpoint.
  const { data, isLoading, isError } = useTasks();

  // Filter tasks by this agent (graceful fallback – empty array if data is
  // unavailable or the endpoint is unimplemented).
  const agentTasks: Task[] = (data?.data ?? []).filter(
    (t) => t.assigned_agent_id === agentId
  );

  if (isLoading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="h-14 animate-pulse rounded-md bg-muted" />
        ))}
      </div>
    );
  }

  if (isError) {
    return (
      <p className="text-sm text-muted-foreground">Unable to load tasks.</p>
    );
  }

  if (agentTasks.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border px-6 py-10 text-center">
        <p className="text-sm text-muted-foreground">
          This agent has no assigned tasks yet.
        </p>
      </div>
    );
  }

  return (
    <ul className="divide-y divide-border rounded-lg border border-border bg-card">
      {agentTasks.map((task) => (
        <li
          key={task.id}
          className="flex items-center justify-between gap-4 px-4 py-3"
        >
          <p className="flex-1 truncate text-sm text-foreground">{task.title}</p>
          <span
            className={`rounded-full px-2 py-0.5 text-xs font-medium ${
              STATUS_CLASSES[task.status] ?? "bg-muted text-muted-foreground"
            }`}
          >
            {formatStatus(task.status)}
          </span>
          <span
            className={`rounded-full px-2 py-0.5 text-xs font-medium ${
              PRIORITY_CLASSES[task.priority] ?? ""
            }`}
          >
            {task.priority}
          </span>
        </li>
      ))}
    </ul>
  );
}

// ---------------------------------------------------------------------------
// New file name prompt dialog helper
// ---------------------------------------------------------------------------

interface NewFileDialogState {
  open: boolean;
  filename: string;
}

/** Knowledge tab: list of knowledge files and an editor for the selected one. */
function KnowledgeTab({ agentId }: { agentId: string }) {
  const queryClient = useQueryClient();
  const [selectedFileId, setSelectedFileId] = useState<string>("");
  const [editorContent, setEditorContent] = useState<string>("");
  const [newFileDialog, setNewFileDialog] = useState<NewFileDialogState>({
    open: false,
    filename: "",
  });

  const {
    data: files = [],
    isLoading,
    isError,
  } = useQuery<KnowledgeFile[]>({
    queryKey: ["knowledge", agentId],
    queryFn: () => getKnowledgeFiles(agentId),
    enabled: Boolean(agentId),
  });

  // When a file is selected from the list, load its content into the editor
  function handleSelectFile(fileId: string): void {
    const file = files.find((f) => f.id === fileId);
    if (file) {
      setSelectedFileId(fileId);
      setEditorContent(file.content);
    }
  }

  const saveMutation = useMutation({
    mutationFn: () =>
      updateKnowledgeFile(agentId, selectedFileId, editorContent),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["knowledge", agentId] });
      toast.success("Knowledge file saved.");
    },
    onError: (error: Error) => {
      toast.error(`Failed to save file: ${error.message}`);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (fileId: string) => deleteKnowledgeFile(agentId, fileId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["knowledge", agentId] });
      toast.success("Knowledge file deleted.");
      // Clear the editor if the deleted file was selected
      setSelectedFileId("");
      setEditorContent("");
    },
    onError: (error: Error) => {
      toast.error(`Failed to delete file: ${error.message}`);
    },
  });

  const createMutation = useMutation({
    mutationFn: (filename: string) =>
      createKnowledgeFile(agentId, { filename, content: "" }),
    onSuccess: (created) => {
      queryClient.invalidateQueries({ queryKey: ["knowledge", agentId] });
      toast.success("Knowledge file created.");
      // Auto-select the newly created file
      setSelectedFileId(created.id);
      setEditorContent("");
      setNewFileDialog({ open: false, filename: "" });
    },
    onError: (error: Error) => {
      toast.error(`Failed to create file: ${error.message}`);
    },
  });

  function handleNewFile(): void {
    setNewFileDialog({ open: true, filename: "" });
  }

  function handleCreateConfirm(): void {
    const trimmed = newFileDialog.filename.trim();
    if (!trimmed) return;
    createMutation.mutate(trimmed);
  }

  function handleDeleteSelected(): void {
    if (!selectedFileId) return;
    deleteMutation.mutate(selectedFileId);
  }

  // Map KnowledgeFile[] to the shape expected by KnowledgeFileList
  const fileListItems = files.map((f) => ({
    id: f.id,
    name: f.filename,
    updated_at: f.updated_at,
  }));

  if (isLoading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-10 animate-pulse rounded-md bg-muted" />
        ))}
      </div>
    );
  }

  if (isError) {
    return (
      <p className="text-sm text-destructive">
        Failed to load knowledge files.
      </p>
    );
  }

  return (
    <div className="flex gap-0 overflow-hidden rounded-lg border border-border bg-card" style={{ minHeight: "520px" }}>
      {/* Sidebar: file list + actions */}
      <div className="flex w-56 flex-shrink-0 flex-col border-r border-border">
        {/* New File button */}
        <div className="border-b border-border px-3 py-2.5">
          <button
            type="button"
            onClick={handleNewFile}
            className="inline-flex w-full items-center justify-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            + New File
          </button>
        </div>

        {/* File list */}
        <div className="flex-1 overflow-y-auto">
          <KnowledgeFileList
            files={fileListItems}
            selectedFileId={selectedFileId}
            onSelectFile={handleSelectFile}
          />
        </div>

        {/* Delete selected file */}
        {selectedFileId && (
          <div className="border-t border-border px-3 py-2">
            <button
              type="button"
              onClick={handleDeleteSelected}
              disabled={deleteMutation.isPending}
              className="inline-flex w-full items-center justify-center gap-1.5 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-1.5 text-xs font-medium text-destructive hover:bg-destructive/20 disabled:pointer-events-none disabled:opacity-50"
            >
              {deleteMutation.isPending ? "Deleting…" : "Delete File"}
            </button>
          </div>
        )}
      </div>

      {/* Editor pane */}
      <div className="flex flex-1 flex-col">
        {selectedFileId ? (
          <KnowledgeEditor
            value={editorContent}
            onChange={setEditorContent}
            onSave={() => saveMutation.mutate()}
          />
        ) : (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
            Select a file to edit or create a new one.
          </div>
        )}
      </div>

      {/* New file name dialog (simple inline overlay) */}
      {newFileDialog.open && (
        <div
          className="absolute inset-0 z-10 flex items-center justify-center bg-black/40"
          onClick={() => setNewFileDialog({ open: false, filename: "" })}
        >
          <div
            className="w-80 rounded-lg border border-border bg-card p-5 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 className="mb-3 text-sm font-semibold">New Knowledge File</h3>
            <input
              type="text"
              autoFocus
              value={newFileDialog.filename}
              onChange={(e) =>
                setNewFileDialog((prev) => ({
                  ...prev,
                  filename: e.target.value,
                }))
              }
              onKeyDown={(e) => {
                if (e.key === "Enter") handleCreateConfirm();
                if (e.key === "Escape")
                  setNewFileDialog({ open: false, filename: "" });
              }}
              placeholder="e.g. overview.md"
              className="mb-4 flex h-9 w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setNewFileDialog({ open: false, filename: "" })}
                className="inline-flex h-8 items-center rounded-md border border-border bg-background px-3 text-xs hover:bg-accent"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleCreateConfirm}
                disabled={
                  !newFileDialog.filename.trim() || createMutation.isPending
                }
                className="inline-flex h-8 items-center rounded-md bg-primary px-3 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
              >
                {createMutation.isPending ? "Creating…" : "Create"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/** Placeholder metrics tab. */
function MetricsTab({
  performanceScore,
}: {
  performanceScore: number;
}) {
  // A simple visual representation of the performance score.
  return (
    <div className="space-y-6">
      <div className="rounded-lg border border-border bg-card p-5">
        <p className="mb-4 text-sm font-medium text-muted-foreground">
          Performance Score
        </p>
        <div className="flex items-end gap-4">
          <span className="text-4xl font-bold text-foreground">
            {performanceScore}
            <span className="text-xl text-muted-foreground">%</span>
          </span>
        </div>
        <div className="mt-4 h-2 w-full rounded-full bg-muted overflow-hidden">
          <div
            className="h-full rounded-full bg-emerald-500"
            style={{ width: `${performanceScore}%` }}
            role="progressbar"
            aria-valuenow={performanceScore}
            aria-valuemin={0}
            aria-valuemax={100}
          />
        </div>
      </div>

      <p className="text-sm text-muted-foreground">
        Detailed metrics and activity charts will appear here as the agent
        completes tasks.
      </p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Skills tab — enable/disable registry skills for this agent
// ---------------------------------------------------------------------------
const SKILL_KIND_META: Record<
  SkillKind,
  { icon: React.ElementType; color: string }
> = {
  tool: { icon: Wrench, color: "text-sky-400" },
  prompt: { icon: MessageSquareText, color: "text-violet-400" },
  bundle: { icon: Boxes, color: "text-orange-400" },
};

function SkillsTab({ agentId }: { agentId: string }) {
  const { data: catalog = [], isLoading: catalogLoading } = useSkills();
  const {
    data: enabled = [],
    isLoading: enabledLoading,
    isError,
  } = useAgentSkills(agentId);
  const enableSkill = useEnableAgentSkill(agentId);
  const disableSkill = useDisableAgentSkill(agentId);

  const enabledIds = new Set(enabled.map((s) => s.id));
  const isLoading = catalogLoading || enabledLoading;

  if (isLoading) {
    return (
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="h-24 animate-pulse rounded-lg bg-muted" />
        ))}
      </div>
    );
  }

  if (isError) {
    return (
      <p className="text-sm text-destructive">Failed to load agent skills.</p>
    );
  }

  if (catalog.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-16 text-center">
        <Sparkles className="h-10 w-10 text-muted-foreground mb-3" />
        <p className="text-base font-medium text-foreground">
          No skills available
        </p>
        <p className="mt-1 text-sm text-muted-foreground">
          Create skills in the{" "}
          <Link href="/skills" className="text-primary hover:underline">
            Skills registry
          </Link>{" "}
          to enable them here.
        </p>
      </div>
    );
  }

  const pending = enableSkill.isPending || disableSkill.isPending;

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Enabled skills are folded into this agent&apos;s runs — tool skills widen
        its available tools, prompt skills patch its system prompt.
      </p>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {catalog.map((skill: Skill) => {
          const meta = SKILL_KIND_META[skill.kind] ?? SKILL_KIND_META.tool;
          const Icon = meta.icon;
          const on = enabledIds.has(skill.id);
          return (
            <div
              key={skill.id}
              className="flex items-start justify-between gap-3 rounded-lg border border-border bg-card p-4"
            >
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <Icon className={`h-4 w-4 shrink-0 ${meta.color}`} />
                  <h4 className="truncate text-sm font-semibold text-foreground">
                    {skill.name}
                  </h4>
                </div>
                {skill.description && (
                  <p className="mt-1 text-xs text-muted-foreground line-clamp-2">
                    {skill.description}
                  </p>
                )}
                <span className="mt-2 inline-block rounded bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">
                  {skill.kind}
                </span>
              </div>
              <button
                type="button"
                disabled={pending}
                onClick={() =>
                  on
                    ? disableSkill.mutate(skill.id)
                    : enableSkill.mutate(skill.id)
                }
                className={`shrink-0 rounded-md px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50 ${
                  on
                    ? "border border-border text-muted-foreground hover:bg-accent"
                    : "bg-primary text-primary-foreground hover:bg-primary/90"
                }`}
              >
                {on ? "Enabled" : "Enable"}
              </button>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page component
// ---------------------------------------------------------------------------
export default function AgentProfilePage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const search = useSearchParams();
  const { data: agent, isLoading, isError } = useAgent(id);
  const isBuiltin = Boolean(agent?.metadata?.builtin);
  const tabs = useMemo(
    () => TABS.filter((t) => t.id !== "workspace" || isBuiltin),
    [isBuiltin]
  );
  const { failures, byAgent } = useRunFailures();
  const failed = byAgent.get(id)?.failed ?? 0;
  const latestFailure = failures.find((f) => f.agentId === id);
  const requested = search.get("tab") as Tab | null;
  const activeTab: Tab = tabs.some((t) => t.id === requested) ? (requested as Tab) : "overview";
  const setActiveTab = (tab: Tab) => router.replace(agentHref(id, tab), { scroll: false });

  useBreadcrumbs(
    agent
      ? [
          { label: agent.name, href: agentHref(agent.id) },
          { label: tabs.find((t) => t.id === activeTab)?.label ?? "Overview" },
        ]
      : []
  );

  if (isLoading) {
    return (
      <div className="space-y-6 p-6">
        <div className="h-8 w-48 animate-pulse rounded bg-muted" />
        <div className="h-32 animate-pulse rounded-lg bg-muted" />
        <div className="h-64 animate-pulse rounded-lg bg-muted" />
      </div>
    );
  }

  if (isError || !agent) {
    return (
      <div className="space-y-4 p-6">
        <Link
          href="/panel/agents"
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          All agents
        </Link>
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-5 py-4 text-sm text-destructive">
          Agent not found or failed to load.
        </div>
      </div>
    );
  }

  const avatarColour = getAvatarColour(agent.name);
  const initials = getInitials(agent.name);

  return (
    <div className="space-y-6 p-6">
      {/* Back navigation */}
      <Link
        href="/panel/agents"
        className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        <ArrowLeft className="h-4 w-4" />
        All agents
      </Link>

      {/* Agent header card */}
      <div className="rounded-lg border border-border bg-card p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          {/* Avatar + name */}
          <div className="flex items-center gap-4">
            {agent.avatar_url ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={agent.avatar_url}
                alt={`${agent.name} avatar`}
                className="h-16 w-16 rounded-full object-cover"
              />
            ) : (
              <span
                className={`flex h-16 w-16 flex-shrink-0 items-center justify-center rounded-full text-xl font-bold text-white ${avatarColour}`}
              >
                {initials}
              </span>
            )}

            <div>
              <h1 className="text-xl font-bold text-foreground">{agent.name}</h1>
              <p className="text-sm text-muted-foreground">{agent.role}</p>

              <div className="mt-2 flex flex-wrap items-center gap-2">
                <AgentStatusBadge status={agent.status} />

                {isBuiltin && (
                  <Link
                    href={agentHref(agent.id, "workspace")}
                    scroll={false}
                    className="rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary hover:underline"
                  >
                    Open workspace
                  </Link>
                )}

                <Link
                  href={agentHref(agent.id, "models")}
                  scroll={false}
                  title="Change model"
                  className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground hover:text-foreground hover:underline"
                >
                  <Cpu className="h-3 w-3" />
                  {modelSummary(agent.model_provider, agent.model_name)}
                </Link>
              </div>
            </div>
          </div>

          {/* Performance score badge */}
          <div className="flex flex-col items-start gap-2 sm:items-end">
            <div className="flex flex-wrap justify-end gap-2">
              <Link
                href={`/panel/tasks?new=1&agent=${agent.id}`}
                className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-3 text-sm font-medium text-primary-foreground hover:opacity-90"
              >
                New task
              </Link>
              <ExportAgentButton agentId={agent.id} />
              <RemoveAgentButton
                agent={agent}
                onRemoved={() => router.push("/panel/agents")}
              />
            </div>
            <span className="text-xs text-muted-foreground">Performance</span>
            {failed > 0 && Math.round(agent.performance_score) === 0 ? (
              <span className="text-lg font-bold text-destructive">
                {failed} failed run{failed === 1 ? "" : "s"}
              </span>
            ) : (
              <span className="text-2xl font-bold text-foreground">
                {agent.performance_score}%
              </span>
            )}
          </div>
        </div>
      </div>

      {failed > 0 && (
        <div role="alert" className="flex flex-col gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-4 sm:flex-row sm:items-center sm:justify-between">
          <p className="flex items-start gap-2 text-sm">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
            <span>
              <span className="font-semibold text-destructive">
                {failed} failed run{failed === 1 ? "" : "s"} in the last two weeks.
              </span>{" "}
              {latestFailure && <span className="text-muted-foreground">Latest: {latestFailure.error}</span>}
            </span>
          </p>
          <div className="flex shrink-0 gap-2">
            <Link
              href={`/panel/tasks?agent=${agent.id}&failed=1`}
              className="inline-flex h-8 items-center rounded-md border border-border bg-background px-3 text-xs font-medium hover:bg-muted"
            >
              See failed tasks
            </Link>
            <Link
              href={agentHref(agent.id, "models")}
              scroll={false}
              className="inline-flex h-8 items-center gap-1 rounded-md bg-primary px-3 text-xs font-semibold text-primary-foreground hover:opacity-90"
            >
              <Cpu className="h-3.5 w-3.5" /> Change model
            </Link>
          </div>
        </div>
      )}

      {/* Tab bar */}
      <div className="border-b border-border">
        <nav className="-mb-px flex gap-0" aria-label="Agent profile tabs">
          {tabs.map(({ id: tabId, label }) => (
            <button
              key={tabId}
              type="button"
              onClick={() => setActiveTab(tabId)}
              className={`inline-flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
                activeTab === tabId
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:border-border hover:text-foreground"
              }`}
              aria-current={activeTab === tabId ? "page" : undefined}
            >
              {tabId === "overview" && <StickyNote className="h-4 w-4" />}
              {tabId === "models" && <Cpu className="h-4 w-4" />}
              {tabId === "workspace" && <LayoutGrid className="h-4 w-4" />}
              {tabId === "tasks" && <Activity className="h-4 w-4" />}
              {tabId === "metrics" && <Activity className="h-4 w-4" />}
              {tabId === "knowledge" && <BookOpen className="h-4 w-4" />}
              {tabId === "skills" && <Sparkles className="h-4 w-4" />}
              {label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab content */}
      <div className="relative">
        {activeTab === "overview" && <OverviewTab agent={agent} />}
        {activeTab === "models" && <ModelsTab agent={agent} />}
        {activeTab === "workspace" && <WorkspaceTab agent={agent} />}
        {activeTab === "tasks" && <TasksTab agentId={agent.id} />}
        {activeTab === "metrics" && (
          <MetricsTab performanceScore={agent.performance_score} />
        )}
        {activeTab === "knowledge" && <KnowledgeTab agentId={agent.id} />}
        {activeTab === "skills" && <SkillsTab agentId={agent.id} />}
      </div>
    </div>
  );
}
