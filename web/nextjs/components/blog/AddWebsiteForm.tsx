"use client";

import { useEffect, useState } from "react";
import axios from "axios";
import { useCreateIntegration, useUpdateIntegration } from "@/lib/hooks/useIntegrations";
import { useBlogConfig } from "@/lib/hooks/useBlog";
import { checkWebsite } from "@/lib/api/blog";
import { apiErrorMessage } from "@/lib/api/client";
import type { WebsiteCheck } from "@/lib/types/blog";

const inputClass =
  "mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground";

const buttonClass =
  "rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50";

/** OpsAPI namespace slugs: lower-case letters, digits and hyphens. */
const SLUG = /^[a-z0-9][a-z0-9-]*$/;

const ADMIN_ONLY =
  "Only organization admins can add websites or keys. Ask one to add it in Settings → Integrations.";

/**
 * Adds an OpsAPI website in two steps. First the namespace: the server
 * validates it and says whether the org already saved it. A saved namespace
 * reuses its stored key, so no key is asked for; a new one asks for its key,
 * which goes to the server once and is sealed there. Keys are never read back.
 *
 * The name defaults to the namespace and the URL to this ring's OpsAPI, both
 * under "More options".
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
  /** Called with the website's name, saved now or already. */
  onSaved: (name: string) => void;
  onCancel: () => void;
}) {
  const create = useCreateIntegration();
  const update = useUpdateIntegration();
  const { data: config } = useBlogConfig();
  const ringURL = config?.opsapi_base_url ?? "";
  // Only false is a refusal: an older server sends nothing and decides itself.
  const cannotSeal = config?.keys_sealed === false;

  const [namespace, setNamespace] = useState("");
  const [baseURL, setBaseURL] = useState(ringURL);
  const [check, setCheck] = useState<WebsiteCheck | null>(null);
  const [checking, setChecking] = useState(false);
  const [apiKey, setApiKey] = useState("");
  const [name, setName] = useState("");
  const [more, setMore] = useState(false);
  const [error, setError] = useState("");

  // The config can arrive after the form opens.
  useEffect(() => {
    setBaseURL((u) => u || ringURL);
  }, [ringURL]);

  const saving = create.isPending || update.isPending;
  const reuse = check?.exists && check.ready;
  // A saved website whose key is missing or unreadable takes a new one.
  const rekey = check?.exists && !check.ready;
  const needsKey = check !== null && !reuse;
  const siteName = name.trim() || check?.namespace || "";

  // Any change to what was checked means checking again.
  function edit(apply: () => void) {
    apply();
    setCheck(null);
    setApiKey("");
    setError("");
  }

  async function runCheck() {
    const ns = namespace.trim();
    if (!SLUG.test(ns)) {
      setError("The namespace is the OpsAPI slug: lower-case letters, digits and hyphens, e.g. jobshout.");
      return;
    }
    if (!baseURL.trim()) {
      setMore(true);
      setError("Set the OpsAPI URL under More options: this ring has no default.");
      return;
    }
    setError("");
    setChecking(true);
    try {
      setCheck(await checkWebsite(ns, baseURL.trim()));
    } catch (e) {
      setError(apiErrorMessage(e, "Could not check the namespace"));
    } finally {
      setChecking(false);
    }
  }

  function failed(e: unknown) {
    if (axios.isAxiosError(e) && e.response?.status === 403) setError(ADMIN_ONLY);
    else if (axios.isAxiosError(e) && e.response?.status === 409) {
      // Saved meanwhile by someone else: look again and reuse it.
      setError("");
      void runCheck();
    } else setError(apiErrorMessage(e, "Could not save the website"));
  }

  function save() {
    if (!check) return;
    if (reuse) {
      onSaved(check.name ?? check.namespace);
      return;
    }
    if (!apiKey.trim()) {
      setError("Paste the namespace's OpsAPI API key.");
      return;
    }
    if (rekey && check.integration_id) {
      const existing = check.name ?? check.namespace;
      update.mutate(
        { id: check.integration_id, payload: { credentials: { api_key: apiKey.trim() } } },
        {
          onSuccess: () => {
            setApiKey("");
            onSaved(existing);
          },
          onError: failed,
        }
      );
      return;
    }
    if (takenNames.includes(siteName.toLowerCase())) {
      setMore(true);
      setError(`A website called "${siteName}" already exists. Give this one another name under More options.`);
      return;
    }
    create.mutate(
      {
        name: siteName,
        provider: "opsapi",
        base_url: check.base_url,
        credentials: { api_key: apiKey.trim() },
        config: { namespace_slug: check.namespace },
      },
      {
        onSuccess: () => {
          setApiKey("");
          onSaved(siteName);
        },
        onError: failed,
      }
    );
  }

  return (
    <div className="space-y-3 rounded-md border border-border bg-background/40 p-4">
      <p className="text-xs text-muted-foreground">
        Enter the OpsAPI namespace to publish to. If it&apos;s already saved, its
        stored key is reused; a new one asks for its key, which is encrypted on
        the server and never shown again.
      </p>

      <div>
        <label htmlFor="site-namespace" className="text-xs text-muted-foreground">
          Namespace
        </label>
        <div className="flex gap-2">
          <input
            id="site-namespace"
            value={namespace}
            onChange={(e) => edit(() => setNamespace(e.target.value.toLowerCase()))}
            onKeyDown={(e) => {
              // Inside another form: Enter checks instead of submitting it.
              if (e.key === "Enter") {
                e.preventDefault();
                void runCheck();
              }
            }}
            placeholder="jobshout"
            spellCheck={false}
            className={inputClass}
          />
          {!check && (
            <button
              type="button"
              onClick={() => void runCheck()}
              disabled={checking || !namespace.trim()}
              className={`mt-1 shrink-0 ${buttonClass}`}
            >
              {checking ? "Checking..." : "Continue"}
            </button>
          )}
        </div>
      </div>

      {reuse && (
        <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-foreground">
          <span className="font-medium">{check.namespace}</span> is already saved as &ldquo;
          {check.name}&rdquo;. Its stored key will be used; you don&apos;t need to enter it again.
        </p>
      )}

      {needsKey && (
        <>
          <p className="text-xs text-muted-foreground">
            {rekey
              ? `"${check.name}" is saved for this namespace, but its stored key can't be used. Enter a new key to replace it.`
              : `${check.namespace} is new. Enter its key to save it.`}
          </p>
          {cannotSeal && (
            <p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
              This server can&apos;t store keys encrypted yet (SECRETS_ENCRYPTION_KEY is not
              set), so it won&apos;t save one. Ask an operator to set it, then add the website.
            </p>
          )}
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
        </>
      )}

      <button
        type="button"
        onClick={() => setMore((v) => !v)}
        className="text-xs text-muted-foreground underline hover:text-foreground"
        aria-expanded={more}
      >
        {more ? "Fewer options" : "More options"}
      </button>
      {!more && check && !check.exists && (
        <span className="ml-2 text-2xs text-muted-foreground">
          Saved as &ldquo;{siteName}&rdquo; on {check.base_url}
        </span>
      )}
      {more && (
        <div className="grid gap-3 sm:grid-cols-2">
          <div className={check && !check.exists ? "" : "sm:col-span-2"}>
            <label htmlFor="site-url" className="text-xs text-muted-foreground">
              OpsAPI URL
            </label>
            <input
              id="site-url"
              value={baseURL}
              onChange={(e) => edit(() => setBaseURL(e.target.value))}
              placeholder="https://opsapi.workstation.co.uk"
              spellCheck={false}
              className={inputClass}
            />
          </div>
          {check && !check.exists && (
            <div>
              <label htmlFor="site-name" className="text-xs text-muted-foreground">
                Name (optional)
              </label>
              <input
                id="site-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={check.namespace}
                className={inputClass}
              />
            </div>
          )}
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
        {check && (
          <button
            type="button"
            onClick={save}
            disabled={saving || (needsKey && cannotSeal)}
            className={buttonClass}
          >
            {saving ? "Saving..." : reuse ? "Use this website" : rekey ? "Save new key" : "Save website"}
          </button>
        )}
      </div>
    </div>
  );
}
