"use client";

import { useEffect, useState } from "react";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils/cn";
import { panelFromPath, panelIndex, type PanelId } from "@/lib/panels";

/**
 * Slides the panel in from top/bottom based on menu order relative to the
 * previous panel. No animation on initial load or plain chat navigation.
 */
export function PanelFrame({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const current = panelFromPath(pathname);
  const [dir, setDir] = useState<"up" | "down" | null>(null);
  const [animKey, setAnimKey] = useState(0);

  useEffect(() => {
    let from: PanelId | null = null;
    let to: PanelId | null = null;
    try {
      from = sessionStorage.getItem("jobshout-panel-from") as PanelId | null;
      to = sessionStorage.getItem("jobshout-panel-to") as PanelId | null;
      sessionStorage.removeItem("jobshout-panel-from");
      sessionStorage.removeItem("jobshout-panel-to");
    } catch {
      /* ignore */
    }

    if (!from || !to || from === to || to === "chat" || current === "chat") {
      setDir(null);
      return;
    }

    const fromIdx = panelIndex(from);
    const toIdx = panelIndex(to);
    if (fromIdx < 0 || toIdx < 0) {
      setDir(null);
      return;
    }
    setDir(toIdx > fromIdx ? "down" : "up");
    setAnimKey((k) => k + 1);
  }, [pathname, current]);

  return (
    <div
      key={animKey}
      className={cn(
        "h-full min-h-0",
        dir === "down" && "animate-panel-from-bottom",
        dir === "up" && "animate-panel-from-top"
      )}
    >
      {children}
    </div>
  );
}
