"use client";

import { useId } from "react";
import type { ConfirmRequest } from "@/lib/types/chat";

export function ConfirmationCard({
  confirmation,
  onApprove,
  onCancel,
  busy,
  live,
  answeredAs,
}: {
  confirmation: ConfirmRequest;
  onApprove: () => void;
  onCancel: () => void;
  busy?: boolean;
  live?: boolean;
  answeredAs?: string;
}) {
  const titleId = useId();
  const expired =
    Boolean(confirmation.expires_at) &&
    Date.parse(confirmation.expires_at as string) < Date.now();
  const active = Boolean(live) && !expired;
  const verdict =
    answeredAs === "yes"
      ? "Approved"
      : answeredAs === "cancel"
        ? "Cancelled"
        : expired
          ? "Expired"
          : "Answered";

  return (
    <div
      className="mt-3 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3"
      role="group"
      aria-labelledby={titleId}
    >
      <p id={titleId} className="text-sm font-medium text-foreground">
        {confirmation.summary || "Please confirm"}
      </p>
      <p className="mt-1 text-sm text-muted-foreground">{confirmation.effect}</p>
      {active ? (
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
      ) : (
        <p className="mt-2 text-xs font-medium text-muted-foreground">{verdict}</p>
      )}
    </div>
  );
}
