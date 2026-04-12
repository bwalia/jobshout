"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Search, Plus, Play, Trash2, Workflow as WorkflowIcon } from "lucide-react";
import {
  useWorkflows,
  useDeleteWorkflow,
  useExecuteWorkflow,
} from "@/lib/hooks/useWorkflows";
import type { Workflow } from "@/lib/types/workflow";

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    active: "bg-green-500/20 text-green-400",
    draft: "bg-yellow-500/20 text-yellow-400",
    archived: "bg-gray-500/20 text-gray-400",
  };
  return (
    <span
      className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors[status] ?? "bg-muted text-muted-foreground"}`}
    >
      {status}
    </span>
  );
}

function WorkflowCard({ workflow }: { workflow: Workflow }) {
  const router = useRouter();
  const deleteWorkflow = useDeleteWorkflow();
  const executeWorkflow = useExecuteWorkflow();

  return (
    <div className="group relative flex flex-col justify-between rounded-lg border border-border bg-card p-4 transition-colors hover:border-primary/40">
      <div>
        <div className="flex items-center justify-between">
          <h3
            className="text-sm font-semibold text-foreground cursor-pointer hover:text-primary transition-colors"
            onClick={() => router.push(`/workflows/${workflow.id}`)}
          >
            {workflow.name}
          </h3>
          <StatusBadge status={workflow.status} />
        </div>
        {workflow.description && (
          <p className="mt-1 text-xs text-muted-foreground line-clamp-2">
            {workflow.description}
          </p>
        )}
        <p className="mt-2 text-xs text-muted-foreground">
          {workflow.steps?.length ?? 0} step{(workflow.steps?.length ?? 0) !== 1 ? "s" : ""}
        </p>
      </div>

      <div className="mt-3 flex items-center gap-2 border-t border-border pt-3">
        <button
          onClick={() =>
            executeWorkflow.mutate({ id: workflow.id, payload: {} })
          }
          disabled={executeWorkflow.isPending}
          className="inline-flex items-center gap-1 rounded-md bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary hover:bg-primary/20 transition-colors disabled:opacity-50"
        >
          <Play className="h-3 w-3" />
          Run
        </button>
        <button
          onClick={() => router.push(`/workflows/${workflow.id}`)}
          className="inline-flex items-center gap-1 rounded-md bg-muted px-2.5 py-1 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
        >
          Edit
        </button>
        <button
          onClick={() => {
            if (confirm("Delete this workflow?")) {
              deleteWorkflow.mutate(workflow.id);
            }
          }}
          disabled={deleteWorkflow.isPending}
          className="ml-auto inline-flex items-center rounded-md p-1 text-muted-foreground hover:text-destructive transition-colors disabled:opacity-50"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
}

export default function WorkflowsPage() {
  const [searchQuery, setSearchQuery] = useState("");
  const { data, isLoading, isError } = useWorkflows({ per_page: 50 });
  const router = useRouter();

  const workflows = (data?.data ?? []).filter((w: Workflow) =>
    searchQuery
      ? w.name.toLowerCase().includes(searchQuery.toLowerCase())
      : true
  );

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-foreground">
            Workflows
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Build and manage multi-step AI workflows with a visual DAG editor.
          </p>
        </div>
        <button
          type="button"
          onClick={() => router.push("/workflows/new")}
          className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          <Plus className="h-4 w-4" />
          New Workflow
        </button>
      </div>

      <div className="relative max-w-md">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <input
          type="search"
          placeholder="Search workflows..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="flex h-9 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </div>

      {isLoading && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="h-36 animate-pulse rounded-lg bg-muted" />
          ))}
        </div>
      )}

      {isError && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-5 py-4 text-sm text-destructive">
          Failed to load workflows. Please try refreshing the page.
        </div>
      )}

      {!isLoading && !isError && workflows.length === 0 && (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-16 text-center">
          <WorkflowIcon className="h-10 w-10 text-muted-foreground mb-3" />
          <p className="text-base font-medium text-foreground">
            No workflows yet
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            {searchQuery
              ? "Try adjusting your search."
              : "Create your first workflow to get started."}
          </p>
          {!searchQuery && (
            <button
              type="button"
              onClick={() => router.push("/workflows/new")}
              className="mt-4 inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
            >
              <Plus className="h-4 w-4" />
              New Workflow
            </button>
          )}
        </div>
      )}

      {!isLoading && !isError && workflows.length > 0 && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {workflows.map((workflow: Workflow) => (
            <WorkflowCard key={workflow.id} workflow={workflow} />
          ))}
        </div>
      )}
    </div>
  );
}
