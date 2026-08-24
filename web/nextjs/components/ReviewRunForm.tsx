'use client';

import { useEffect, useState } from 'react';
import { apiClient, apiErrorMessage } from '@/lib/api/client';
import { CreateReviewRunRequest, ReviewRepos, ReviewRun } from '@/types/review';

const inputClass =
  'mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50';

export function ReviewRunForm({
  agentId,
  onRunCreated,
}: {
  agentId?: string;
  onRunCreated?: (run: ReviewRun) => void;
}) {
  const [repos, setRepos] = useState<ReviewRepos | null>(null);
  const [repo, setRepo] = useState('');
  const [prNumber, setPrNumber] = useState('');
  const [dryRun, setDryRun] = useState(true);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    void (async () => {
      try {
        const { data } = await apiClient.get<ReviewRepos>('/review-runs/repos');
        setRepos(data);
        if (data.allowed?.length && !repo) {
          setRepo(data.allowed[0]);
        }
      } catch (err) {
        setError(apiErrorMessage(err, 'Failed to load allowlisted repos'));
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    const n = parseInt(prNumber, 10);
    if (!repo || !n) {
      setError('Repo and a PR number are required');
      return;
    }
    setSubmitting(true);
    try {
      const payload: CreateReviewRunRequest = {
        repo,
        pr_number: n,
        dry_run: dryRun,
        ...(agentId ? { agent_id: agentId } : {}),
      };
      const { data: run } = await apiClient.post<ReviewRun>('/review-runs', payload);
      setPrNumber('');
      onRunCreated?.(run);
    } catch (err) {
      setError(apiErrorMessage(err, 'Failed to start review'));
    } finally {
      setSubmitting(false);
    }
  };

  if (repos && !repos.enabled) {
    return (
      <p className="text-sm text-muted-foreground">
        PR review is not enabled on this ring.
      </p>
    );
  }

  const allowed = repos?.allowed ?? [];

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div>
        <label htmlFor="repo" className="text-sm font-medium text-foreground">
          Repository
        </label>
        {allowed.length > 0 ? (
          <select
            id="repo"
            value={repo}
            onChange={(e) => setRepo(e.target.value)}
            disabled={submitting}
            className={inputClass}
          >
            {allowed.map((slug) => (
              <option key={slug} value={slug}>
                {slug}
              </option>
            ))}
          </select>
        ) : (
          <input
            id="repo"
            type="text"
            placeholder="owner/name"
            value={repo}
            onChange={(e) => setRepo(e.target.value)}
            disabled={submitting}
            className={inputClass}
          />
        )}
      </div>

      <div>
        <label htmlFor="pr" className="text-sm font-medium text-foreground">
          Pull request number
        </label>
        <input
          id="pr"
          type="number"
          min={1}
          placeholder="42"
          value={prNumber}
          onChange={(e) => setPrNumber(e.target.value)}
          disabled={submitting}
          className={inputClass}
        />
      </div>

      <label className="flex items-start gap-2 text-sm text-foreground">
        <input
          type="checkbox"
          checked={dryRun}
          onChange={(e) => setDryRun(e.target.checked)}
          disabled={submitting}
          className="mt-0.5"
        />
        <span>
          Preview only — do not post comments on GitHub
          <span className="mt-0.5 block text-xs text-muted-foreground">
            Uncheck to post the MERGE/FIX review on the pull request.
          </span>
        </span>
      </label>

      {error && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 p-2 text-sm text-destructive">
          {error}
        </div>
      )}

      <button
        type="submit"
        disabled={submitting || !repo || !prNumber}
        className="w-full rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {submitting ? 'Queuing…' : dryRun ? 'Preview review' : 'Review and post to GitHub'}
      </button>
    </form>
  );
}
