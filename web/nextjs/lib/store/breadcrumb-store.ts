"use client";

import { useEffect } from "react";
import { create } from "zustand";

export interface Crumb {
  label: string;
  href?: string;
}

interface BreadcrumbState {
  /** Crumbs a page adds after its nav item (an agent's name, a tab…). */
  trail: Crumb[];
  setTrail: (trail: Crumb[]) => void;
}

export const useBreadcrumbStore = create<BreadcrumbState>()((set) => ({
  trail: [],
  setTrail: (trail) => set({ trail }),
}));

/**
 * Adds crumbs after the page's nav item, e.g. ["Article Writer", "Overview"]
 * under "All agents". Cleared when the page unmounts.
 */
export function useBreadcrumbs(trail: Crumb[]) {
  const setTrail = useBreadcrumbStore((s) => s.setTrail);
  const key = JSON.stringify(trail);
  useEffect(() => {
    setTrail(JSON.parse(key) as Crumb[]);
    return () => setTrail([]);
  }, [key, setTrail]);
}
