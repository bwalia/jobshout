"use client";

import { useEffect, useRef } from "react";
import { useFormState, useFormStatus } from "react-dom";
import { sendMessageAction } from "@/app/messages/actions";
import { ErrorNotice, buttonClass } from "@/components/ui";
import { EMPTY_FORM_STATE } from "@/lib/form-state";

function Send() {
  const { pending } = useFormStatus();
  return (
    <button type="submit" disabled={pending} className={buttonClass("primary", "md")}>
      {pending ? "Sending…" : "Send"}
    </button>
  );
}

/** Reply box. Clears itself after a successful send. */
export function MessageComposer({ conversationId }: { conversationId: string }) {
  const [state, action] = useFormState(sendMessageAction.bind(null, conversationId), EMPTY_FORM_STATE);
  const form = useRef<HTMLFormElement>(null);
  useEffect(() => {
    if (state.ok) form.current?.reset();
  }, [state]);

  return (
    <form ref={form} action={action} className="space-y-3">
      {state.message && !state.ok ? <ErrorNotice title={state.message} /> : null}
      <label htmlFor="message-body" className="sr-only">
        Message
      </label>
      <textarea
        id="message-body"
        name="body"
        rows={3}
        maxLength={8000}
        required
        placeholder="Write a message…"
        className="w-full rounded-xl border border-line bg-raised px-4 py-3 text-sm text-ink placeholder:text-mute focus:border-ink focus:outline-none"
        aria-invalid={Boolean(state.fieldErrors?.body)}
      />
      {state.fieldErrors?.body ? <p className="text-sm text-shout">{state.fieldErrors.body}</p> : null}
      <div className="flex justify-end">
        <Send />
      </div>
    </form>
  );
}
