import { Metadata } from 'next';
import { GitPullRequest } from 'lucide-react';
import { ReviewAgentClient } from '@/components/ReviewAgentClient';

export const metadata: Metadata = {
  title: 'PR Reviewer',
  description: 'AI review of GitHub pull requests via OpenCode and a local coder model',
};

export default function ReviewAgentPage() {
  return (
    <div className="space-y-6">
      <div>
        <div className="mb-1 flex items-center gap-3">
          <GitPullRequest className="h-7 w-7 text-primary" />
          <h1 className="text-2xl font-semibold tracking-tight">PR Reviewer</h1>
        </div>
        <p className="text-sm text-muted-foreground">
          Reviews explore the repository around the diff — not the patch in isolation — then
          return MERGE or FIX. Thinking runs on the workstation model; clones stay in this
          cluster.
        </p>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <ReviewAgentClient />
        </div>
        <div className="space-y-4">
          <div className="rounded-xl border border-border bg-card p-4 text-card-foreground">
            <h3 className="mb-3 text-sm font-semibold">What it checks</h3>
            <ul className="space-y-2 text-sm text-muted-foreground">
              <li>Does the change break behaviour that works today?</li>
              <li>Does it actually deliver the fix or feature the PR promises?</li>
              <li>Callers, tests, and nearby code — not the diff alone</li>
            </ul>
          </div>
          <div className="rounded-xl border border-border bg-accent p-4">
            <p className="text-xs text-accent-foreground">
              <strong>Preview</strong> is on by default so int cannot spam GitHub while we
              prove the sidecar. Uncheck it only when you want comments posted on the PR.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
