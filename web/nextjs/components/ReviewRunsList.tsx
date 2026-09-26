'use client';

import { useEffect, useState } from 'react';
import { SignalDot, type SignalStatus } from '@/components/ui/signal-dot';
import { apiClient, apiErrorMessage } from '@/lib/api/client';
import { PaginatedReviewRuns, ReviewRun, ReviewRunStatus } from '@/types/review';

const statusDot: Record<ReviewRunStatus, SignalStatus> = {
  queued: 'queued',
  running: 'live',
  completed: 'done',
  failed: 'error',
};

export function ReviewRunsList({ onRunSelected }: { onRunSelected?: (run: ReviewRun) => void }) {
  const [runs, setRuns] = useState<ReviewRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);

  useEffect(() => {
    void (async () => {
      setLoading(true);
      setError('');
      try {
        const { data } = await apiClient.get<PaginatedReviewRuns>('/review-runs', {
          params: { page, per_page: 10 },
        });
        setRuns(data.data || []);
        setTotalPages(data.total_pages || 1);
      } catch (err) {
        setError(apiErrorMessage(err, 'Failed to fetch runs'));
      } finally {
        setLoading(false);
      }
    })();
  }, [page]);

  if (loading && runs.length === 0) {
    return <div className="py-8 text-center text-muted-foreground">Loading runs…</div>;
  }
  if (error) {
    return <div className="py-8 text-center text-destructive">{error}</div>;
  }
  if (runs.length === 0) {
    return <div className="py-8 text-center text-muted-foreground">No reviews yet.</div>;
  }

  return (
    <div className="space-y-4">
      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-full text-sm">
          <thead className="bg-muted text-left text-xs text-muted-foreground">
            <tr>
              <th className="px-4 py-2 font-medium">PR</th>
              <th className="px-4 py-2 font-medium">Status</th>
              <th className="px-4 py-2 font-medium">Verdict</th>
              <th className="px-4 py-2 font-medium">Created</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {runs.map((run) => (
              <tr
                key={run.id}
                className="cursor-pointer hover:bg-muted/60"
                onClick={() => onRunSelected?.(run)}
              >
                <td className="px-4 py-2 font-mono text-foreground">
                  {run.repo}#{run.pr_number}
                  {run.dry_run ? (
                    <span className="ml-2 rounded bg-muted px-1.5 py-0.5 text-[10px] uppercase text-muted-foreground">
                      preview
                    </span>
                  ) : null}
                </td>
                <td className="px-4 py-2">
                  <span className="flex items-center gap-2">
                    <SignalDot status={statusDot[run.status] ?? 'idle'} size="md" />
                    {run.status}
                  </span>
                </td>
                <td className="px-4 py-2">{run.decision ?? '—'}</td>
                <td className="px-4 py-2 text-muted-foreground">
                  {new Date(run.created_at).toLocaleDateString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="flex items-center justify-between text-sm">
        <span className="text-muted-foreground">
          Page {page} of {totalPages}
        </span>
        <div className="flex gap-2">
          <button
            type="button"
            disabled={page === 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            className="rounded-md border border-input bg-background px-3 py-1 hover:bg-accent disabled:opacity-50"
          >
            Previous
          </button>
          <button
            type="button"
            disabled={page === totalPages}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            className="rounded-md border border-input bg-background px-3 py-1 hover:bg-accent disabled:opacity-50"
          >
            Next
          </button>
        </div>
      </div>
    </div>
  );
}
