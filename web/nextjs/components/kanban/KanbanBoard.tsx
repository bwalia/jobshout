"use client";

import { useCallback, useMemo } from "react";
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { arrayMove } from "@dnd-kit/sortable";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { getProjectTasks, updateTask, reorderTask } from "@/lib/api/tasks";
import { KanbanColumn } from "@/components/kanban/KanbanColumn";
import { TaskCard } from "@/components/kanban/TaskCard";
import { useKanbanStore } from "@/lib/store/kanban-store";
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

// ---------------------------------------------------------------------------
// KanbanBoard
// ---------------------------------------------------------------------------

interface KanbanBoardProps {
  projectId: string;
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
export function KanbanBoard({ projectId }: KanbanBoardProps) {
  const queryClient = useQueryClient();
  const { activeTaskId, setActiveTask, clearActiveTask } = useKanbanStore();

  // ---------------------------------------------------------------------------
  // Data fetching
  // ---------------------------------------------------------------------------

  const queryKey = ["tasks", projectId] as const;

  const { data, isLoading, isError } = useQuery({
    queryKey,
    queryFn: () => getProjectTasks(projectId, { per_page: 200 }),
    enabled: Boolean(projectId),
  });

  const allTasks: Task[] = data?.data ?? [];

  // ---------------------------------------------------------------------------
  // Derived: tasks grouped by status
  // ---------------------------------------------------------------------------

  const tasksByStatus = useMemo(() => {
    const grouped: Record<TaskStatus, Task[]> = {
      backlog: [],
      todo: [],
      in_progress: [],
      review: [],
      done: [],
    };

    for (const task of allTasks) {
      if (grouped[task.status]) {
        grouped[task.status].push(task);
      }
    }

    // Sort each column by position so the server-defined order is respected
    for (const status of COLUMN_ORDER) {
      grouped[status].sort((a, b) => a.position - b.position);
    }

    return grouped;
  }, [allTasks]);

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
    onError: (error: Error, variables) => {
      // Roll back optimistic update on failure
      queryClient.invalidateQueries({ queryKey });
      toast.error(`Failed to move task: ${error.message}`);
    },
  });

  // ---------------------------------------------------------------------------
  // DnD sensors
  // ---------------------------------------------------------------------------

  const sensors = useSensors(
    useSensor(PointerSensor, {
      // Require the pointer to move at least 5px before initiating a drag so
      // simple clicks on task cards still open the detail modal
      activationConstraint: { distance: 5 },
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

      let targetStatus: TaskStatus;
      let targetPosition: number;

      if (isColumn) {
        // Dropped directly onto a column — place at end
        targetStatus = overId as TaskStatus;
        targetPosition = tasksByStatus[targetStatus].length;
      } else {
        // Dropped onto another task card — find the target task's column + position
        const overTask = allTasks.find((t) => t.id === overId);
        if (!overTask) return;
        targetStatus = overTask.status;
        targetPosition = overTask.position;
      }

      // No-op: same position in same column
      if (
        draggedTask.status === targetStatus &&
        draggedTask.position === targetPosition
      ) {
        return;
      }

      // ---------------------------------------------------------------------------
      // Optimistic update: reorder the cache immediately for instant feedback
      // ---------------------------------------------------------------------------
      queryClient.setQueryData(queryKey, (old: typeof data) => {
        if (!old) return old;

        const updatedTasks = old.data.map((t) => {
          if (t.id === draggedTaskId) {
            return { ...t, status: targetStatus, position: targetPosition };
          }
          return t;
        });

        return { ...old, data: updatedTasks };
      });

      // Fire the API call in the background
      transitionMutation.mutate({
        taskId: draggedTaskId,
        status: targetStatus,
        position: targetPosition,
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

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex flex-col items-center gap-3 text-muted-foreground">
          <svg
            className="h-8 w-8 animate-spin"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              className="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              strokeWidth="4"
            />
            <path
              className="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
            />
          </svg>
          <span className="text-sm">Loading board…</span>
        </div>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="rounded-xl border border-destructive/50 bg-destructive/10 p-8 text-center">
          <p className="font-medium text-destructive">Failed to load tasks</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Please refresh the page or try again later.
          </p>
        </div>
      </div>
    );
  }

  return (
    <DndContext
      sensors={sensors}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragCancel={onDragCancel}
    >
      {/* Horizontally scrollable board surface */}
      <div className="flex h-full gap-4 overflow-x-auto pb-4">
        {COLUMN_ORDER.map((status) => (
          <KanbanColumn
            key={status}
            status={status}
            tasks={tasksByStatus[status]}
            projectId={projectId}
          />
        ))}
      </div>

      {/* Floating drag preview shown while a card is being dragged */}
      <DragOverlay>
        {activeTask ? (
          <div className="rotate-2 opacity-90">
            {/*
              Render a non-interactive copy of the card.
              We pass a no-op handler for onOpenDetail since this is a visual
              overlay only — the click will never fire during a drag.
            */}
            <TaskCard task={activeTask} onOpenDetail={() => undefined} />
          </div>
        ) : null}
      </DragOverlay>
    </DndContext>
  );
}
