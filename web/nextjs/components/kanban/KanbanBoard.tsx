"use client";

import { useCallback, useMemo, useState } from "react";
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  closestCorners,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { arrayMove } from "@dnd-kit/sortable";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, Search, X } from "lucide-react";
import { toast } from "sonner";
import { fetchAllTaskPages, getProjectTasks, reorderTask } from "@/lib/api/tasks";
import { TaskCountLabel } from "@/components/task-manager/TaskProgressChip";
import { useAgents } from "@/lib/hooks/useAgents";
import { taskKeys } from "@/lib/hooks/useTasks";
import { KanbanColumn } from "@/components/kanban/KanbanColumn";
import { TaskCard } from "@/components/kanban/TaskCard";
import { useKanbanStore } from "@/lib/store/kanban-store";
import { cn } from "@/lib/utils/cn";
import type { Task } from "@/lib/types/project";
import type { TaskStatus } from "@/lib/types/common";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/** Ordered list of Kanban columns rendered left-to-right */
const COLUMN_ORDER: TaskStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "review",
  "done",
];

function groupByStatus(tasks: Task[]): Record<TaskStatus, Task[]> {
  const grouped: Record<TaskStatus, Task[]> = {
    backlog: [],
    todo: [],
    in_progress: [],
    review: [],
    done: [],
  };
  for (const task of tasks) {
    if (grouped[task.status]) grouped[task.status].push(task);
  }
  for (const status of COLUMN_ORDER) {
    grouped[status].sort((a, b) => a.position - b.position);
  }
  return grouped;
}

// ---------------------------------------------------------------------------
// KanbanBoard
// ---------------------------------------------------------------------------

interface KanbanBoardProps {
  projectId: string;
  /** Shown in the board toolbar, matching the Ops API project board. */
  projectName?: string;
  onOpenTask?: (task: Task) => void;
}

/**
 * Full Kanban board for a project.
 *
 * Architecture:
 * - Fetches all tasks for the project via TanStack Query
 * - Groups tasks by status into five columns
 * - @dnd-kit handles drag-and-drop interactions
 * - Optimistic updates mutate the cache immediately, then fire the API call
 * - DragOverlay renders a floating copy of the dragged card for visual feedback
 * - Active drag task ID is tracked in the Zustand kanban-store
 */
