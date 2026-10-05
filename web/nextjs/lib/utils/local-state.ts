"use client";

import { useCallback, useEffect, useState } from "react";

/**
 * Small per-browser preferences (a collapsed section, a chosen view). Storage
 * can be unavailable — private windows, blocked site data — so every access is
 * guarded and the default is used instead.
 */
export function readLocal<T>(key: string, fallback: T): T {
  try {
    const raw = window.localStorage.getItem(key);
    return raw === null ? fallback : (JSON.parse(raw) as T);
  } catch {
    return fallback;
  }
}

export function writeLocal<T>(key: string, value: T): void {
  try {
    window.localStorage.setItem(key, JSON.stringify(value));
  } catch {
    /* not persisted; the in-memory value still applies */
  }
}

/**
 * useState that survives a reload. The first render uses the fallback (so
 * server and client markup match), then the stored value is applied.
 */
export function useLocalState<T>(key: string, fallback: T): [T, (v: T | ((prev: T) => T)) => void] {
  const [value, setValue] = useState<T>(fallback);
  useEffect(() => {
    setValue(readLocal(key, fallback));
    // fallback is a constant default; re-reading on its identity would loop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  const set = useCallback(
    (v: T | ((prev: T) => T)) => {
      setValue((prev) => {
        const next = typeof v === "function" ? (v as (p: T) => T)(prev) : v;
        writeLocal(key, next);
        return next;
      });
    },
    [key]
  );
  return [value, set];
}
