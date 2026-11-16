"use client";

import { useActionState } from "react";
import { createOrgAction, createRequisitionAction, type HireActionState } from "@/app/hire/actions";
import type { HireOrg, Requisition } from "@/lib/hire";
import { buttonClass } from "@/components/ui";

const initial: HireActionState = { ok: false };

export function CreateOrgForm() {
  const [state, action, pending] = useActionState(createOrgAction, initial);
  return (
    <form action={action} className="space-y-3 rounded-2xl border border-line bg-raised/40 p-5">
      <h2 className="font-display text-lg font-semibold text-ink">Create hiring organisation</h2>
      <p className="text-sm text-mute">
        Contact details stay private until a candidate accepts an invitation.
      </p>
      <label className="block text-sm">
        <span className="text-mute">Name</span>
        <input name="name" required className="mt-1 w-full rounded-xl border border-line bg-canvas px-3 py-2" />
      </label>
      <label className="block text-sm">
        <span className="text-mute">Slug</span>
        <input
          name="slug"
          required
          pattern="[a-z0-9-]+"
          className="mt-1 w-full rounded-xl border border-line bg-canvas px-3 py-2"
        />
      </label>
      {state.error ? <p className="text-sm text-shout">{state.error}</p> : null}
      {state.ok ? <p className="text-sm text-signal">Organisation created.</p> : null}
      <button type="submit" disabled={pending} className={buttonClass("primary", "md")}>
        {pending ? "Creating…" : "Create organisation"}
      </button>
    </form>
  );
}

export function CreateRequisitionForm({ orgs }: { orgs: HireOrg[] }) {
  const [state, action, pending] = useActionState(createRequisitionAction, initial);
  if (orgs.length === 0) return null;
  return (
    <form action={action} className="space-y-3 rounded-2xl border border-line bg-raised/40 p-5">
      <h2 className="font-display text-lg font-semibold text-ink">Open a requisition</h2>
      <label className="block text-sm">
        <span className="text-mute">Organisation</span>
        <select name="org_id" className="mt-1 w-full rounded-xl border border-line bg-canvas px-3 py-2">
          {orgs.map((o) => (
            <option key={o.id} value={o.id}>
              {o.name}
            </option>
          ))}
        </select>
      </label>
      <label className="block text-sm">
        <span className="text-mute">Title</span>
        <input name="title" required className="mt-1 w-full rounded-xl border border-line bg-canvas px-3 py-2" />
      </label>
      <label className="block text-sm">
        <span className="text-mute">Kind</span>
        <select name="kind" className="mt-1 w-full rounded-xl border border-line bg-canvas px-3 py-2">
          <option value="permanent">Permanent</option>
          <option value="contract">Contract</option>
          <option value="freelance">Freelance</option>
        </select>
      </label>
      <label className="block text-sm">
        <span className="text-mute">Location</span>
        <input name="location" className="mt-1 w-full rounded-xl border border-line bg-canvas px-3 py-2" />
      </label>
      <label className="flex items-center gap-2 text-sm text-body">
        <input type="checkbox" name="remote_ok" defaultChecked />
        Remote OK
      </label>
      <label className="block text-sm">
        <span className="text-mute">Skills (comma-separated)</span>
        <input name="skills" className="mt-1 w-full rounded-xl border border-line bg-canvas px-3 py-2" />
      </label>
      <label className="block text-sm">
        <span className="text-mute">Description</span>
        <textarea name="description" rows={4} className="mt-1 w-full rounded-xl border border-line bg-canvas px-3 py-2" />
      </label>
      {state.error ? <p className="text-sm text-shout">{state.error}</p> : null}
      {state.ok ? <p className="text-sm text-signal">Requisition opened.</p> : null}
      <button type="submit" disabled={pending} className={buttonClass("primary", "md")}>
        {pending ? "Saving…" : "Open requisition"}
      </button>
    </form>
  );
}

export function OrgList({
  orgs,
  requisitionsByOrg,
}: {
  orgs: HireOrg[];
  requisitionsByOrg: Record<string, Requisition[]>;
}) {
  if (orgs.length === 0) {
    return <p className="text-sm text-mute">No hiring organisations yet.</p>;
  }
  return (
    <ul className="space-y-6">
      {orgs.map((org) => (
        <li key={org.id} className="rounded-2xl border border-line p-5">
          <h3 className="font-display text-xl font-semibold text-ink">{org.name}</h3>
          <p className="text-sm text-mute">/{org.slug}</p>
          <ul className="mt-4 space-y-2">
            {(requisitionsByOrg[org.id] ?? []).map((r) => (
              <li key={r.id} className="rounded-xl bg-raised/50 px-3 py-2 text-sm">
                <span className="font-medium text-ink">{r.title}</span>
                <span className="text-mute"> · {r.kind} · {r.status}</span>
              </li>
            ))}
            {(requisitionsByOrg[org.id] ?? []).length === 0 ? (
              <li className="text-sm text-mute">No open requisitions.</li>
            ) : null}
          </ul>
        </li>
      ))}
    </ul>
  );
}
