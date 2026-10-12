"use client";

import { useFormState, useFormStatus } from "react-dom";
import { saveHeadlineAction } from "@/app/members/actions";
import { Field, Input, buttonClass } from "@/components/ui";
import { EMPTY_FORM_STATE } from "@/lib/form-state";
import { HEADLINE_MAX } from "@/lib/members";

function Save() {
  const { pending } = useFormStatus();
  return (
    <button type="submit" disabled={pending} className={buttonClass("secondary", "md")}>
      {pending ? "Saving…" : "Save"}
    </button>
  );
}

/** Your own one-line headline, edited on your profile. */
export function HeadlineForm({ headline }: { headline: string }) {
  const [state, action] = useFormState(saveHeadlineAction, EMPTY_FORM_STATE);
  const error = state.fieldErrors?.headline ?? (!state.ok ? state.message : undefined);
  return (
    <form action={action} className="surface-card mt-6 p-5">
      <Field
        label="Your headline"
        htmlFor="headline"
        hint="One line on what you build or write about. Shown on your profile and in the members directory."
        error={error}
      >
        <div className="flex flex-col gap-2.5 sm:flex-row">
          <Input
            id="headline"
            name="headline"
            defaultValue={headline}
            maxLength={HEADLINE_MAX}
            placeholder="e.g. Building agents for conveyancing teams"
            aria-describedby="headline-hint"
          />
          <Save />
        </div>
      </Field>
      {state.ok && state.message ? (
        <p role="status" className="mt-2 text-xs font-medium text-good">
          {state.message}
        </p>
      ) : null}
    </form>
  );
}
