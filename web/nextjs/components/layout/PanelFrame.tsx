"use client";

import { useEffect, useState } from "react";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils/cn";
import { panelFromPath, panelIndex, type PanelId } from "@/lib/panels";

const FROM_KEY = "jobshout-panel-from";
const TO_KEY = "jobshout-panel-to";

/**
 * Reads (without consuming) the transition the sidebar recorded just before
 * navigating. Read-only so it is safe to call from render, including React's
 * strict-mode double invocations; the values are cleared in an effect after
 * the navigation commits.
 */
function peekDir(current: PanelId): "up" | "down" | null {
  let from: PanelId | null = null;
  let to: PanelId | null = null;
  try {
    from = sessionStorage.getItem(FROM_KEY) as PanelId | null;
    to = sessionStorage.getItem(TO_KEY) as PanelId | null;
  } catch {
    return null;
  }

  if (!from || !to || from === to || to === "chat" || current === "chat") {
    return null;
  }
  const fromIdx = panelIndex(from);
  const toIdx = panelIndex(to);
  if (fromIdx < 0 || toIdx < 0) return null;
  return toIdx > fromIdx ? "down" : "up";
}

/**
 * Slides the panel in from top/bottom based on menu order relative to the
 * previous panel. No animation on initial load or plain chat navigation.
 *
 * The direction is derived synchronously during the render that swaps the
 * page in, so the animation class is present on the new page's very first
 * paint — the page mounts exactly once per navigation. (The previous
 * implementation computed it in a post-paint effect and bumped a wrapper
 * key, which painted the page and then unmounted and remounted the whole
 * subtree, resetting all of its local state.)
 */
export function PanelFrame({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const current = panelFromPath(pathname);
  // Initial mount is the initial page load (the layout persists across client
  // navigations), which never animates — this also keeps server and client
  // markup identical for hydration.
  const [nav, setNav] = useState<{
    pathname: string;
    dir: "up" | "down" | null;
  }>(() => ({ pathname, dir: null }));

  // Derived-state-from-props pattern: re-derive the direction when the route
  // changes, during render, so the first paint already animates.
  if (nav.pathname !== pathname) {
    setNav({ pathname, dir: peekDir(current) });
  }
  const dir = nav.pathname === pathname ? nav.dir : null;

  // Consume the recorded transition only after the navigation has committed.
  useEffect(() => {
    try {
      sessionStorage.removeItem(FROM_KEY);
      sessionStorage.removeItem(TO_KEY);
    } catch {
      /* ignore */
    }
  }, [pathname]);

  // Keyed by pathname: the wrapper (and animation) restarts exactly when the
  // page underneath changes anyway, never remounting an already-painted page.
  return (
    <div
      key={pathname}
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
