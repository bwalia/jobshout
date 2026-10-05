"use client";

import { useFormState, useFormStatus } from "react-dom";
import { setIndustriesAction } from "@/app/showcase/actions";
import { IndustryPicker } from "@/components/showcase/IndustryPicker";
import { Button } from "@/components/ui";
import { EMPTY_FORM_STATE } from "@/lib/form-state";
import type { IndustryNode, Kind } from "@/lib/showcase";

function Save() {
  const { pending } = useFormStatus();
  return (
    <Button type="submit" size="sm" variant="primary" disabled={pending}>
      {pending ? "Saving…" : "Save industries"}
    </Button>
  );
}

/** Editors: set a published entry's industries in place. It stays live. */
export function ClassifyEntry({ id, kind, tree }: { id: string; kind: Kind; tree: IndustryNode[] }) {
  const [state, action] = useFormState(setIndustriesAction, EMPTY_FORM_STATE);
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
      <IndustryPicker tree={tree} initial={[]} noun={kind === "app" ? "app" : kind} error={state.ok ? undefined : state.message || undefined} />
      <Save />
    </form>
  );
}
