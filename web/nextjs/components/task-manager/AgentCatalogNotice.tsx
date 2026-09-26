"use client";

/**
 * Shown while GET /agent-schemas is in flight or has failed for a specialist.
 * Callers must not fall back to the generic task form in this state.
 */
export function AgentCatalogNotice({
  missing,
  isError,
  onRetry,
}: {
  missing: boolean;
  isError: boolean;
  onRetry: () => void;
}) {
  if (!missing) return null;
  if (isError) {
    return (
      <div
        className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive"
        role="alert"
      >
        <p>Could not load this agent&apos;s form.</p>
        <button
          type="button"
          onClick={() => void onRetry()}
          className="mt-2 text-sm font-medium underline underline-offset-2 hover:no-underline"
        >
          Retry
        </button>
      </div>
    );
  }
  return (
    <p className="text-sm text-muted-foreground">Loading agent form…</p>
  );
}
