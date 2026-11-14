"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useFormState, useFormStatus } from "react-dom";
import { createGroupAction } from "@/app/groups/actions";
import { Button, ErrorNotice, Field, Input, Textarea } from "@/components/ui";
import { EMPTY_FORM_STATE } from "@/lib/form-state";
import { slugify } from "@/lib/groups";

function Submit() {
  const { pending } = useFormStatus();
  return (
    <Button type="submit" size="md" disabled={pending}>
      {pending ? "Creating…" : "Create group"}
    </Button>
  );
}

export function CreateGroupForm() {
  const router = useRouter();
  const [state, action] = useFormState(createGroupAction, EMPTY_FORM_STATE);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);

  useEffect(() => {
    if (state.ok && state.result?.status) {
      router.push(`/groups/${state.result.status}`);
    }
  }, [state, router]);

  return (
    <form action={action} className="space-y-4 rounded-card border border-line bg-surface p-5 sm:p-6">
      <h2 className="font-display text-xl font-semibold text-ink">Create a group</h2>
      <p className="text-sm text-mute">Public groups show in search. Secret groups never appear in browse.</p>
      {state.message && !state.ok ? <ErrorNotice title={state.message} /> : null}
      <Field label="Name" htmlFor="name" error={state.fieldErrors?.name}>
        <Input
          id="name"
          name="name"
          required
          maxLength={120}
          value={name}
          onChange={(e) => {
            const v = e.target.value;
            setName(v);
            if (!slugTouched) setSlug(slugify(v));
          }}
        />
      </Field>
      <Field label="Slug" htmlFor="slug" error={state.fieldErrors?.slug}>
        <Input
          id="slug"
          name="slug"
          required
          maxLength={64}
          value={slug}
          onChange={(e) => {
            setSlugTouched(true);
            setSlug(slugify(e.target.value));
          }}
        />
      </Field>
      <Field label="Description" htmlFor="description">
        <Textarea id="description" name="description" rows={3} maxLength={2000} />
      </Field>
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="Kind" htmlFor="kind">
          <select
            id="kind"
            name="kind"
            defaultValue="skill"
            className="w-full rounded-xl border border-line bg-bg px-3 py-2 text-ink"
          >
            <option value="skill">Skill</option>
            <option value="industry">Industry</option>
            <option value="location">Location</option>
            <option value="company">Company</option>
            <option value="ai_agent">AI agent</option>
          </select>
        </Field>
        <Field label="Visibility" htmlFor="visibility">
          <select
            id="visibility"
            name="visibility"
            defaultValue="public"
            className="w-full rounded-xl border border-line bg-bg px-3 py-2 text-ink"
          >
            <option value="public">Public</option>
            <option value="private">Private</option>
            <option value="secret">Secret</option>
          </select>
        </Field>
        <Field label="Join" htmlFor="join_policy">
          <select
            id="join_policy"
            name="join_policy"
            defaultValue="open"
            className="w-full rounded-xl border border-line bg-bg px-3 py-2 text-ink"
          >
            <option value="open">Open</option>
            <option value="request">Request</option>
            <option value="invite">Invite</option>
          </select>
        </Field>
      </div>
      <Submit />
    </form>
  );
}
