"use client";

import { useState } from "react";
import { useFormState, useFormStatus } from "react-dom";
import { EMPTY_FORM_STATE } from "@/app/actions";
import { moderateAction } from "@/app/insights/actions";
import { Button, Textarea } from "@/components/ui";
import type { InsightStatus } from "@/lib/insights";

function ActionButton({
  value,
  label,
  variant = "secondary",
}: {
  value: string;
  label: string;
  variant?: "primary" | "secondary" | "ghost";
}) {
  const { pending } = useFormStatus();
  return (
    <Button type="submit" name="action" value={value} size="sm" variant={variant} disabled={pending}>
      {label}
    </Button>
  );
}

export function ReviewActions({
  id,
  status,
  featured,
}: {
  id: string;
  status: InsightStatus;
  featured: boolean;
}) {
  const [state, action] = useFormState(moderateAction, EMPTY_FORM_STATE);
  const [rejecting, setRejecting] = useState(false);

  if (state.ok) {
    return (
      <p role="status" className="text-sm font-medium text-good">
        {state.message}
      </p>
    );
  }

  return (
    <form action={action} className="space-y-3">
      <input type="hidden" name="id" value={id} />
      {rejecting ? (
        <div>
          <label htmlFor={`note-${id}`} className="block text-sm font-semibold text-ink">
            What should the author change?
          </label>
          <Textarea
            id={`note-${id}`}
            name="note"
            rows={3}
            className="mt-2"
            placeholder="Add a source for the hiring claim in paragraph two."
            aria-invalid={Boolean(state.fieldErrors?.note)}
          />
        </div>
      ) : null}
      <div className="flex flex-wrap gap-2">
        {status === "pending_review" ? (
          rejecting ? (
            <>
              <ActionButton value="reject" label="Send back" variant="primary" />
              <Button type="button" size="sm" variant="ghost" onClick={() => setRejecting(false)}>
                Cancel
              </Button>
            </>
          ) : (
            <>
              <ActionButton value="approve" label="Approve & publish" variant="primary" />
              <Button type="button" size="sm" variant="secondary" onClick={() => setRejecting(true)}>
                Request changes
              </Button>
            </>
          )
        ) : null}
        {status === "published" ? (
          <>
            <ActionButton
              value={featured ? "unfeature" : "feature"}
              label={featured ? "Unfeature" : "Feature on front page"}
            />
            <ActionButton value="archive" label="Archive" variant="ghost" />
          </>
        ) : null}
        {status === "archived" || status === "rejected" ? (
          <ActionButton value="approve" label="Publish" />
        ) : null}
      </div>
      {state.message && !state.ok ? (
        <p role="alert" className="text-xs font-medium text-shout">
          {state.message}
        </p>
      ) : null}
    </form>
  );
}
