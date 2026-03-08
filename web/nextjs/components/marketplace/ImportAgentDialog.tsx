"use client";

import { useEffect, useRef } from "react";

interface ImportAgentDialogProps {
  agentName: string;
  agentModelProvider: string;
  agentModelName: string;
  agentCategory: string;
  agentDescription: string;
  /** Whether the import mutation is currently in-flight */
  isImporting?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ImportAgentDialog({
  agentName,
  agentModelProvider,
  agentModelName,
  agentCategory,
  agentDescription,
  isImporting = false,
  onConfirm,
  onCancel,
}: ImportAgentDialogProps) {
  const dialogRef = useRef<HTMLDivElement>(null);

  // Close dialog on Escape key (disabled while import is in-flight to prevent
  // accidentally closing during a pending network request)
  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape" && !isImporting) {
        onCancel();
      }
    }
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onCancel, isImporting]);

  // Close dialog when clicking the backdrop (outside the dialog box)
  function handleBackdropClick(event: React.MouseEvent<HTMLDivElement>): void {
    if (event.target === event.currentTarget && !isImporting) {
      onCancel();
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="import-dialog-title"
    >
      <div
        ref={dialogRef}
        className="w-full max-w-md rounded-xl border border-border bg-card p-6 shadow-xl"
      >
        {/* Dialog header */}
        <h2
          id="import-dialog-title"
          className="text-lg font-semibold leading-tight"
        >
          Import {agentName} to your team?
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          This agent will be added to your organisation and ready to assign to
          projects immediately.
        </p>

        {/* Agent preview card */}
        <div className="mt-5 rounded-lg border border-border bg-background p-4">
          <div className="flex items-start gap-3">
            {/* Avatar placeholder */}
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-base font-bold text-primary">
              {agentName.charAt(0)}
            </div>

            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{agentName}</span>
                <span className="inline-flex items-center rounded-full bg-secondary px-2 py-0.5 text-xs font-medium text-secondary-foreground">
                  {agentCategory}
                </span>
              </div>
              <p className="mt-0.5 text-sm text-muted-foreground">
                {agentModelProvider} &middot; {agentModelName}
              </p>
              <p className="mt-2 text-sm text-muted-foreground line-clamp-3">
                {agentDescription}
              </p>
            </div>
          </div>
        </div>

        {/* Action buttons */}
        <div className="mt-6 flex justify-end gap-3">
          <button
            type="button"
            onClick={onCancel}
            disabled={isImporting}
            className="inline-flex h-9 items-center justify-center rounded-md border border-border bg-background px-4 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={isImporting}
            className="inline-flex h-9 items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
          >
            {isImporting ? "Importing…" : "Confirm Import"}
          </button>
        </div>
      </div>
    </div>
  );
}
