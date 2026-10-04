"use client";

import type { NavBadge } from "@/lib/nav";

/** Red failure counts for sidebar items, keyed by NavItem.badge. */
export function useNavBadges(): Partial<Record<NavBadge, number>> {
  return {};
}
