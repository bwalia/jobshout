"use client";

import { useState } from "react";
import { X } from "lucide-react";
import { useCreateProject } from "@/lib/hooks/useProjects";
import type { Priority } from "@/lib/types/common";
import type { Project } from "@/lib/types/project";

const PRIORITIES: Priority[] = ["low", "medium", "high", "critical"];

export function NewProjectDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (project: Project) => void;
}) {
  const createProject = useCreateProject();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<Priority>("medium");
  const [dueDate, setDueDate] = useState("");

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    const project = await createProject.mutateAsync({
      name: name.trim(),
      description: description.trim() || undefined,
      priority,
      due_date: dueDate || undefined,
    });
    onCreated(project);
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onClick={onClose}
      role="dialog"
      aria-label="New project"
    >
      <form
        onSubmit={handleSubmit}
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md space-y-4 rounded-xl border border-border bg-card p-5"
      >
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold">New Project</h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-muted-foreground hover:bg-secondary hover:text-foreground"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="space-y-1.5">
          <label htmlFor="project-name" className="text-sm font-medium">
            Name
          </label>
          <input
            id="project-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            autoFocus
            className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>

        <div className="space-y-1.5">
          <label htmlFor="project-desc" className="text-sm font-medium">
            Description
          </label>
          <textarea
            id="project-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1.5">
            <label htmlFor="project-priority" className="text-sm font-medium">
              Priority
            </label>
            <select
              id="project-priority"
              value={priority}
              onChange={(e) => setPriority(e.target.value as Priority)}
              className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm capitalize focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {PRIORITIES.map((p) => (
                <option key={p} value={p} className="capitalize">
                  {p[0].toUpperCase() + p.slice(1)}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1.5">
            <label htmlFor="project-due" className="text-sm font-medium">
              Due date
            </label>
            <input
              id="project-due"
              type="date"
              value={dueDate}
              onChange={(e) => setDueDate(e.target.value)}
              className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-1">
          <button
            type="button"
            onClick={onClose}
            className="h-9 rounded-md border border-border px-4 text-sm hover:bg-secondary"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={createProject.isPending || !name.trim()}
            className="h-9 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
          >
            {createProject.isPending ? "Creating…" : "Create Project"}
          </button>
        </div>
      </form>
    </div>
  );
}
