"use client";

import { Suspense } from "react";
import { usePathname } from "next/navigation";
import { Menu } from "lucide-react";
import { ChatSidebar } from "./ChatSidebar";
import { CommandPalette } from "./CommandPalette";
import { PanelFrame } from "./PanelFrame";
import { useUiStore } from "@/lib/store/ui-store";
import { cn } from "@/lib/utils/cn";

function needsLegacyGutter(pathname: string): boolean {
  if (pathname.startsWith("/panel/")) return false;
  if (pathname === "/chat" || pathname.startsWith("/chat/")) return false;
  if (pathname.startsWith("/articles/")) return false;
  if (pathname.startsWith("/org-builder")) return false;
  if (pathname.startsWith("/workflows/") && pathname !== "/workflows") return false;
  if (pathname.includes("/knowledge")) return false;
  if (/^\/projects\/[^/]+$/.test(pathname)) return false;
  return true;
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const { sidebarCollapsed, setMobileSidebarOpen } = useUiStore();
  const pathname = usePathname();
  const gutter = needsLegacyGutter(pathname);

  return (
    <div className="flex h-[100dvh] flex-col bg-background lg:h-screen">
      <Suspense fallback={null}>
        <ChatSidebar />
      </Suspense>

      <div
        className={cn(
          "flex min-h-0 flex-1 flex-col transition-[margin] duration-200",
          "lg:ml-[260px]",
          sidebarCollapsed && "lg:ml-[72px]"
        )}
      >
        <header className="sticky top-0 z-20 flex h-12 shrink-0 items-center gap-2 border-b border-border bg-background px-3 lg:hidden">
          <button
            type="button"
            onClick={() => setMobileSidebarOpen(true)}
            className="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-secondary hover:text-foreground"
            aria-label="Open sidebar"
          >
            <Menu className="h-5 w-5" />
          </button>
          <span className="text-sm font-semibold">JobShout</span>
        </header>

        <main className="relative min-h-0 flex-1 overflow-y-auto scrollbar-thin">
          <PanelFrame>
            <div className={cn("h-full min-h-0", gutter && "p-6")}>{children}</div>
          </PanelFrame>
        </main>
      </div>

      <CommandPalette />
    </div>
  );
}
