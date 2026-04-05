"use client";

import { useState, useMemo } from "react";
import { useTasks } from "@/lib/hooks/useTasks";
import { useAgents } from "@/lib/hooks/useAgents";
import { apiClient } from "@/lib/api/client";
import { toast } from "sonner";

interface TaskNode {
  id: string;
  title: string;
  description: string | null;
  status: string;
  priority: string;
  parent_id: string | null;
  assigned_agent_id: string | null;
  due_date: string | null;
  story_points: number | null;
  depth: number;
  children: TaskNode[];
  expanded: boolean;
}

const STATUS_CONFIG: Record<string, { label: string; color: string; bg: string }> = {
  backlog: { label: "Backlog", color: "text-gray-600", bg: "bg-gray-100 dark:bg-gray-800" },
  todo: { label: "To Do", color: "text-blue-600", bg: "bg-blue-100 dark:bg-blue-900/30" },
  in_progress: { label: "In Progress", color: "text-yellow-600", bg: "bg-yellow-100 dark:bg-yellow-900/30" },
  review: { label: "Review", color: "text-purple-600", bg: "bg-purple-100 dark:bg-purple-900/30" },
  done: { label: "Done", color: "text-green-600", bg: "bg-green-100 dark:bg-green-900/30" },
};

const PRIORITY_CONFIG: Record<string, { label: string; color: string }> = {
  low: { label: "Low", color: "text-gray-500" },
  medium: { label: "Med", color: "text-blue-500" },
  high: { label: "High", color: "text-orange-500" },
  critical: { label: "Crit", color: "text-red-500" },
};

