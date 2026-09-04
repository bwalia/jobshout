"use client";

import { useId } from "react";
import { Ban, Check, Clock, ShieldAlert } from "lucide-react";
import type { ConfirmRequest } from "@/lib/types/chat";

/**
 * An action the agent will not take until the user says so.
 *
 * The tint is signal-warn rather than a raw amber: amber is this product's
 * primary, so an amber card read as ordinary brand chrome instead of "this is
 * waiting on a decision", and it left the Approve button with nothing to
 * stand out against.
 */
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
      ? { label: "Approved", icon: Check, cls: "text-signal-live" }
      : answeredAs === "cancel"
        ? { label: "Cancelled", icon: Ban, cls: "text-muted-foreground" }
        : expired
          ? { label: "Expired", icon: Clock, cls: "text-muted-foreground" }
          : { label: "Answered", icon: Check, cls: "text-muted-foreground" };
  const VerdictIcon = verdict.icon;

  return (
    <div
      className="mt-2 overflow-hidden rounded-lg border border-signal-warn/40 bg-signal-warn/5"
      role="group"
      aria-labelledby={titleId}
    >
      <div className="flex min-w-0 items-start gap-2 px-3 py-2.5">
        <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-signal-warn" />
        <div className="min-w-0">
          <p
            id={titleId}
            className="min-w-0 break-words text-[15px] font-semibold leading-6 text-foreground"
          >
            {confirmation.summary || "Please confirm"}
          </p>
          <p className="mt-0.5 min-w-0 break-words text-sm leading-6 text-muted-foreground">
            {confirmation.effect}
          </p>
        </div>
      </div>

      {active ? (
        <div className="flex gap-2 border-t border-signal-warn/25 px-3 py-2">
          <button
            type="button"
            disabled={busy}
            onClick={onApprove}
            className="inline-flex items-center justify-center rounded-md bg-primary px-3 py-1.5 text-sm font-semibold text-primary-foreground transition-opacity hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:opacity-50 max-sm:min-h-[44px] max-sm:flex-1"
          >
            Approve
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={onCancel}
            className="inline-flex items-center justify-center rounded-md border border-border bg-card px-3 py-1.5 text-sm font-medium text-foreground transition-colors hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:opacity-50 max-sm:min-h-[44px] max-sm:flex-1"
          >
            Cancel
          </button>
        </div>
      ) : (
        <p className="flex items-center gap-1.5 border-t border-signal-warn/25 px-3 py-2 text-xs font-medium text-muted-foreground">
          <VerdictIcon className={"h-3.5 w-3.5 shrink-0 " + verdict.cls} />
          {verdict.label}
        </p>
      )}
    </div>
  );
}
