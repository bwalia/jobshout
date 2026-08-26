"use client";

import { Suspense } from "react";
import { Menu } from "lucide-react";
import { ChatSidebar } from "./ChatSidebar";
import { CommandPalette } from "./CommandPalette";
import { PanelFrame } from "./PanelFrame";
import { useUiStore } from "@/lib/store/ui-store";
import { cn } from "@/lib/utils/cn";

export function AppShell({ children }: { children: React.ReactNode }) {
  const { sidebarCollapsed, setMobileSidebarOpen } = useUiStore();

  return (
    <div className="min-h-screen bg-background">
      <Suspense fallback={null}>
        <ChatSidebar />
      </Suspense>

      <div
        className={cn(
          "flex min-h-screen flex-col transition-[margin] duration-200",
          "lg:ml-[260px]",
          sidebarCollapsed && "lg:ml-[72px]"
        )}
      >
        {/* Mobile top bar */}
        <header className="sticky top-0 z-20 flex h-12 shrink-0 items-center gap-2 border-b border-border bg-background px-3 lg:hidden">
          <button
            type="button"
            onClick={() => setMobileSidebarOpen(true)}
            className="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-secondary hover:text-foreground"
            aria-label="Open menu"
          >
            <Menu className="h-5 w-5" />
          </button>
          <span className="text-sm font-semibold">JobShout</span>
        </header>

        <main className="relative flex-1 overflow-y-auto scrollbar-thin">
          <PanelFrame>{children}</PanelFrame>
        </main>
      </div>

      <CommandPalette />
    </div>
  );
}
