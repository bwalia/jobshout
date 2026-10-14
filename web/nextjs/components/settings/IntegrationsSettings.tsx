"use client";

import { useState } from "react";
import axios from "axios";
import { apiErrorMessage } from "@/lib/api/client";
import type { Integration, IntegrationProvider } from "@/lib/api/integrations";
import {
  useCreateIntegration,
  useDeleteIntegration,
  useIntegrations,
  useUpdateIntegration,
} from "@/lib/hooks/useIntegrations";

/** Where a form field is stored on the integration. */
type Where = "base_url" | "credentials" | "config";

interface Field {
  where: Where;
  key: string;
  label: string;
  help: string;
  secret?: boolean;
  type?: "url" | "email" | "text";
  placeholder?: string;
}

// What each provider needs, as the server-side code reads it
// (opsapi: blog destination + opsapi tools; jira/github: their adapters).
const PROVIDERS: { value: IntegrationProvider; label: string; fields: Field[] }[] = [
  {
    value: "opsapi",
    label: "OpsAPI (your website's CMS)",
    fields: [
      {
        where: "base_url",
        key: "base_url",
        label: "OpsAPI address",
        help: "The web address of your OpsAPI server.",
        type: "url",
        placeholder: "https://opsapi.example.com",
      },
      {
        where: "credentials",
        key: "api_key",
        label: "API key",
        help: "The key OpsAPI gave you for this website. It usually starts with opsk_.",
        secret: true,
      },
      {
        where: "config",
        key: "namespace_slug",
        label: "Website namespace",
        help: "The short name of the website in OpsAPI that should receive the articles.",
        placeholder: "my-website",
      },
    ],
  },
  {
    value: "jira",
    label: "Jira",
    fields: [
      {
        where: "base_url",
        key: "base_url",
        label: "Jira address",
        help: "The address you open Jira at.",
        type: "url",
        placeholder: "https://yourcompany.atlassian.net",
      },
      {
        where: "credentials",
        key: "email",
        label: "Jira email",
        help: "The email address you sign in to Jira with.",
        type: "email",
      },
      {
        where: "credentials",
        key: "api_token",
        label: "API token",
        help: "Create one in your Atlassian account, under Security, then API tokens.",
        secret: true,
      },
      {
        where: "credentials",
        key: "project_key",
        label: "Project key",
        help: "The short code in front of your issue numbers, for example OPS in OPS-123.",
        placeholder: "OPS",
      },
    ],
  },
  {
    value: "github",
    label: "GitHub",
    fields: [
      {
        where: "credentials",
        key: "token",
        label: "Access token",
        help: "A GitHub personal access token that is allowed to create issues in the repository.",
        secret: true,
      },
      {
        where: "credentials",
        key: "owner",
        label: "Owner",
        help: "The person or organisation that owns the repository, e.g. acme in github.com/acme/website.",
        placeholder: "acme",
      },
      {
        where: "credentials",
        key: "repo",
        label: "Repository",
        help: "The repository name, e.g. website in github.com/acme/website.",
        placeholder: "website",
      },
    ],
  },
];

const PROVIDER_LABEL: Record<string, string> = {
  opsapi: "OpsAPI",
  jira: "Jira",
  github: "GitHub",
  slack: "Slack",
  teams: "Microsoft Teams",
};

const fieldsFor = (provider: string) => PROVIDERS.find((p) => p.value === provider)?.fields ?? [];
const fieldId = (f: Field) => `integration-${f.where}-${f.key}`;

const inputClass =
  "flex h-11 w-full rounded-md border border-input bg-background px-3 py-2 text-base ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:bg-muted disabled:text-muted-foreground";
const primaryButton =
  "inline-flex h-11 items-center rounded-md bg-primary px-5 text-base font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2";
const secondaryButton =
  "inline-flex h-11 items-center rounded-md border border-input bg-background px-4 text-base font-medium hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2";

