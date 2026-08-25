"use client";

import type { ConfirmRequest } from "@/lib/types/chat";

export function ConfirmationCard({
  confirmation,
  onApprove,
  onCancel,
  busy,
}: {
  confirmation: ConfirmRequest;
  onApprove: () => void;
  onCancel: () => void;
  busy?: boolean;
}) {
  return (
    <div
      className="mt-3 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3"
      role="alertdialog"
      aria-labelledby="chat-confirm-title"
    >
      <p id="chat-confirm-title" className="text-sm font-medium text-foreground">
        {confirmation.summary || "Please confirm"}
      </p>
      <p className="mt-1 text-sm text-muted-foreground">{confirmation.effect}</p>
      <div className="mt-3 flex gap-2">
        <button
          type="button"
          disabled={busy}
          onClick={onApprove}
          className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
        >
          Approve
        </button>
        <button
          type="button"
          disabled={busy}
          onClick={onCancel}
          className="rounded-md border border-border px-3 py-1.5 text-sm hover:bg-secondary disabled:opacity-50"
        >
          Cancel
        </button>
      </div>
    </div>
  );
}
