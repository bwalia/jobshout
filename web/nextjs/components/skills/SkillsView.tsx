"use client";

import { useState } from "react";
import {
  Search,
  Plus,
  Trash2,
  Sparkles,
  Wrench,
  MessageSquareText,
  Boxes,
} from "lucide-react";
import {
  useSkills,
  useCreateSkill,
  useDeleteSkill,
  useUpdateSkill,
} from "@/lib/hooks/useSkills";
import type { Skill, SkillKind, CreateSkillRequest } from "@/lib/api/skills";
import { THEME_BADGE } from "@/lib/status-colors";

const KIND_META: Record<
  SkillKind,
  { label: string; icon: React.ElementType; color: string }
> = {
  tool: { label: "Tool", icon: Wrench, color: "text-sky-700 dark:text-sky-400" },
  prompt: { label: "Prompt", icon: MessageSquareText, color: "text-violet-700 dark:text-violet-400" },
  bundle: { label: "Bundle", icon: Boxes, color: "text-amber-700 dark:text-amber-400" },
};

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    published: THEME_BADGE.success,
    draft: THEME_BADGE.warning,
    deprecated: THEME_BADGE.muted,
  };
  return (
    <span
      className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors[status] ?? "bg-muted text-muted-foreground"}`}
    >
      {status}
    </span>
  );
}

function CreateSkillDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const createSkill = useCreateSkill();
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [kind, setKind] = useState<SkillKind>("tool");
  const [version, setVersion] = useState("0.1.0");
  const [configJSON, setConfigJSON] = useState('{\n  "tool": "http_request"\n}');
  const [jsonError, setJsonError] = useState<string | null>(null);

  if (!open) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    let parsedConfig: Record<string, unknown> = {};
    try {
      parsedConfig = configJSON.trim() ? JSON.parse(configJSON) : {};
    } catch {
      setJsonError("Config must be valid JSON.");
      return;
    }
    setJsonError(null);

    const payload: CreateSkillRequest = {
      slug,
      name,
      kind,
      version: version || undefined,
      description: description || undefined,
      config_json: parsedConfig,
    };

    createSkill.mutate(payload, {
      onSuccess: () => {
        setSlug("");
        setName("");
        setDescription("");
        setKind("tool");
        setVersion("0.1.0");
        setConfigJSON('{\n  "tool": "http_request"\n}');
        onClose();
      },
    });
  };

  const configHint =
    kind === "tool"
      ? 'e.g. {"tool": "http_request"} or {"tools": ["http_request", "shell_command"]}'
      : kind === "prompt"
        ? 'e.g. {"prompt": "Always answer concisely."}'
        : "Bundle config is reserved for a future phase.";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-lg rounded-lg border border-border bg-card p-6 shadow-xl space-y-4"
      >
        <h2 className="text-lg font-semibold text-foreground">Create Skill</h2>

        <div className="flex gap-3">
          <div className="flex-1">
            <label className="text-xs text-muted-foreground">Name</label>
            <input
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
              placeholder="Web Fetch"
            />
          </div>
          <div className="flex-1">
            <label className="text-xs text-muted-foreground">Slug</label>
            <input
              required
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm font-mono"
              placeholder="web-fetch"
            />
          </div>
        </div>

        <div className="flex gap-3">
          <div className="flex-1">
            <label className="text-xs text-muted-foreground">Kind</label>
            <select
              value={kind}
              onChange={(e) => setKind(e.target.value as SkillKind)}
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
            >
              <option value="tool">Tool</option>
              <option value="prompt">Prompt</option>
              <option value="bundle">Bundle</option>
            </select>
          </div>
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
            placeholder="What capability does this skill grant?"
          />
        </div>

        <div>
          <label className="text-xs text-muted-foreground">
            Config (JSON)
          </label>
          <textarea
            value={configJSON}
            onChange={(e) => setConfigJSON(e.target.value)}
            rows={5}
            className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm font-mono"
          />
          <p className="mt-1 text-[11px] text-muted-foreground">{configHint}</p>
          {jsonError && (
            <p className="mt-1 text-[11px] text-destructive">{jsonError}</p>
          )}
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
            disabled={createSkill.isPending || !name || !slug}
            className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
          >
            {createSkill.isPending ? "Creating..." : "Create"}
          </button>
        </div>
      </form>
    </div>
  );
}

function SkillCard({ skill }: { skill: Skill }) {
  const deleteSkill = useDeleteSkill();
  const updateSkill = useUpdateSkill();
  const meta = KIND_META[skill.kind] ?? KIND_META.tool;
  const Icon = meta.icon;
  const isBuiltIn = !skill.org_id;

  const toggleStatus = () => {
    const next = skill.status === "published" ? "deprecated" : "published";
    updateSkill.mutate({ id: skill.id, payload: { status: next } });
  };

  return (
    <div className="group relative flex flex-col justify-between rounded-lg border border-border bg-card p-4 transition-colors hover:border-primary/40">
      <div>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Icon className={`h-4 w-4 ${meta.color}`} />
            <h3 className="text-sm font-semibold text-foreground">
              {skill.name}
            </h3>
          </div>
          <StatusBadge status={skill.status} />
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">
            {skill.slug}
          </span>
          <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">
            v{skill.version}
          </span>
          <span className="text-[10px] text-muted-foreground">{meta.label}</span>
          {isBuiltIn && (
            <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] text-primary">
              built-in
            </span>
          )}
        </div>
        {skill.description && (
          <p className="mt-2 text-xs text-muted-foreground line-clamp-3">
            {skill.description}
          </p>
        )}
      </div>

      <div className="mt-3 flex items-center gap-2 border-t border-border pt-3">
        <button
          onClick={toggleStatus}
          disabled={updateSkill.isPending || isBuiltIn}
          title={
            isBuiltIn
              ? "Built-in skills cannot be modified"
              : skill.status === "published"
                ? "Deprecate"
                : "Publish"
          }
          className="inline-flex items-center gap-1 rounded-md bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary hover:bg-primary/20 transition-colors disabled:opacity-40"
        >
          {skill.status === "published" ? "Deprecate" : "Publish"}
        </button>
        {!isBuiltIn && (
          <button
            onClick={() => {
              if (confirm(`Delete skill "${skill.name}"?`)) {
                deleteSkill.mutate(skill.id);
              }
            }}
            disabled={deleteSkill.isPending}
            className="ml-auto inline-flex items-center rounded-md p-1 text-muted-foreground hover:text-destructive transition-colors disabled:opacity-50"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
    </div>
  );
}

export function SkillsView({
  hideHeader = false,
}: {
  hideHeader?: boolean;
}) {
  const [searchQuery, setSearchQuery] = useState("");
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const { data, isLoading, isError } = useSkills();

  const skills = (data ?? []).filter((s) =>
    searchQuery
      ? s.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        s.slug.toLowerCase().includes(searchQuery.toLowerCase())
      : true
  );

  return (
    <>
      <div className="space-y-6">
        <div className="flex items-start justify-between gap-4">
          {!hideHeader && (
            <div>
              <h1 className="text-2xl font-bold tracking-tight text-foreground">
                Skills
              </h1>
              <p className="mt-1 text-sm text-muted-foreground">
                Reusable capability bundles — tools and prompt patches — that you
                enable per agent to shape how it works.
              </p>
            </div>
          )}
          <button
            type="button"
            onClick={() => setIsCreateOpen(true)}
            className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            <Plus className="h-4 w-4" />
            New Skill
          </button>
        </div>

        <div className="relative max-w-md">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="search"
            placeholder="Search skills..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="flex h-9 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>

        {isLoading && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="h-40 animate-pulse rounded-lg bg-muted" />
            ))}
          </div>
        )}

        {isError && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-5 py-4 text-sm text-destructive">
            Failed to load skills. Please try refreshing.
          </div>
        )}

        {!isLoading && !isError && skills.length === 0 && (
          <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-16 text-center">
            <Sparkles className="h-10 w-10 text-muted-foreground mb-3" />
            <p className="text-base font-medium text-foreground">
              No skills yet
            </p>
            <p className="mt-1 text-sm text-muted-foreground">
              {searchQuery
                ? "Try adjusting your search."
                : "Create a skill to grant your agents a reusable capability."}
            </p>
            {!searchQuery && (
              <button
                type="button"
                onClick={() => setIsCreateOpen(true)}
                className="mt-4 inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
              >
                <Plus className="h-4 w-4" />
                New Skill
              </button>
            )}
          </div>
        )}

        {!isLoading && !isError && skills.length > 0 && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {skills.map((skill) => (
              <SkillCard key={skill.id} skill={skill} />
            ))}
          </div>
        )}
      </div>

      <CreateSkillDialog
        open={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
      />
    </>
  );
}

