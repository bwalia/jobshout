"use client";

import type { NavBadge } from "@/lib/nav";
import { useRunFailures } from "@/lib/hooks/useRunFailures";

/** Red failure counts for sidebar items, keyed by NavItem.badge. */
export function useNavBadges(): Partial<Record<NavBadge, number>> {
  const { failedTaskIds, failedAgentCount } = useRunFailures();
  return {
    "failed-tasks": failedTaskIds.size,
    "failed-agents": failedAgentCount,
  };
}
