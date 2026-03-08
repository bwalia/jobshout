"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Cpu, StickyNote, Activity, BookOpen } from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { AgentStatusBadge } from "@/components/agent/AgentStatusBadge";
import { useAgent } from "@/lib/hooks/useAgents";
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
import type { Task } from "@/lib/types/project";

// ---------------------------------------------------------------------------
// Tab types
// ---------------------------------------------------------------------------
type Tab = "overview" | "tasks" | "metrics" | "knowledge";

const TABS: { id: Tab; label: string }[] = [
  { id: "overview", label: "Overview" },
  { id: "tasks", label: "Tasks" },
  { id: "metrics", label: "Metrics" },
  { id: "knowledge", label: "Knowledge" },
];

// ---------------------------------------------------------------------------
// Avatar helpers (mirrored from AgentCard to keep consistency)
// ---------------------------------------------------------------------------
const AVATAR_COLOURS = [
  "bg-violet-600",
  "bg-blue-600",
  "bg-emerald-600",
  "bg-amber-600",
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

/** Overview tab: agent details, description, system prompt. */
function OverviewTab({ agent }: { agent: NonNullable<ReturnType<typeof useAgent>["data"]> }) {
  return (
    <div className="space-y-6">
      {/* Core details */}
      <section>
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-widest text-muted-foreground">
          Details
        </h3>
        <dl className="space-y-3 rounded-lg border border-border bg-card p-5">
          <DetailRow label="Role" value={agent.role} />
          <DetailRow label="Model Provider" value={agent.model_provider} />
          <DetailRow label="Model Name" value={agent.model_name} />
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
  medium: "bg-yellow-500/20 text-yellow-400",
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
// Page component
// ---------------------------------------------------------------------------
export default function AgentProfilePage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const [activeTab, setActiveTab] = useState<Tab>("overview");

  const { data: agent, isLoading, isError } = useAgent(id);

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="h-8 w-48 animate-pulse rounded bg-muted" />
        <div className="h-32 animate-pulse rounded-lg bg-muted" />
        <div className="h-64 animate-pulse rounded-lg bg-muted" />
      </div>
    );
  }

  if (isError || !agent) {
    return (
      <div className="space-y-4">
        <Link
          href="/agents"
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Agents
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
    <div className="space-y-6">
      {/* Back navigation */}
      <button
        type="button"
        onClick={() => router.back()}
        className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        <ArrowLeft className="h-4 w-4" />
        Back
      </button>

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

                {agent.model_provider && (
                  <span className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                    <Cpu className="h-3 w-3" />
                    {agent.model_provider}
                    {agent.model_name ? ` / ${agent.model_name}` : ""}
                  </span>
                )}
              </div>
            </div>
          </div>

          {/* Performance score badge */}
          <div className="flex flex-col items-start gap-1 sm:items-end">
            <span className="text-xs text-muted-foreground">Performance</span>
            <span className="text-2xl font-bold text-foreground">
              {agent.performance_score}%
            </span>
          </div>
        </div>
      </div>

      {/* Tab bar */}
      <div className="border-b border-border">
        <nav className="-mb-px flex gap-0" aria-label="Agent profile tabs">
          {TABS.map(({ id: tabId, label }) => (
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
              {tabId === "tasks" && <Activity className="h-4 w-4" />}
              {tabId === "metrics" && <Activity className="h-4 w-4" />}
              {tabId === "knowledge" && <BookOpen className="h-4 w-4" />}
              {label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab content */}
      <div className="relative">
        {activeTab === "overview" && <OverviewTab agent={agent} />}
        {activeTab === "tasks" && <TasksTab agentId={agent.id} />}
        {activeTab === "metrics" && (
          <MetricsTab performanceScore={agent.performance_score} />
        )}
        {activeTab === "knowledge" && <KnowledgeTab agentId={agent.id} />}
      </div>
    </div>
  );
}
