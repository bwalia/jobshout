"use client";

import { useEffect, useRef } from "react";
import { useFormState, useFormStatus } from "react-dom";
import { createPostAction } from "@/app/groups/actions";
import { Button, ErrorNotice, Field, Input, Textarea } from "@/components/ui";
import { EMPTY_FORM_STATE } from "@/lib/form-state";

function Submit() {
  const { pending } = useFormStatus();
  return (
    <Button type="submit" size="md" disabled={pending}>
      {pending ? "Posting…" : "Post"}
    </Button>
  );
}

export function PostComposer({ slug }: { slug: string }) {
  const bound = createPostAction.bind(null, slug);
  const [state, action] = useFormState(bound, EMPTY_FORM_STATE);
  const formRef = useRef<HTMLFormElement>(null);

  useEffect(() => {
    if (state.ok) formRef.current?.reset();
  }, [state]);

  return (
    <form
      ref={formRef}
      action={action}
      className="space-y-3 rounded-card border border-line bg-surface p-5"
    >
      <h2 className="font-display text-lg font-semibold text-ink">Start a discussion</h2>
      {state.message && !state.ok ? <ErrorNotice title={state.message} /> : null}
      <Field label="Title (optional)" htmlFor="title">
        <Input id="title" name="title" maxLength={200} placeholder="What's on your mind?" />
      </Field>
      <Field label="Post" htmlFor="body" error={state.fieldErrors?.body} required>
        <Textarea id="body" name="body" required rows={4} maxLength={20000} />
      </Field>
      <input type="hidden" name="kind" value="discussion" />
      <Submit />
    </form>
  );
}
