import { create } from "zustand";

/**
 * Lightweight Zustand store that tracks which task card is currently being
 * dragged on the Kanban board.
 *
 * Keeping this state outside of React avoids prop-drilling through
 * DndContext → KanbanColumn → TaskCard and allows the DragOverlay to read
 * the active task ID from anywhere in the component tree.
 */
interface KanbanDragState {
  /** ID of the task currently being dragged; null when no drag is active */
  activeTaskId: string | null;

  /** Store the active task id when a drag starts */
  setActiveTask: (taskId: string) => void;

  /** Clear the active task id when a drag ends or is cancelled */
  clearActiveTask: () => void;
}

export const useKanbanStore = create<KanbanDragState>((set) => ({
  activeTaskId: null,

  setActiveTask: (taskId: string) => set({ activeTaskId: taskId }),

  clearActiveTask: () => set({ activeTaskId: null }),
}));
