"use client";

import { useEffect, useState } from "react";
import axios from "axios";
import { useCreateIntegration } from "@/lib/hooks/useIntegrations";
import { useBlogConfig } from "@/lib/hooks/useBlog";
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
 * Namespace and key are all most sites need: the name defaults to the
 * namespace and the OpsAPI URL to this ring's, both under "More options".
 *
 * It is a <div>, not a <form>: it sits inside other forms.
 */
export function AddWebsiteForm({
  takenNames,
  onSaved,
  onCancel,
}: {
  /** Existing website names, lower-cased: names are how runs pick a site. */
  takenNames: string[];
  onSaved: (name: string) => void;
  onCancel: () => void;
}) {
  const create = useCreateIntegration();
  const { data: config } = useBlogConfig();
  const ringURL = config?.opsapi_base_url ?? "";
  // Only false is a refusal: an older server sends nothing and decides itself.
  const cannotSeal = config?.keys_sealed === false;

  const [namespace, setNamespace] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [name, setName] = useState("");
  const [baseURL, setBaseURL] = useState(ringURL);
  const [more, setMore] = useState(false);
  const [error, setError] = useState("");

  // The config can arrive after the form opens.
  useEffect(() => {
    setBaseURL((u) => u || ringURL);
  }, [ringURL]);

  const siteName = name.trim() || namespace.trim();

  function problem(): string {
    if (!SLUG.test(namespace.trim())) {
      return "The namespace is the OpsAPI slug: lower-case letters, digits and hyphens, e.g. jobshout.";
    }
    if (!apiKey.trim()) return "Paste the namespace's OpsAPI API key.";
    if (takenNames.includes(siteName.toLowerCase())) {
      return `A website called "${siteName}" already exists. Pick it from the list, or give this one another name under More options.`;
    }
    try {
      const u = new URL(baseURL.trim());
      if (u.protocol !== "https:" && u.protocol !== "http:") throw new Error();
    } catch {
      return "Set the OpsAPI URL under More options: this ring has no default.";
    }
    return "";
  }

  function save() {
    const p = problem();
    setError(p);
    if (p) {
      if (!ringURL || p.includes("More options")) setMore(true);
      return;
    }
    create.mutate(
      {
        name: siteName,
        provider: "opsapi",
        base_url: baseURL.trim().replace(/\/+$/, ""),
        credentials: { api_key: apiKey.trim() },
        config: { namespace_slug: namespace.trim() },
      },
      {
        onSuccess: () => {
          setApiKey("");
          onSaved(siteName);
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
      {cannotSeal && (
        <p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          This server can&apos;t store keys encrypted yet (SECRETS_ENCRYPTION_KEY is not
          set), so it won&apos;t save one. Ask an operator to set it, then add the website.
        </p>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
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
        <div>
          <label htmlFor="site-key" className="text-xs text-muted-foreground">
            OpsAPI API key
          </label>
          <input
            id="site-key"
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder="opsk_… with cms create, update and read"
            className={inputClass}
          />
        </div>
      </div>

      <button
        type="button"
        onClick={() => setMore((v) => !v)}
        className="text-xs text-muted-foreground underline hover:text-foreground"
        aria-expanded={more}
      >
        {more ? "Fewer options" : "More options"}
      </button>
      {!more && (
        <span className="ml-2 text-2xs text-muted-foreground">
          Saved as &ldquo;{siteName || "namespace"}&rdquo;
          {ringURL ? ` on ${ringURL}` : ""}
        </span>
      )}
      {more && (
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <label htmlFor="site-name" className="text-xs text-muted-foreground">
              Name (optional)
            </label>
            <input
              id="site-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={namespace || "JobShout"}
              className={inputClass}
            />
          </div>
          <div>
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
        </div>
      )}

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
          disabled={create.isPending || cannotSeal}
          className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
        >
          {create.isPending ? "Saving..." : "Save website"}
        </button>
      </div>
    </div>
  );
}
