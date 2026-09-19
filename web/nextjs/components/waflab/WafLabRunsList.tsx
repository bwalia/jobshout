"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { FileDown } from "lucide-react";
import { apiErrorMessage } from "@/lib/api/client";
import {
  downloadWafLabReportPDF,
  listWafLabRuns,
  type WAFLabRun,
  type WAFLabRunStatus,
} from "@/lib/api/waf-lab";
import { reportVersionLabel } from "@/lib/security-report";

interface WafLabRunsListProps {
  onRunSelected?: (run: WAFLabRun) => void;
}

const statusLabel: Record<WAFLabRunStatus, string> = {
  queued: "Queued",
  running: "Running",
  completed: "Completed",
  failed: "Failed",
  cancelled: "Cancelled",
};

export function WafLabRunsList({ onRunSelected }: WafLabRunsListProps) {
  const [runs, setRuns] = useState<WAFLabRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      setLoading(true);
      setError("");
      try {
        const data = await listWafLabRuns(page, 10);
        if (cancelled) return;
        setRuns(data.data || []);
        setTotalPages(data.total_pages || 1);
      } catch (err) {
        if (!cancelled) setError(apiErrorMessage(err, "Failed to fetch runs"));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [page]);

  if (loading && runs.length === 0) {
    return <div className="py-8 text-center text-muted-foreground">Loading runs…</div>;
  }
  if (error) {
    return <div className="py-8 text-center text-destructive">{error}</div>;
  }
  if (runs.length === 0) {
    return <div className="py-8 text-center text-muted-foreground">No WAF lab runs yet.</div>;
  }

  return (
    <div className="space-y-4">
      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-full text-sm">
          <thead className="bg-muted text-left text-xs text-muted-foreground">
            <tr>
              <th className="px-4 py-2 font-medium">Secure host</th>
              <th className="px-4 py-2 font-medium">Mode</th>
              <th className="px-4 py-2 font-medium">Status</th>
              <th className="px-4 py-2 font-medium">Score</th>
              <th className="px-4 py-2 font-medium">Version</th>
              <th className="px-4 py-2 font-medium">Created</th>
              <th className="px-4 py-2 text-right font-medium">PDF</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((run) => (
              <tr
                key={run.id}
                className="cursor-pointer border-t border-border hover:bg-muted/40"
                onClick={() => onRunSelected?.(run)}
              >
                <td className="px-4 py-2 font-medium text-foreground">{run.secure_host}</td>
                <td className="px-4 py-2 text-muted-foreground">{run.mode}</td>
                <td className="px-4 py-2">{statusLabel[run.status] ?? run.status}</td>
                <td className="px-4 py-2 tabular-nums text-muted-foreground">
                  {run.score
                    ? `${run.score.secure_blocked}/${run.score.secure_expected}`
                    : "—"}
                </td>
                <td className="px-4 py-2 text-xs text-muted-foreground">
                  {reportVersionLabel(run)}
                </td>
                <td className="px-4 py-2 text-muted-foreground">
                  {new Date(run.created_at).toLocaleString()}
                </td>
                <td className="px-4 py-2 text-right">
                  {["completed", "failed", "cancelled"].includes(run.status) && (
                    <button
                      type="button"
                      className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
                      onClick={(e) => {
                        e.stopPropagation();
                        void (async () => {
                          try {
                            const blob = await downloadWafLabReportPDF(run.id);
                            const url = URL.createObjectURL(blob);
                            const a = document.createElement("a");
                            a.href = url;
                            a.download = `waf-lab-v${run.report_seq ?? "r"}.pdf`;
                            document.body.appendChild(a);
                            a.click();
                            a.remove();
                            URL.revokeObjectURL(url);
                          } catch (err) {
                            toast.error(apiErrorMessage(err, "PDF download failed"));
                          }
                        })();
                      }}
                    >
                      <FileDown className="h-3.5 w-3.5" />
                      PDF
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {totalPages > 1 && (
        <div className="flex items-center justify-between text-sm">
          <button
            type="button"
            disabled={page <= 1}
            className="text-primary disabled:opacity-40"
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            Previous
          </button>
          <span className="text-muted-foreground">
            Page {page} of {totalPages}
          </span>
          <button
            type="button"
            disabled={page >= totalPages}
            className="text-primary disabled:opacity-40"
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
          >
            Next
          </button>
        </div>
      )}
    </div>
  );
}
