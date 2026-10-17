"use client";

import { useState } from "react";
import axios from "axios";
import { useCreateIntegration } from "@/lib/hooks/useIntegrations";
import { apiErrorMessage } from "@/lib/api/client";

const inputClass =
  "mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground";

/** OpsAPI namespace slugs: lower-case letters, digits and hyphens. */
const SLUG = /^[a-z0-9][a-z0-9-]*$/;

/**
 * Saves an OpsAPI website — a namespace and its key — as an opsapi
 * integration, the same record Settings → Integrations edits. The key goes to
 * the server once and is sealed there; it is never read back, so it lives in
 * this form only until it is sent.
 *
 * It is a <div>, not a <form>: it sits inside the article dialog's form.
 */
export function AddWebsiteForm({
  defaultBaseURL,
  takenNames,
  onSaved,
  onCancel,
}: {
  defaultBaseURL: string;
  /** Existing website names, lower-cased: names are how runs pick a site. */
  takenNames: string[];
  onSaved: (name: string) => void;
  onCancel: () => void;
}) {
  const create = useCreateIntegration();
  const [name, setName] = useState("");
  const [baseURL, setBaseURL] = useState(defaultBaseURL);
  const [namespace, setNamespace] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [error, setError] = useState("");

  function problem(): string {
    const n = name.trim();
    if (!n) return "Give the website a name, e.g. JobShout.";
    if (takenNames.includes(n.toLowerCase())) return `A website called "${n}" already exists.`;
    try {
      const u = new URL(baseURL.trim());
      if (u.protocol !== "https:" && u.protocol !== "http:") throw new Error();
    } catch {
      return "The OpsAPI URL must be a full http(s) address.";
    }
    if (!SLUG.test(namespace.trim())) {
      return "The namespace is the OpsAPI slug: lower-case letters, digits and hyphens.";
    }
    if (!apiKey.trim()) return "Paste the namespace's OpsAPI API key.";
    return "";
  }

  function save() {
    const p = problem();
    setError(p);
    if (p) return;
    const n = name.trim();
    create.mutate(
      {
        name: n,
        provider: "opsapi",
        base_url: baseURL.trim().replace(/\/+$/, ""),
        credentials: { api_key: apiKey.trim() },
        config: { namespace_slug: namespace.trim() },
      },
      {
        onSuccess: () => {
          setApiKey("");
          onSaved(n);
        },
        onError: (e) => {
          setError(
            axios.isAxiosError(e) && e.response?.status === 403
              ? "Only organization admins can add websites. Ask one to add it in Settings → Integrations."
              : apiErrorMessage(e, "Could not save the website")
          );
        },
      }
    );
  }

  return (
    <div className="space-y-3 rounded-md border border-border bg-background/40 p-4">
      <p className="text-xs text-muted-foreground">
        Add an OpsAPI namespace to publish to. The key is encrypted on the
        server and never shown again; future articles reuse it.
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <label htmlFor="site-name" className="text-xs text-muted-foreground">
            Name
          </label>
          <input
            id="site-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="JobShout"
            className={inputClass}
          />
        </div>
        <div>
          <label htmlFor="site-namespace" className="text-xs text-muted-foreground">
            Namespace
          </label>
          <input
            id="site-namespace"
            value={namespace}
            onChange={(e) => setNamespace(e.target.value.toLowerCase())}
            placeholder="jobshout"
            spellCheck={false}
            className={inputClass}
          />
        </div>
        <div className="sm:col-span-2">
          <label htmlFor="site-url" className="text-xs text-muted-foreground">
            OpsAPI URL
          </label>
          <input
            id="site-url"
            value={baseURL}
            onChange={(e) => setBaseURL(e.target.value)}
            placeholder="https://opsapi.workstation.co.uk"
            spellCheck={false}
            className={inputClass}
          />
        </div>
        <div className="sm:col-span-2">
          <label htmlFor="site-key" className="text-xs text-muted-foreground">
            API key
          </label>
          <input
            id="site-key"
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder="A namespace key with cms create, update and read"
            className={inputClass}
          />
        </div>
      </div>
      {error && <p className="text-xs text-destructive">{error}</p>}
      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="rounded-md border border-border px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent"
        >
          Cancel
        </button>
        <button
          type="button"
          onClick={save}
          disabled={create.isPending}
          className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
        >
          {create.isPending ? "Saving..." : "Save website"}
        </button>
      </div>
    </div>
  );
}
