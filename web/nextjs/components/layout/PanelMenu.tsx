"use client";

import { useCallback, useEffect, useId, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Menu } from "lucide-react";
import { cn } from "@/lib/utils/cn";
import { PANELS, panelFromPath, panelIndex, type PanelId } from "@/lib/panels";
import { useUiStore } from "@/lib/store/ui-store";

const CLOSE_DELAY_MS = 180;

export function PanelMenu({ collapsed }: { collapsed?: boolean }) {
  const router = useRouter();
  const pathname = usePathname();
  const activeId = panelFromPath(pathname);
  const { panelMenuOpen, setPanelMenuOpen } = useUiStore();
  const [focusIdx, setFocusIdx] = useState(0);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const listId = useId();

  const open = useCallback(() => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    setPanelMenuOpen(true);
    setFocusIdx(Math.max(0, panelIndex(activeId)));
  }, [activeId, setPanelMenuOpen]);

  const scheduleClose = useCallback(() => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    closeTimer.current = setTimeout(() => setPanelMenuOpen(false), CLOSE_DELAY_MS);
  }, [setPanelMenuOpen]);

  const close = useCallback(() => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    setPanelMenuOpen(false);
  }, [setPanelMenuOpen]);

  const navigate = useCallback(
    (id: PanelId, href: string) => {
      // Store previous for transition direction (read by PanelFrame).
      try {
        sessionStorage.setItem("jobshout-panel-from", activeId);
        sessionStorage.setItem("jobshout-panel-to", id);
      } catch {
        /* ignore */
      }
      close();
      router.push(href);
    },
    [activeId, close, router]
  );

  useEffect(() => {
    if (!panelMenuOpen) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        close();
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setFocusIdx((i) => (i + 1) % PANELS.length);
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        setFocusIdx((i) => (i - 1 + PANELS.length) % PANELS.length);
      } else if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        const p = PANELS[focusIdx];
        if (p) navigate(p.id, p.href);
      } else if (e.key === "Home") {
        e.preventDefault();
        setFocusIdx(0);
      } else if (e.key === "End") {
        e.preventDefault();
        setFocusIdx(PANELS.length - 1);
      }
    }
    function onPointer(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        close();
      }
    }
    window.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onPointer);
    return () => {
      window.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onPointer);
    };
  }, [panelMenuOpen, close, focusIdx, navigate]);

  useEffect(() => {
    return () => {
      if (closeTimer.current) clearTimeout(closeTimer.current);
    };
  }, []);

  // Roving focus: keep DOM focus on the highlighted item while open.
  useEffect(() => {
    if (panelMenuOpen) itemRefs.current[focusIdx]?.focus();
  }, [panelMenuOpen, focusIdx]);

  return (
    <div
      ref={rootRef}
      className="relative"
      onMouseEnter={open}
      onMouseLeave={scheduleClose}
    >
      <button
        type="button"
        onClick={() => (panelMenuOpen ? close() : open())}
        aria-haspopup="menu"
        aria-expanded={panelMenuOpen}
        aria-controls={listId}
        aria-label="Open panels"
        className={cn(
          "flex h-10 w-10 items-center justify-center rounded-lg text-sidebar-foreground transition-colors",
          "hover:bg-sidebar-muted hover:text-foreground",
          panelMenuOpen && "bg-sidebar-muted text-foreground"
        )}
      >
        <Menu className="h-5 w-5" />
      </button>

      {panelMenuOpen && (
        <div
          id={listId}
          role="menu"
          aria-label="Panels"
          className={cn(
            "absolute left-0 top-full z-50 mt-1 min-w-[220px] overflow-hidden rounded-lg border border-border bg-popover py-1 shadow-card-hover",
            collapsed && "left-0"
          )}
          onMouseEnter={open}
          onMouseLeave={scheduleClose}
        >
          {PANELS.map((panel, idx) => {
            const Icon = panel.icon;
            const isActive = panel.id === activeId;
            const focused = idx === focusIdx;
            return (
              <button
                key={panel.id}
                type="button"
                role="menuitem"
                ref={(el) => {
                  itemRefs.current[idx] = el;
                }}
                tabIndex={focused ? 0 : -1}
                onMouseEnter={() => setFocusIdx(idx)}
                onClick={() => navigate(panel.id, panel.href)}
                className={cn(
                  "flex w-full items-center gap-3 px-3 py-2.5 text-left text-sm transition-colors",
                  isActive
                    ? "bg-accent text-accent-foreground"
                    : "text-foreground hover:bg-secondary",
                  focused && !isActive && "bg-secondary/80"
                )}
              >
                <Icon className="h-4 w-4 shrink-0 opacity-70" />
                <span className="font-medium">{panel.label}</span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
