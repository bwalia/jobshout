'use client';

import { useEffect, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { apiClient } from '@/lib/api/client';
import { ReviewRun } from '@/types/review';
import { ReviewRunForm } from './ReviewRunForm';
import { ReviewRunResult } from './ReviewRunResult';
import { ReviewRunsList } from './ReviewRunsList';

type Tab = 'run' | 'history';

interface AgentSummary {
  id: string;
  metadata?: { builtin?: string };
}

export function ReviewAgentClient() {
  const searchParams = useSearchParams();
  const runParam = searchParams.get('run');

  const [agentId, setAgentId] = useState('');
  const [loadError, setLoadError] = useState('');
  const [createdRun, setCreatedRun] = useState<ReviewRun | null>(null);
  const [historyRun, setHistoryRun] = useState<ReviewRun | null>(null);
  const [tab, setTab] = useState<Tab>('run');

  useEffect(() => {
    void (async () => {
      try {
        const { data } = await apiClient.get<{ data: AgentSummary[] }>('/agents', {
          params: { per_page: 100 },
        });
        const agents = Array.isArray(data.data) ? data.data : [];
        const reviewer = agents.find((a) => a.metadata?.builtin === 'pr_reviewer');
        if (reviewer) {
          setAgentId(reviewer.id);
        } else {
          setLoadError('No PR Reviewer agent found for this organization.');
        }
      } catch {
        setLoadError('Failed to load the PR Reviewer agent.');
      }
    })();
  }, []);

  // Deep-link from Task Manager Create & run: ?agent=review&run=<id>
  useEffect(() => {
    if (!runParam) return;
    let cancelled = false;
    void (async () => {
      try {
        const { data } = await apiClient.get<ReviewRun>(`/review-runs/${runParam}`);
        if (cancelled) return;
        setCreatedRun(data);
        setTab('run');
      } catch {
        // Leave the form; run may still appear under History.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [runParam]);

  if (loadError) {
    return (
      <div className="rounded-lg border border-destructive/40 bg-destructive/10 p-6 text-center text-sm text-destructive">
        {loadError}
      </div>
    );
  }

  if (!agentId) {
    return (
      <div className="rounded-lg border border-border bg-card p-6 text-center text-muted-foreground">
        Loading agent configuration…
      </div>
    );
  }

  const tabClass = (t: Tab) =>
    `flex-1 rounded-md px-3 py-1.5 text-sm font-medium ${
      tab === t ? 'bg-card text-card-foreground shadow' : 'text-muted-foreground hover:text-foreground'
    }`;

  return (
    <div className="space-y-4">
      <div className="flex gap-1 rounded-lg bg-muted p-1">
        <button type="button" className={tabClass('run')} onClick={() => setTab('run')}>
          New review
        </button>
        <button type="button" className={tabClass('history')} onClick={() => setTab('history')}>
          History
        </button>
      </div>

      {tab === 'run' &&
        (createdRun ? (
          <div className="rounded-lg border border-border bg-card p-4 text-card-foreground">
            <button
              type="button"
              onClick={() => setCreatedRun(null)}
              className="mb-4 text-sm font-medium text-primary hover:underline"
            >
              ← Back to form
            </button>
            <ReviewRunResult run={createdRun} />
          </div>
        ) : (
          <div className="rounded-lg border border-border bg-card p-4 text-card-foreground">
            <h2 className="font-semibold">Review a pull request</h2>
            <p className="mb-4 text-sm text-muted-foreground">
              The reviewer explores the repo around the diff, then returns MERGE or FIX.
            </p>
            <ReviewRunForm agentId={agentId} onRunCreated={setCreatedRun} />
          </div>
        ))}

      {tab === 'history' && (
        <div className="rounded-lg border border-border bg-card p-4 text-card-foreground">
          {historyRun ? (
            <>
              <button
                type="button"
                onClick={() => setHistoryRun(null)}
                className="mb-4 text-sm font-medium text-primary hover:underline"
              >
                ← Back to history
              </button>
              <ReviewRunResult run={historyRun} />
            </>
          ) : (
            <>
              <h2 className="font-semibold">Review history</h2>
              <p className="mb-4 text-sm text-muted-foreground">Previous and in-flight reviews for this org.</p>
              <ReviewRunsList onRunSelected={setHistoryRun} />
            </>
          )}
        </div>
      )}
    </div>
  );
}
