"use client";

import { useEffect, useState } from "react";
import { MoonIcon, SunIcon } from "@/components/icons";

type Theme = "light" | "dark";

/**
 * Reads the theme the no-flash script in <head> already resolved, so the
 * button never disagrees with what is on screen.
 */
export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme | null>(null);

  useEffect(() => {
    setTheme(document.documentElement.classList.contains("dark") ? "dark" : "light");
  }, []);

  function toggle() {
    const next: Theme = theme === "dark" ? "light" : "dark";
    document.documentElement.classList.toggle("dark", next === "dark");
    try {
      localStorage.setItem("jobshout-theme", next);
    } catch {
      // Private mode or blocked storage — the toggle still works for this visit.
    }
    setTheme(next);
  }

  const isDark = theme === "dark";

  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={isDark ? "Switch to light theme" : "Switch to dark theme"}
      className="flex h-10 w-10 cursor-pointer items-center justify-center rounded-pill border border-line text-mute transition-colors duration-200 hover:border-edge hover:text-ink"
    >
      {/* Placeholder keeps layout stable before the effect resolves. */}
      {theme === null ? (
        <span className="h-[18px] w-[18px]" />
      ) : isDark ? (
        <SunIcon className="h-[18px] w-[18px]" />
      ) : (
        <MoonIcon className="h-[18px] w-[18px]" />
      )}
    </button>
  );
}
