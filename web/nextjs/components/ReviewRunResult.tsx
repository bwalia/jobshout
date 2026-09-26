'use client';

import { useEffect, useState } from 'react';
import { AlertCircle, CheckCircle2, GitPullRequest } from 'lucide-react';
import { SignalDot, type SignalStatus } from '@/components/ui/signal-dot';
import { apiClient } from '@/lib/api/client';
import { ReviewFinding, ReviewRun, ReviewRunStatus } from '@/types/review';

const TERMINAL: ReviewRunStatus[] = ['completed', 'failed'];

const statusDot: Record<ReviewRunStatus, SignalStatus> = {
  queued: 'queued',
  running: 'live',
  completed: 'done',
  failed: 'error',
};

const statusLabel: Record<ReviewRunStatus, string> = {
  queued: 'Queued',
  running: 'Reviewing…',
  completed: 'Completed',
  failed: 'Failed',
};

export function ReviewRunResult({ run }: { run: ReviewRun }) {
  const [live, setLive] = useState<ReviewRun>(run);

  useEffect(() => {
    setLive(run);
  }, [run]);

  useEffect(() => {
    let stop = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const tick = async () => {
      try {
        const { data } = await apiClient.get<ReviewRun>(`/review-runs/${run.id}`);
        if (stop) return;
        setLive(data);
        if (!TERMINAL.includes(data.status)) {
          timer = setTimeout(tick, 4000);
        }
      } catch {
        if (!stop) timer = setTimeout(tick, 8000);
      }
    };
    void tick();
    return () => {
      stop = true;
      if (timer) clearTimeout(timer);
    };
  }, [run.id]);

  const result = live.result;
  const blocking = result?.blocking ?? [];
  const rest = [
    ...(result?.inline ?? []),
    ...(result?.orphaned ?? []),
  ].filter((f) => !blocking.some((b) => b.file === f.file && b.line === f.line && b.title === f.title));

  return (
    <div className="space-y-4">
      <header className="rounded-lg border border-border bg-background p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <GitPullRequest className="h-5 w-5" />
            </span>
            <div>
              <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                Pull request review
              </p>
              <h3 className="font-semibold text-foreground">
                {live.repo}#{live.pr_number}
              </h3>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {live.dry_run ? 'Preview only — nothing posted to GitHub' : 'Will post (or posted) on GitHub'}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <SignalDot status={statusDot[live.status] ?? 'idle'} size="lg" />
            <span className="text-sm font-medium">{statusLabel[live.status] ?? live.status}</span>
          </div>
        </div>

        {live.status === 'failed' && live.error_message && (
          <div className="mt-3 flex gap-2 rounded-lg border border-destructive/40 bg-destructive/10 p-3">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
            <p className="text-sm text-destructive">{live.error_message}</p>
          </div>
        )}

        {live.decision && (
          <p className="mt-4 text-lg font-semibold">
            {live.decision === 'FIX' ? 'FIX before merging' : 'MERGE'}
          </p>
        )}
        {live.verdict && <p className="mt-1 text-sm text-foreground">{live.verdict}</p>}
        {live.summary && <p className="mt-2 text-sm text-muted-foreground">{live.summary}</p>}
        {live.github_url && (
          <a
            href={live.github_url}
            target="_blank"
            rel="noreferrer"
            className="mt-3 inline-block text-sm font-medium text-primary hover:underline"
          >
            Open GitHub review
          </a>
        )}
      </header>

      {live.stage_log && live.stage_log.length > 0 && (
        <section className="rounded-lg border border-border bg-background p-4">
          <h3 className="mb-2 text-sm font-semibold">Progress</h3>
          <ol className="space-y-1 font-mono text-xs text-muted-foreground">
            {live.stage_log.map((line, i) => (
              <li key={`${i}-${line}`}>{line}</li>
            ))}
          </ol>
        </section>
      )}

      {blocking.length > 0 && (
        <FindingList title="Fix these first" findings={blocking} />
      )}
      {rest.length > 0 && (
        <FindingList title="Also worth a look" findings={rest} />
      )}
      {live.status === 'completed' && blocking.length === 0 && rest.length === 0 && (
        <div className="flex gap-2 rounded-lg border border-border bg-muted/40 p-4 text-sm text-muted-foreground">
          <CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-600" />
          No findings reported.
        </div>
      )}
    </div>
  );
}

function FindingList({ title, findings }: { title: string; findings: ReviewFinding[] }) {
  return (
    <section className="rounded-lg border border-border bg-background p-4">
      <h3 className="mb-3 text-sm font-semibold">{title}</h3>
      <ul className="space-y-3">
        {findings.map((f, i) => (
          <li key={`${f.file}:${f.line}:${f.title}:${i}`} className="text-sm">
            <p className="font-medium text-foreground">{f.title}</p>
            <p className="font-mono text-xs text-muted-foreground">
              {f.file}
              {f.line ? `:${f.line}` : ''} · {f.severity} · {f.category}
            </p>
            <p className="mt-1 text-muted-foreground">{f.reason}</p>
            {f.suggestion && (
              <p className="mt-1 text-foreground">
                <span className="font-medium">Fix:</span> {f.suggestion}
              </p>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