function initialValues(editing: Integration | null): Record<string, string> {
  const values: Record<string, string> = {};
  if (!editing) return values;
  for (const f of fieldsFor(editing.provider)) {
    if (f.secret) continue; // never prefill a secret
    const v =
      f.where === "base_url"
        ? editing.base_url
        : f.where === "credentials"
          ? editing.credentials?.[f.key]
          : editing.config?.[f.key];
    values[fieldId(f)] = typeof v === "string" ? v : "";
  }
  return values;
}

function IntegrationForm({ editing, onDone }: { editing: Integration | null; onDone: () => void }) {
  const [name, setName] = useState(editing?.name ?? "");
  const [provider, setProvider] = useState<string>(editing?.provider ?? "opsapi");
  const [values, setValues] = useState<Record<string, string>>(() => initialValues(editing));
  const [error, setError] = useState<string | null>(null);
  const create = useCreateIntegration();
  const update = useUpdateIntegration();
  const saving = create.isPending || update.isPending;
  const fields = fieldsFor(provider);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    let baseURL: string | undefined;
    const credentials: Record<string, string> = {};
    const config: Record<string, unknown> = {};
    for (const f of fields) {
      const v = (values[fieldId(f)] ?? "").trim();
      if (!v) continue; // blank = keep what is saved (all fields are required when adding)
      if (f.where === "base_url") baseURL = v;
      else if (f.where === "credentials") credentials[f.key] = v;
      else config[f.key] = v;
    }
    const onError = (err: unknown) => setError(apiErrorMessage(err, "Something went wrong."));

    if (editing) {
      update.mutate(
        {
          id: editing.id,
          payload: {
            name: name.trim(),
            base_url: baseURL,
            credentials,
            config: { ...editing.config, ...config },
          },
        },
        { onSuccess: onDone, onError }
      );
    } else {
      create.mutate(
        {
          name: name.trim(),
          provider: provider as IntegrationProvider,
          base_url: baseURL,
          credentials,
          config,
        },
        { onSuccess: onDone, onError }
      );
    }
  }

  return (
    <form
      onSubmit={handleSubmit}
      aria-labelledby="integration-form-title"
      className="mt-6 space-y-5 rounded-lg border border-border bg-background p-5"
    >
      <h3 id="integration-form-title" className="text-lg font-semibold">
        {editing ? `Edit "${editing.name}"` : "Add an integration"}
      </h3>

      <div className="space-y-2">
        <label htmlFor="integration-provider" className="text-base font-medium">
          Service
        </label>
        <select
          id="integration-provider"
          value={provider}
          disabled={!!editing}
          onChange={(e) => setProvider(e.target.value)}
          aria-describedby="integration-provider-help"
          className={inputClass}
        >
          {PROVIDERS.map((p) => (
            <option key={p.value} value={p.value}>
              {p.label}
            </option>
          ))}
          {editing && !fieldsFor(editing.provider).length && (
            <option value={editing.provider}>{PROVIDER_LABEL[editing.provider] ?? editing.provider}</option>
          )}
        </select>
        <p id="integration-provider-help" className="text-sm text-muted-foreground">
          {editing ? "The service can't be changed. Remove this one and add a new one instead." : "The tool you want JobShout to connect to."}
        </p>
      </div>

      <div className="space-y-2">
        <label htmlFor="integration-name" className="text-base font-medium">
          Name
        </label>
        <input
          id="integration-name"
          required
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. Company website"
          aria-describedby="integration-name-help"
          className={inputClass}
        />
        <p id="integration-name-help" className="text-sm text-muted-foreground">
          A name you will recognise. Agents and the writer settings show this name.
        </p>
      </div>

      {fields.map((f) => {
        const id = fieldId(f);
        const saved = editing?.credential_keys.includes(f.key);
        const help = f.secret && editing
          ? saved
            ? `${f.help} A value is saved (${editing.credentials?.[f.key] ?? "hidden"}).`
            : `${f.help} Nothing is saved yet.`
          : f.help;
        return (
          <div key={id} className="space-y-2">
            <label htmlFor={id} className="text-base font-medium">
              {f.label}
            </label>
            <input
              id={id}
              type={f.secret ? "password" : (f.type ?? "text")}
              autoComplete={f.secret ? "new-password" : "off"}
              required={!editing}
              value={values[id] ?? ""}
              onChange={(e) => setValues((v) => ({ ...v, [id]: e.target.value }))}
              placeholder={f.secret && editing && saved ? "Leave blank to keep the current value" : f.placeholder}
              aria-describedby={`${id}-help`}
              className={inputClass}
            />
            <p id={`${id}-help`} className="text-sm text-muted-foreground">
              {help}
            </p>
          </div>
        );
      })}

      {error && (
        <p role="alert" className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-base text-destructive">
          {error}
        </p>
      )}

      <div className="flex flex-wrap gap-3">
        <button type="submit" disabled={saving} className={primaryButton}>
          {saving ? "Saving…" : editing ? "Save changes" : "Add integration"}
        </button>
        <button type="button" onClick={onDone} disabled={saving} className={secondaryButton}>
          Cancel
        </button>
      </div>
    </form>
  );
}