export function KanbanBoard({ projectId, projectName, onOpenTask }: KanbanBoardProps) {
  const queryClient = useQueryClient();
  const { activeTaskId, setActiveTask, clearActiveTask } = useKanbanStore();
  const [search, setSearch] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const { data: agentsResp } = useAgents({ per_page: 100 });
  const assigneeNames = useMemo(() => {
    const m = new Map<string, string>();
    for (const a of agentsResp?.data ?? []) m.set(a.id, a.name);
    return m;
  }, [agentsResp]);

  // ---------------------------------------------------------------------------
  // Data fetching
  // ---------------------------------------------------------------------------

  // The shared per-project key: every task mutation in the app invalidates
  // this one, so the board picks up tasks created or edited elsewhere. (An
  // ad-hoc ["tasks", projectId] key here used to match none of them.)
  const queryKey = taskKeys.projectLists(projectId);

  const { data, isLoading, isError, isFetching, refetch } = useQuery({
    queryKey,
    queryFn: () =>
      fetchAllTaskPages((page, perPage) =>
        getProjectTasks(projectId, { page, per_page: perPage })
      ),
    enabled: Boolean(projectId),
  });

  const allTasks: Task[] = data?.data ?? [];

  const visibleTasks = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return allTasks;
    return allTasks.filter(
      (t) =>
        t.title.toLowerCase().includes(q) ||
        (t.description ?? "").toLowerCase().includes(q)
    );
  }, [allTasks, search]);

  const tasksByStatus = useMemo(() => groupByStatus(allTasks), [allTasks]);
  const displayByStatus = useMemo(
    () => groupByStatus(visibleTasks),
    [visibleTasks]
  );

  // ---------------------------------------------------------------------------
  // Active task (for DragOverlay)
  // ---------------------------------------------------------------------------

  const activeTask = useMemo(
    () => (activeTaskId ? allTasks.find((t) => t.id === activeTaskId) : null),
    [activeTaskId, allTasks]
  );

  // ---------------------------------------------------------------------------
  // Mutations
  // ---------------------------------------------------------------------------

  const transitionMutation = useMutation({
    mutationFn: ({
      taskId,
      status,
      position,
    }: {
      taskId: string;
      status: TaskStatus;
      position: number;
    }) => reorderTask(taskId, { status, position }),
    onError: (error: Error) => {
      toast.error(`Failed to move task: ${error.message}`);
    },
    // Success or failure, reconcile with the server's recomputed positions —
    // the optimistic patch uses dense local indexes that only approximate them.
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey });
    },
  });

  // ---------------------------------------------------------------------------
  // DnD sensors
  // ---------------------------------------------------------------------------

  const sensors = useSensors(
    useSensor(PointerSensor, {
      // Require the pointer to move at least 8px before initiating a drag so
      // simple clicks on task cards still open the detail modal
      activationConstraint: { distance: 8 },
    })
  );

  // ---------------------------------------------------------------------------
  // DnD event handlers
  // ---------------------------------------------------------------------------

  const onDragStart = useCallback(
    (event: DragStartEvent) => {
      setActiveTask(String(event.active.id));
    },
    [setActiveTask]
  );

  const onDragEnd = useCallback(
    (event: DragEndEvent) => {
      clearActiveTask();

      const { active, over } = event;
      if (!over) return;

      const draggedTaskId = String(active.id);
      const overId = String(over.id);

      // Determine whether we're dropping onto a column (status string) or onto
      // another task card (its ID)
      const isColumn = (COLUMN_ORDER as string[]).includes(overId);

      const draggedTask = allTasks.find((t) => t.id === draggedTaskId);
      if (!draggedTask) return;

      const sourceStatus = draggedTask.status;
      const sourceColumn = tasksByStatus[sourceStatus];
      const sourceIndex = sourceColumn.findIndex((t) => t.id === draggedTaskId);
      if (sourceIndex < 0) return;

      let targetStatus: TaskStatus;
      let targetIndex: number;

      if (isColumn) {
        // Dropped onto a column's background — place at the end.
        targetStatus = overId as TaskStatus;
        targetIndex =
          targetStatus === sourceStatus
            ? tasksByStatus[targetStatus].length - 1
            : tasksByStatus[targetStatus].length;
      } else {
        // Dropped onto another card — take its place (dnd-kit sortable
        // previews the drop that way, so the result must match the preview).
        const overTask = allTasks.find((t) => t.id === overId);
        if (!overTask) return;
        targetStatus = overTask.status;
        targetIndex = tasksByStatus[targetStatus].findIndex(
          (t) => t.id === overId
        );
        if (targetIndex < 0) return;
      }

      // No-op: dropped back where it started
      if (sourceStatus === targetStatus && sourceIndex === targetIndex) {
        return;
      }

      // Final order of the affected column(s). Every neighbour is
      // repositioned, not just the dragged card — giving two cards the same
      // position used to make same-column drags a visual no-op (stable sort
      // kept the old order) while the server actually moved the task.
      const newColumns = new Map<TaskStatus, Task[]>();
      if (sourceStatus === targetStatus) {
        newColumns.set(
          targetStatus,
          arrayMove(sourceColumn, sourceIndex, targetIndex)
        );
      } else {
        newColumns.set(
          sourceStatus,
          sourceColumn.filter((t) => t.id !== draggedTaskId)
        );
        const dest = [...tasksByStatus[targetStatus]];
        dest.splice(targetIndex, 0, { ...draggedTask, status: targetStatus });
        newColumns.set(targetStatus, dest);
      }

      // The server inserts at a *stored* position ("shift everything >= p,
      // place at p") and stored positions can be sparse, so derive the wire
      // position from the neighbour the card lands after — not from the
      // dense index.
      const finalOrder = newColumns.get(targetStatus)!;
      const finalIndex = finalOrder.findIndex((t) => t.id === draggedTaskId);
      const wirePosition =
        finalIndex === 0
          ? (finalOrder[1]?.position ?? 0)
          : finalOrder[finalIndex - 1].position + 1;

      // ---------------------------------------------------------------------------
      // Optimistic update: reorder the cache immediately for instant feedback
      // ---------------------------------------------------------------------------
      queryClient.setQueryData(queryKey, (old: typeof data) => {
        if (!old) return old;

        const placement = new Map<string, { status: TaskStatus; position: number }>();
        newColumns.forEach((tasks, status) => {
          tasks.forEach((t, i) => placement.set(t.id, { status, position: i }));
        });

        const updatedTasks = old.data.map((t) => {
          const p = placement.get(t.id);
          return p ? { ...t, ...p } : t;
        });

        return { ...old, data: updatedTasks };
      });

      // Fire the API call in the background
      transitionMutation.mutate({
        taskId: draggedTaskId,
        status: targetStatus,
        position: wirePosition,
      });
    },
    [
      allTasks,
      tasksByStatus,
      queryClient,
      queryKey,
      transitionMutation,
      clearActiveTask,
    ]
  );

  const onDragCancel = useCallback(() => {
    clearActiveTask();
  }, [clearActiveTask]);

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div className="flex h-full min-h-0 flex-col bg-muted/40">
      <div className="flex items-center justify-between gap-2 border-b border-border bg-background px-3 py-3 md:px-6 md:py-4">
        <div className="flex min-w-0 items-center gap-2 md:gap-4">
          <div className="min-w-0">
            <p className="truncate text-lg font-bold md:text-xl">Board</p>
            {projectName && (
              <p className="truncate text-xs text-muted-foreground md:text-sm">
                {projectName}
              </p>
            )}
          </div>
          <span className="hidden whitespace-nowrap rounded bg-muted px-2 py-1 text-xs text-muted-foreground sm:inline-block md:text-sm">
            <TaskCountLabel loaded={allTasks.length} total={data?.total} />
          </span>
        </div>
        <div className="flex shrink-0 items-center gap-1 md:gap-2">
          {searchOpen ? (
            <div className="flex items-center gap-1">
              <input
                autoFocus
                type="search"
                placeholder="Search tasks…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="h-8 w-40 rounded-md border border-input bg-background px-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:w-64"
              />
              <button
                type="button"
                onClick={() => {
                  setSearchOpen(false);
                  setSearch("");
                }}
                className="rounded-md p-2 text-muted-foreground hover:text-foreground"
                aria-label="Close search"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
          ) : (
            <button
              type="button"
              onClick={() => setSearchOpen(true)}
              className="rounded-md p-2 text-muted-foreground hover:bg-secondary hover:text-foreground"
              aria-label="Search tasks"
            >
              <Search className="h-4 w-4" />
            </button>
          )}
          <button
            type="button"
            onClick={() => void refetch()}
            disabled={isFetching}
            className="rounded-md p-2 text-muted-foreground hover:bg-secondary hover:text-foreground disabled:opacity-50"
            aria-label="Refresh board"
          >
            <RefreshCw className={cn("h-4 w-4", isFetching && "animate-spin")} />
          </button>
        </div>
      </div>

      {isLoading ? (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          Loading board…
        </div>
      ) : isError ? (
        <div className="m-6 rounded-xl border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">Failed to load tasks</p>
          <button
            type="button"
            onClick={() => void refetch()}
            className="mt-2 h-8 rounded-md border border-border px-3 text-sm hover:bg-secondary"
          >
            Try again
          </button>
        </div>
      ) : (
        <DndContext
          sensors={sensors}
          collisionDetection={closestCorners}
          onDragStart={onDragStart}
          onDragEnd={onDragEnd}
          onDragCancel={onDragCancel}
        >
          <div className="min-h-0 flex-1 overflow-x-auto overflow-y-hidden p-3 md:p-6">
            <div className="flex h-full gap-3 md:gap-4">
              {COLUMN_ORDER.map((status) => (
                <KanbanColumn
                  key={status}
                  status={status}
                  tasks={displayByStatus[status]}
                  projectId={projectId}
                  assigneeNames={assigneeNames}
                  isDragging={Boolean(activeTaskId)}
                  onOpenTask={onOpenTask}
                />
              ))}
            </div>
          </div>

          <DragOverlay
            dropAnimation={{
              duration: 250,
              easing: "cubic-bezier(0.18, 0.67, 0.6, 1.22)",
            }}
          >
            {activeTask ? (
              <TaskCard
                task={activeTask}
                assigneeName={
                  activeTask.assigned_agent_id
                    ? assigneeNames.get(activeTask.assigned_agent_id)
                    : undefined
                }
                onOpenDetail={() => undefined}
                isOverlay
              />
            ) : null}
          </DragOverlay>
        </DndContext>
      )}
    </div>
  );
}
