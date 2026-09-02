"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { ShieldAlert, ArrowRight } from "lucide-react";
import { apiClient } from "@/lib/api/client";
import { cn } from "@/lib/utils/cn";
import { SignalDot, type SignalStatus } from "@/components/ui/signal-dot";
import type {
  PaginatedPentestRuns,
  PentestRun,
  PentestRunStatus,
} from "@/types/pentest";

const TERMINAL: PentestRunStatus[] = [
  "completed",
  "failed",
  "cancelled",
  "budget_exceeded",
];

const STATUS_DOT: Record<PentestRunStatus, SignalStatus> = {
  queued: "queued",
  starting: "live",
  running: "live",
  completed: "done",
  failed: "error",
  budget_exceeded: "error",
  cancelled: "idle",
};

const STATUS_LABEL: Record<PentestRunStatus, string> = {
  queued: "Queued",
  starting: "Starting",
  running: "Scanning",
  completed: "Completed",
  failed: "Failed",
  budget_exceeded: "Budget exceeded",
  cancelled: "Cancelled",
};

async function fetchRecentRuns(): Promise<PentestRun[]> {
  const { data } = await apiClient.get<PaginatedPentestRuns>("/pentest-runs", {
    params: { per_page: 8 },
  });
  return Array.isArray(data.data) ? data.data : [];
}

function hostOf(target: string): string {
  try {
    return new URL(target).host;
  } catch {
    return target;
  }
}

function relativeTime(iso?: string): string {
  if (!iso) return "";
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(iso).toLocaleDateString();
}

/**
 * Dashboard surface for the Security Tester: the last scan's status, the current
 * severity tallies, whether a scan is live now, and how many high/critical
 * findings are outstanding — so the agent is visible without opening its panel.
 */
export function SecurityTesterCard() {
  const { data: runs = [], isLoading } = useQuery({
    queryKey: ["pentest", "dashboard-recent"],
    queryFn: fetchRecentRuns,
    refetchInterval: (query) => {
      const rs = query.state.data as PentestRun[] | undefined;
      return rs?.some((r) => !TERMINAL.includes(r.status)) ? 5000 : false;
    },
  });

  const latest = runs[0];
  const scanning = runs.some((r) => !TERMINAL.includes(r.status));
  // Severity picture comes from the most recent run that actually reported, not a
  // still-queued one that has no tallies yet.
  const reported = runs.find((r) => TERMINAL.includes(r.status)) ?? latest;
  const highCritical = runs.reduce((sum, r) => sum + (r.high_severity ?? 0), 0);

  return (
    <section className="flex flex-col rounded-xl border border-border bg-card shadow-card">
      <header className="flex items-center justify-between gap-3 border-b border-border px-5 py-3.5">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <ShieldAlert className="h-4 w-4 text-red-600 dark:text-red-400" />
          Security Tester
        </h2>
        <Link
          href="/panel/security-tester"
          className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
        >
          Open <ArrowRight className="h-3 w-3" />
        </Link>
      </header>
      <div className="min-h-0 flex-1 p-5">
        {isLoading ? (
          <div className="h-24 animate-pulse rounded-lg bg-secondary" />
        ) : !latest ? (
          <div className="flex h-24 flex-col items-center justify-center gap-2 text-center">
            <p className="text-sm text-muted-foreground">No scans yet.</p>
            <Link
              href="/panel/security-tester"
              className="text-sm font-medium text-primary hover:underline"
            >
              Run your first security scan
            </Link>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{hostOf(latest.target)}</p>
                <p className="truncate font-mono text-xs text-muted-foreground">
                  {latest.target}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1.5">
                <SignalDot status={STATUS_DOT[latest.status] ?? "idle"} size="md" />
                <span className="text-xs font-medium">
                  {scanning ? "Scanning…" : STATUS_LABEL[latest.status] ?? latest.status}
                </span>
              </div>
            </div>

            <div className="grid grid-cols-3 gap-2">
              <SeverityTile
                label="Critical + High"
                value={reported?.high_severity ?? 0}
                tone="red"
              />
              <SeverityTile label="Medium" value={reported?.medium_severity ?? 0} tone="amber" />
              <SeverityTile label="Low" value={reported?.low_severity ?? 0} tone="blue" />
            </div>

            <p className="text-xs text-muted-foreground">
              {highCritical > 0 ? (
                <span className="font-medium text-red-600 dark:text-red-400">
                  {highCritical} high/critical
                </span>
              ) : (
                <span>No high/critical</span>
              )}{" "}
              across the last {runs.length} scan{runs.length === 1 ? "" : "s"} · updated{" "}
              {relativeTime(latest.updated_at || latest.created_at)}
            </p>
          </div>
        )}
      </div>
    </section>
  );
}

function SeverityTile({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: "red" | "amber" | "blue";
}) {
  const color =
    tone === "red"
      ? "text-red-600 dark:text-red-400"
      : tone === "amber"
        ? "text-amber-600 dark:text-amber-400"
        : "text-blue-600 dark:text-blue-400";
  return (
    <div className="rounded-lg border border-border bg-background px-3 py-2">
      <p className="text-[11px] text-muted-foreground">{label}</p>
      <p className={cn("text-xl font-semibold", color)}>{value}</p>
    </div>
  );
}