export default function TaskManagerPage() {
  const { data: tasksResponse, isLoading, refetch } = useTasks();
  const { data: agentsResponse } = useAgents();
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const [filterStatus, setFilterStatus] = useState<string>("all");
  const [filterPriority, setFilterPriority] = useState<string>("all");
  const [showAddForm, setShowAddForm] = useState<string | null>(null); // parent_id or "root"
  const [newTaskTitle, setNewTaskTitle] = useState("");
  const [newTaskPriority, setNewTaskPriority] = useState("medium");

  const agents = agentsResponse?.data ?? [];
  const rawTasks = tasksResponse?.data ?? [];

  // Build hierarchical tree
  const taskTree = useMemo(() => {
    const taskMap = new Map<string, TaskNode>();
    const roots: TaskNode[] = [];

    // Create nodes
    for (const t of rawTasks) {
      taskMap.set(t.id, {
        id: t.id,
        title: t.title,
        description: t.description,
        status: t.status,
        priority: t.priority,
        parent_id: t.parent_id,
        assigned_agent_id: t.assigned_agent_id,
        due_date: t.due_date,
        story_points: t.story_points,
        depth: 0,
        children: [],
        expanded: expandedIds.has(t.id),
      });
    }

    // Build tree
    for (const node of Array.from(taskMap.values())) {
      if (node.parent_id && taskMap.has(node.parent_id)) {
        const parent = taskMap.get(node.parent_id)!;
        node.depth = parent.depth + 1;
        parent.children.push(node);
      } else {
        roots.push(node);
      }
    }

    // Calculate depths recursively
    function setDepths(nodes: TaskNode[], depth: number) {
      for (const n of nodes) {
        n.depth = depth;
        setDepths(n.children, depth + 1);
      }
    }
    setDepths(roots, 0);

    return roots;
  }, [rawTasks, expandedIds]);

  // Flatten for display (respecting expanded state)
  const flatList = useMemo(() => {
    const result: TaskNode[] = [];
    function traverse(nodes: TaskNode[]) {
      for (const node of nodes) {
        // Apply filters
        if (filterStatus !== "all" && node.status !== filterStatus) continue;
        if (filterPriority !== "all" && node.priority !== filterPriority) continue;
        result.push(node);
        if (expandedIds.has(node.id) && node.children.length > 0) {
          traverse(node.children);
        }
      }
    }
    traverse(taskTree);
    return result;
  }, [taskTree, expandedIds, filterStatus, filterPriority]);

  function toggleExpand(id: string) {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function expandAll() {
    const allIds = new Set(rawTasks.map((t) => t.id));
    setExpandedIds(allIds);
  }

  function collapseAll() {
    setExpandedIds(new Set());
  }

  async function handleAddSubtask(parentId: string | null) {
    if (!newTaskTitle.trim()) return;
    // Need a project_id - use the first task's project_id or fallback
    const projectId = rawTasks[0]?.project_id;
    if (!projectId) {
      toast.error("No project found. Create a project first.");
      return;
    }

    try {
      await apiClient.post("/tasks", {
        project_id: projectId,
        title: newTaskTitle,
        priority: newTaskPriority,
        parent_id: parentId === "root" ? undefined : parentId,
      });
      toast.success("Task created.");
      setNewTaskTitle("");
      setShowAddForm(null);
      refetch();
    } catch {
      toast.error("Failed to create task.");
    }
  }

  async function handleStatusChange(taskId: string, newStatus: string) {
    try {
      await apiClient.patch(`/tasks/${taskId}/transition`, { status: newStatus });
      refetch();
    } catch {
      toast.error("Failed to update status.");
    }
  }

  // Stats
  const stats = useMemo(() => {
    const total = rawTasks.length;
    const done = rawTasks.filter((t) => t.status === "done").length;
    const inProgress = rawTasks.filter((t) => t.status === "in_progress").length;
    const rootCount = rawTasks.filter((t) => !t.parent_id).length;
    const maxDepth = Math.max(0, ...rawTasks.map(() => 0)); // simplified
    return { total, done, inProgress, rootCount, maxDepth };
  }, [rawTasks]);

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Multi-Level Task Manager</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Hierarchical task management with parent-child relationships and dependencies.
          </p>
        </div>
        <button
          onClick={() => setShowAddForm(showAddForm === "root" ? null : "root")}
          className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
        >
          {showAddForm === "root" ? "Cancel" : "Add Root Task"}
        </button>
      </div>

      {/* Stats bar */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div className="rounded-lg border border-border bg-card p-3 text-center">
          <p className="text-2xl font-bold">{stats.total}</p>
          <p className="text-xs text-muted-foreground">Total Tasks</p>
        </div>
        <div className="rounded-lg border border-border bg-card p-3 text-center">
          <p className="text-2xl font-bold text-green-600">{stats.done}</p>
          <p className="text-xs text-muted-foreground">Completed</p>
        </div>
        <div className="rounded-lg border border-border bg-card p-3 text-center">
          <p className="text-2xl font-bold text-yellow-600">{stats.inProgress}</p>
          <p className="text-xs text-muted-foreground">In Progress</p>
        </div>
        <div className="rounded-lg border border-border bg-card p-3 text-center">
          <p className="text-2xl font-bold">{stats.rootCount}</p>
          <p className="text-xs text-muted-foreground">Root Tasks</p>
        </div>
      </div>

      {/* Filters & controls */}
      <div className="flex flex-wrap items-center gap-3">
        <select
          value={filterStatus}
          onChange={(e) => setFilterStatus(e.target.value)}
          className="flex h-8 rounded-md border border-input bg-background px-2 text-xs"
        >
          <option value="all">All Status</option>
          {Object.entries(STATUS_CONFIG).map(([k, v]) => (
            <option key={k} value={k}>{v.label}</option>
          ))}
        </select>
        <select
          value={filterPriority}
          onChange={(e) => setFilterPriority(e.target.value)}
          className="flex h-8 rounded-md border border-input bg-background px-2 text-xs"
        >
          <option value="all">All Priority</option>
          {Object.entries(PRIORITY_CONFIG).map(([k, v]) => (
            <option key={k} value={k}>{v.label}</option>
          ))}
        </select>
        <button onClick={expandAll} className="inline-flex h-8 items-center rounded-md border border-input bg-background px-3 text-xs hover:bg-accent">
          Expand All
        </button>
        <button onClick={collapseAll} className="inline-flex h-8 items-center rounded-md border border-input bg-background px-3 text-xs hover:bg-accent">
          Collapse All
        </button>
      </div>

      {/* Add root task form */}
      {showAddForm === "root" && (
        <div className="flex items-center gap-2 rounded-lg border border-border bg-card p-3">
          <input
            type="text"
            value={newTaskTitle}
            onChange={(e) => setNewTaskTitle(e.target.value)}
            placeholder="New task title..."
            autoFocus
            onKeyDown={(e) => e.key === "Enter" && handleAddSubtask(null)}
            className="flex h-8 flex-1 rounded-md border border-input bg-background px-3 text-sm"
          />
          <select
            value={newTaskPriority}
            onChange={(e) => setNewTaskPriority(e.target.value)}
            className="flex h-8 rounded-md border border-input bg-background px-2 text-xs"
          >
            <option value="low">Low</option>
            <option value="medium">Medium</option>
            <option value="high">High</option>
            <option value="critical">Critical</option>
          </select>
          <button
            onClick={() => handleAddSubtask(null)}
            className="inline-flex h-8 items-center rounded-md bg-primary px-3 text-xs font-medium text-primary-foreground hover:bg-primary/90"
          >
            Add
          </button>
        </div>
      )}

      {/* Task tree */}
      {isLoading ? (
        <div className="flex items-center justify-center py-12">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        </div>
      ) : flatList.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border p-12 text-center">
          <p className="text-sm text-muted-foreground">No tasks found.</p>
        </div>
      ) : (
        <div className="rounded-xl border border-border bg-card">
          {/* Header */}
          <div className="grid grid-cols-12 gap-2 border-b border-border px-4 py-2 text-xs font-medium text-muted-foreground">
            <div className="col-span-5">Task</div>
            <div className="col-span-2">Status</div>
            <div className="col-span-1">Priority</div>
            <div className="col-span-2">Agent</div>
            <div className="col-span-2 text-right">Actions</div>
          </div>

          {/* Rows */}
          {flatList.map((task) => (
            <div key={task.id}>
              <div className="grid grid-cols-12 items-center gap-2 border-b border-border/50 px-4 py-2 hover:bg-accent/30">
                {/* Task name with indent */}
                <div className="col-span-5 flex items-center gap-1">
                  <div style={{ width: `${task.depth * 24}px` }} className="flex-shrink-0" />
                  {task.children.length > 0 ? (
                    <button
                      onClick={() => toggleExpand(task.id)}
                      className="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded text-xs text-muted-foreground hover:bg-accent"
                    >
                      {expandedIds.has(task.id) ? "v" : ">"}
                    </button>
                  ) : (
                    <div className="w-5 flex-shrink-0" />
                  )}
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{task.title}</p>
                    {task.children.length > 0 && (
                      <p className="text-xs text-muted-foreground">
                        {task.children.length} subtask{task.children.length > 1 ? "s" : ""}
                      </p>
                    )}
                  </div>
                </div>

                {/* Status */}
                <div className="col-span-2">
                  <select
                    value={task.status}
                    onChange={(e) => handleStatusChange(task.id, e.target.value)}
                    className={`h-7 rounded-md border-0 px-2 text-xs font-medium ${STATUS_CONFIG[task.status]?.bg ?? ""} ${STATUS_CONFIG[task.status]?.color ?? ""}`}
                  >
                    {Object.entries(STATUS_CONFIG).map(([k, v]) => (
                      <option key={k} value={k}>{v.label}</option>
                    ))}
                  </select>
                </div>

                {/* Priority */}
                <div className="col-span-1">
                  <span className={`text-xs font-medium ${PRIORITY_CONFIG[task.priority]?.color ?? ""}`}>
                    {PRIORITY_CONFIG[task.priority]?.label ?? task.priority}
                  </span>
                </div>

                {/* Agent */}
                <div className="col-span-2">
                  {task.assigned_agent_id ? (
                    <span className="text-xs">
                      {agents.find((a) => a.id === task.assigned_agent_id)?.name ?? "Agent"}
                    </span>
                  ) : (
                    <span className="text-xs text-muted-foreground">Unassigned</span>
                  )}
                </div>

                {/* Actions */}
                <div className="col-span-2 flex items-center justify-end gap-1">
                  <button
                    onClick={() =>
                      setShowAddForm(showAddForm === task.id ? null : task.id)
                    }
                    title="Add subtask"
                    className="inline-flex h-6 items-center rounded border border-input bg-background px-2 text-xs hover:bg-accent"
                  >
                    + Sub
                  </button>
                  {task.due_date && (
                    <span className="text-xs text-muted-foreground">
                      {new Date(task.due_date).toLocaleDateString()}
                    </span>
                  )}
                </div>
              </div>

              {/* Inline add subtask form */}
              {showAddForm === task.id && (
                <div
                  className="flex items-center gap-2 border-b border-border/50 bg-accent/20 px-4 py-2"
                  style={{ paddingLeft: `${(task.depth + 1) * 24 + 40}px` }}
                >
                  <input
                    type="text"
                    value={newTaskTitle}
                    onChange={(e) => setNewTaskTitle(e.target.value)}
                    placeholder={`Add subtask under "${task.title}"...`}
                    autoFocus
                    onKeyDown={(e) => e.key === "Enter" && handleAddSubtask(task.id)}
                    className="flex h-7 flex-1 rounded-md border border-input bg-background px-2 text-xs"
                  />
                  <select
                    value={newTaskPriority}
                    onChange={(e) => setNewTaskPriority(e.target.value)}
                    className="flex h-7 rounded-md border border-input bg-background px-2 text-xs"
                  >
                    <option value="low">Low</option>
                    <option value="medium">Med</option>
                    <option value="high">High</option>
                    <option value="critical">Crit</option>
                  </select>
                  <button
                    onClick={() => handleAddSubtask(task.id)}
                    className="inline-flex h-7 items-center rounded-md bg-primary px-2 text-xs font-medium text-primary-foreground"
                  >
                    Add
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
