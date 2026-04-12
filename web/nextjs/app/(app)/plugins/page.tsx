"use client";

import { useState } from "react";
import {
  Search,
  Plus,
  Play,
  Trash2,
  Puzzle,
  MoreVertical,
} from "lucide-react";
import {
  usePlugins,
  useCreatePlugin,
  useDeletePlugin,
  useExecutePlugin,
} from "@/lib/hooks/usePlugins";
import type { Plugin, CreatePluginRequest } from "@/lib/types/workflow";

function PluginStatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    active: "bg-green-500/20 text-green-400",
    inactive: "bg-yellow-500/20 text-yellow-400",
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

function CreatePluginDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const createPlugin = useCreatePlugin();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [version, setVersion] = useState("1.0.0");
  const [workflowDef, setWorkflowDef] = useState("{}");

  if (!open) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    let parsedDef: Record<string, unknown> = {};
    try {
      parsedDef = JSON.parse(workflowDef);
    } catch {
      return;
    }

    const payload: CreatePluginRequest = {
      name,
      version,
      description: description || undefined,
      workflow_def: parsedDef,
    };

    createPlugin.mutate(payload, {
      onSuccess: () => {
        setName("");
        setDescription("");
        setVersion("1.0.0");
        setWorkflowDef("{}");
        onClose();
      },
    });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-lg rounded-lg border border-border bg-card p-6 shadow-xl space-y-4"
      >
        <h2 className="text-lg font-semibold text-foreground">
          Create Plugin
        </h2>

        <div>
          <label className="text-xs text-muted-foreground">Name</label>
          <input
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
            placeholder="My Plugin"
          />
        </div>

        <div className="flex gap-3">
          <div className="flex-1">
            <label className="text-xs text-muted-foreground">Version</label>
            <input
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
            />
          </div>
        </div>

        <div>
          <label className="text-xs text-muted-foreground">Description</label>
          <input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
            placeholder="What does this plugin do?"
          />
        </div>

        <div>
          <label className="text-xs text-muted-foreground">
            Workflow Definition (JSON)
          </label>
          <textarea
            value={workflowDef}
            onChange={(e) => setWorkflowDef(e.target.value)}
            rows={6}
            className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm font-mono"
            placeholder='{"nodes": [], "edges": []}'
          />
        </div>

        <div className="flex items-center justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-border px-4 py-2 text-sm text-muted-foreground hover:bg-accent transition-colors"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={createPlugin.isPending || !name}
            className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
          >
            {createPlugin.isPending ? "Creating..." : "Create"}
          </button>
        </div>
      </form>
    </div>
  );
}

function PluginCard({ plugin }: { plugin: Plugin }) {
  const deletePlugin = useDeletePlugin();
  const executePlugin = useExecutePlugin();

  return (
    <div className="group relative flex flex-col justify-between rounded-lg border border-border bg-card p-4 transition-colors hover:border-primary/40">
      <div>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Puzzle className="h-4 w-4 text-indigo-400" />
            <h3 className="text-sm font-semibold text-foreground">
              {plugin.name}
            </h3>
          </div>
          <PluginStatusBadge status={plugin.status} />
        </div>
        <div className="mt-1 flex items-center gap-2">
          <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">
            v{plugin.version}
          </span>
          <span className="text-[10px] text-muted-foreground">
            {plugin.plugin_type}
          </span>
        </div>
        {plugin.description && (
          <p className="mt-2 text-xs text-muted-foreground line-clamp-2">
            {plugin.description}
          </p>
        )}
        {plugin.permissions && plugin.permissions.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1">
            {plugin.permissions.map((perm) => (
              <span
                key={perm}
                className="rounded bg-amber-500/10 px-1.5 py-0.5 text-[10px] text-amber-400"
              >
                {perm}
              </span>
            ))}
          </div>
        )}
      </div>

      <div className="mt-3 flex items-center gap-2 border-t border-border pt-3">
        <button
          onClick={() => executePlugin.mutate({ id: plugin.id })}
          disabled={executePlugin.isPending || plugin.status !== "active"}
          className="inline-flex items-center gap-1 rounded-md bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary hover:bg-primary/20 transition-colors disabled:opacity-50"
        >
          <Play className="h-3 w-3" />
          Run
        </button>
        <button
          onClick={() => {
            if (confirm("Delete this plugin?")) {
              deletePlugin.mutate(plugin.id);
            }
          }}
          disabled={deletePlugin.isPending}
          className="ml-auto inline-flex items-center rounded-md p-1 text-muted-foreground hover:text-destructive transition-colors disabled:opacity-50"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
}

export default function PluginsPage() {
  const [searchQuery, setSearchQuery] = useState("");
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const { data, isLoading, isError } = usePlugins({ per_page: 50 });

  const plugins = (data?.data ?? []).filter((p: Plugin) =>
    searchQuery
      ? p.name.toLowerCase().includes(searchQuery.toLowerCase())
      : true
  );

  return (
    <>
      <div className="space-y-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">
              Plugins
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Manage custom workflow plugins with sandboxed execution and
              versioning.
            </p>
          </div>
          <button
            type="button"
            onClick={() => setIsCreateOpen(true)}
            className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            <Plus className="h-4 w-4" />
            New Plugin
          </button>
        </div>

        <div className="relative max-w-md">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="search"
            placeholder="Search plugins..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="flex h-9 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>

        {isLoading && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <div
                key={i}
                className="h-36 animate-pulse rounded-lg bg-muted"
              />
            ))}
          </div>
        )}

        {isError && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-5 py-4 text-sm text-destructive">
            Failed to load plugins. Please try refreshing.
          </div>
        )}

        {!isLoading && !isError && plugins.length === 0 && (
          <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-16 text-center">
            <Puzzle className="h-10 w-10 text-muted-foreground mb-3" />
            <p className="text-base font-medium text-foreground">
              No plugins yet
            </p>
            <p className="mt-1 text-sm text-muted-foreground">
              {searchQuery
                ? "Try adjusting your search."
                : "Create your first plugin to extend the platform."}
            </p>
            {!searchQuery && (
              <button
                type="button"
                onClick={() => setIsCreateOpen(true)}
                className="mt-4 inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
              >
                <Plus className="h-4 w-4" />
                New Plugin
              </button>
            )}
          </div>
        )}

        {!isLoading && !isError && plugins.length > 0 && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {plugins.map((plugin: Plugin) => (
              <PluginCard key={plugin.id} plugin={plugin} />
            ))}
          </div>
        )}
      </div>

      <CreatePluginDialog
        open={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
      />
    </>
  );
}
