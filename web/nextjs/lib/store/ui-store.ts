import { create } from "zustand";
import { persist } from "zustand/middleware";

interface UiState {
  sidebarCollapsed: boolean;
  mobileSidebarOpen: boolean;
  commandPaletteOpen: boolean;
  /**
   * When true, workspace tabs (Projects, Task Board, …) stay hidden even
   * though you are on Dashboard. A second click on Dashboard toggles this.
   */
  workspaceNavCollapsed: boolean;
  /** Task Board detail drawer. */
  openTaskId: string | null;
  /** Local chat title overrides (no rename API yet). */
  chatTitleOverrides: Record<string, string>;
  /**
   * The global "Run a task" dialog: open, and the agent it was opened for
   * (null = pick one). One dialog for every screen.
   */
  runDialog: { open: boolean; agentId: string | null };

  setSidebarCollapsed: (v: boolean) => void;
  toggleSidebar: () => void;
  setMobileSidebarOpen: (v: boolean) => void;
  setCommandPaletteOpen: (v: boolean) => void;
  toggleCommandPalette: () => void;
  setWorkspaceNavCollapsed: (v: boolean) => void;
  setOpenTaskId: (id: string | null) => void;
  setChatTitle: (sessionId: string, title: string) => void;
  clearChatTitle: (sessionId: string) => void;
  openRunDialog: (agentId?: string | null) => void;
  closeRunDialog: () => void;
}

export const useUiStore = create<UiState>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      mobileSidebarOpen: false,
      commandPaletteOpen: false,
      workspaceNavCollapsed: false,
      openTaskId: null,
      chatTitleOverrides: {},
      runDialog: { open: false, agentId: null },

      setSidebarCollapsed: (sidebarCollapsed) => set({ sidebarCollapsed }),
      toggleSidebar: () =>
        set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setMobileSidebarOpen: (mobileSidebarOpen) => set({ mobileSidebarOpen }),
      setCommandPaletteOpen: (commandPaletteOpen) => set({ commandPaletteOpen }),
      toggleCommandPalette: () =>
        set((s) => ({ commandPaletteOpen: !s.commandPaletteOpen })),
      setWorkspaceNavCollapsed: (workspaceNavCollapsed) =>
        set({ workspaceNavCollapsed }),
      setOpenTaskId: (openTaskId) => set({ openTaskId }),
      openRunDialog: (agentId) => set({ runDialog: { open: true, agentId: agentId ?? null }, commandPaletteOpen: false }),
      closeRunDialog: () => set({ runDialog: { open: false, agentId: null } }),
      setChatTitle: (sessionId, title) =>
        set((s) => ({
          chatTitleOverrides: { ...s.chatTitleOverrides, [sessionId]: title },
        })),
      clearChatTitle: (sessionId) =>
        set((s) => {
          const next = { ...s.chatTitleOverrides };
          delete next[sessionId];
          return { chatTitleOverrides: next };
        }),
    }),
    {
      name: "jobshout-ui",
      partialize: (s) => ({
        sidebarCollapsed: s.sidebarCollapsed,
        workspaceNavCollapsed: s.workspaceNavCollapsed,
        chatTitleOverrides: s.chatTitleOverrides,
      }),
    }
  )
);
