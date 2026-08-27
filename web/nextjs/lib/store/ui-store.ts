import { create } from "zustand";
import { persist } from "zustand/middleware";

interface UiState {
  sidebarCollapsed: boolean;
  mobileSidebarOpen: boolean;
  commandPaletteOpen: boolean;
  /** Task Board detail drawer. */
  openTaskId: string | null;
  /** Local chat title overrides (no rename API yet). */
  chatTitleOverrides: Record<string, string>;

  setSidebarCollapsed: (v: boolean) => void;
  toggleSidebar: () => void;
  setMobileSidebarOpen: (v: boolean) => void;
  setCommandPaletteOpen: (v: boolean) => void;
  toggleCommandPalette: () => void;
  setOpenTaskId: (id: string | null) => void;
  setChatTitle: (sessionId: string, title: string) => void;
  clearChatTitle: (sessionId: string) => void;
}

export const useUiStore = create<UiState>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      mobileSidebarOpen: false,
      commandPaletteOpen: false,
      openTaskId: null,
      chatTitleOverrides: {},

      setSidebarCollapsed: (sidebarCollapsed) => set({ sidebarCollapsed }),
      toggleSidebar: () =>
        set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setMobileSidebarOpen: (mobileSidebarOpen) => set({ mobileSidebarOpen }),
      setCommandPaletteOpen: (commandPaletteOpen) => set({ commandPaletteOpen }),
      toggleCommandPalette: () =>
        set((s) => ({ commandPaletteOpen: !s.commandPaletteOpen })),
      setOpenTaskId: (openTaskId) => set({ openTaskId }),
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
        chatTitleOverrides: s.chatTitleOverrides,
      }),
    }
  )
);