/** Settings → Integrations: connect the org to OpsAPI, Jira and GitHub. */
export function IntegrationsSettings() {
  const { data, isLoading, error } = useIntegrations();
  const remove = useDeleteIntegration();
  // null = form closed, "new" = adding, otherwise the integration being edited.
  const [open, setOpen] = useState<Integration | "new" | null>(null);
  const forbidden = axios.isAxiosError(error) && error.response?.status === 403;

  function handleRemove(i: Integration) {
    if (!confirm(`Remove "${i.name}"? Agents will no longer be able to use it.`)) return;
    remove.mutate(i.id);
  }

  return (
    <section id="integrations" className="scroll-mt-20 rounded-xl border border-border bg-card p-6">
      <h2 className="text-base font-semibold">Integrations</h2>
      <p className="mt-1 text-sm text-muted-foreground">
        Connect JobShout to the tools you already use — for example your CMS (OpsAPI), Jira or
        GitHub — so agents can file their work there.
      </p>

      {forbidden ? (
        <p className="mt-6 text-base text-muted-foreground">
          Only organisation admins can see and change integrations. Ask an admin if you need one added.
        </p>
      ) : isLoading ? (
        <p className="mt-6 text-base text-muted-foreground">Loading…</p>
      ) : error ? (
        <p role="alert" className="mt-6 text-base text-destructive">
          {apiErrorMessage(error, "Could not load integrations.")}
        </p>
      ) : (
        <>
          {data && data.length > 0 ? (
            <ul className="mt-6 divide-y divide-border rounded-lg border border-border">
              {data.map((i) => (
                <li key={i.id} className="flex flex-wrap items-center justify-between gap-3 p-4">
                  <div className="min-w-0">
                    <p className="text-base font-medium">{i.name}</p>
                    <p className="text-sm text-muted-foreground">
                      {PROVIDER_LABEL[i.provider] ?? i.provider} ·{" "}
                      {i.status === "active" ? "Active" : i.status} · Last updated{" "}
                      {new Date(i.updated_at).toLocaleDateString()}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    <button type="button" onClick={() => setOpen(i)} className={secondaryButton}>
                      Edit
                    </button>
                    <button
                      type="button"
                      onClick={() => handleRemove(i)}
                      disabled={remove.isPending}
                      className={`${secondaryButton} text-destructive`}
                      aria-label={`Remove ${i.name}`}
                    >
                      Remove
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          ) : (
            <p className="mt-6 text-base text-muted-foreground">No integrations yet.</p>
          )}

          {open ? (
            <IntegrationForm
              key={open === "new" ? "new" : open.id}
              editing={open === "new" ? null : open}
              onDone={() => setOpen(null)}
            />
          ) : (
            <button type="button" onClick={() => setOpen("new")} className={`${primaryButton} mt-6`}>
              Add integration
            </button>
          )}
        </>
      )}
    </section>
  );
}
