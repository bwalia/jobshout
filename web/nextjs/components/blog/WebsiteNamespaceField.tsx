"use client";

import { useEffect, useMemo, useState } from "react";
import axios from "axios";
import { useQuery } from "@tanstack/react-query";

import { useCreateIntegration, useUpdateIntegration } from "@/lib/hooks/useIntegrations";
import { useBlogConfig } from "@/lib/hooks/useBlog";
import { checkWebsite, getBlogDestinations } from "@/lib/api/blog";
import { apiErrorMessage } from "@/lib/api/client";
import type { WebsiteCheck } from "@/lib/types/blog";
import { ADMIN_ONLY, SLUG } from "@/components/blog/AddWebsiteForm";
import { cn } from "@/lib/utils/cn";

const inputCls =
  "flex h-10 w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50";

const buttonCls =
  "shrink-0 rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50";

/**
 * A "website" schema field: the OpsAPI namespace a run publishes to. Pick a
 * saved one or type a new one. The server says whether it is connected
 * (GET /blogs/destinations/check); a connected namespace reuses its stored
 * key, so no key is asked for. A new one, or one whose key is unusable, shows
 * a key box that saves the key straight to the integration, sealed there.
 *
 * The field's value is only ever the namespace: the key never enters launch
 * values, which are persisted on the task. The launch is checked again on
 * the server, so an unconnected namespace is refused there too.
 */
export function WebsiteNamespaceField({
  id,
  value,
  onChange,
  disabled,
  autoFocus,
  placeholder,
  error,
}: {
  id: string;
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
  autoFocus?: boolean;
  placeholder?: string;
  error?: boolean;
}) {
  const { data: config } = useBlogConfig();
  const { data: sites } = useQuery({
    queryKey: ["blog-destinations"],
    queryFn: getBlogDestinations,
  });
  const create = useCreateIntegration();
  const update = useUpdateIntegration();
  // Only false is a refusal: an older server sends nothing and decides itself.
  const cannotSeal = config?.keys_sealed === false;

  const [check, setCheck] = useState<WebsiteCheck | null>(null);
  const [checking, setChecking] = useState(false);
  const [apiKey, setApiKey] = useState("");
  const [message, setMessage] = useState("");
  // Bumped after a key is saved, to check the namespace again.
  const [recheck, setRecheck] = useState(0);

  const ns = value.trim().toLowerCase();
  const valid = SLUG.test(ns);

  const suggestions = useMemo(() => {
    const out = new Set<string>();
    if (config?.cms_namespace) out.add(config.cms_namespace);
    for (const s of sites ?? []) if (s.namespace) out.add(s.namespace);
    return Array.from(out).sort();
  }, [config?.cms_namespace, sites]);

  useEffect(() => {
    setCheck(null);
    setApiKey("");
    setMessage("");
    if (!valid) return;
    let cancelled = false;
    const t = setTimeout(() => {
      setChecking(true);
      checkWebsite(ns)
        .then((res) => {
          if (!cancelled) setCheck(res);
        })
        .catch((e) => {
          if (!cancelled) setMessage(apiErrorMessage(e, "Could not check the namespace"));
        })
        .finally(() => {
          if (!cancelled) setChecking(false);
        });
    }, 400);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [ns, valid, recheck]);

  const connected = Boolean(check?.exists && check.ready);
  // A saved website whose key is missing or unreadable takes a new one.
  const rekey = Boolean(check?.exists && !check.ready && check.integration_id);
  const needsKey = check !== null && !connected;
  const saving = create.isPending || update.isPending;

  function failed(e: unknown) {
    if (axios.isAxiosError(e) && e.response?.status === 403) setMessage(ADMIN_ONLY);
    // Connected meanwhile by someone else: look again and reuse it.
    else if (axios.isAxiosError(e) && e.response?.status === 409) setRecheck((n) => n + 1);
    else setMessage(apiErrorMessage(e, "Could not connect the namespace"));
  }

  function done() {
    setApiKey("");
    setRecheck((n) => n + 1);
  }

  function connect() {
    if (!check) return;
    const key = apiKey.trim();
    if (!key) {
      setMessage("Paste the namespace's OpsAPI API key.");
      return;
    }
    setMessage("");
    if (rekey && check.integration_id) {
      update.mutate(
        { id: check.integration_id, payload: { credentials: { api_key: key } } },
        { onSuccess: done, onError: failed }
      );
      return;
    }
    const taken = new Set((sites ?? []).map((s) => s.name.toLowerCase()));
    if (taken.has(check.namespace)) {
      setMessage(
        `A website called "${check.namespace}" already exists for another namespace. Add this one in Settings → Integrations under another name.`
      );
      return;
    }
    create.mutate(
      {
        name: check.namespace,
        provider: "opsapi",
        base_url: check.base_url,
        credentials: { api_key: key },
        config: { namespace_slug: check.namespace },
      },
      { onSuccess: done, onError: failed }
    );
  }

  const borderCls = error ? "border-destructive" : "border-input";
  const listId = `${id}-namespaces`;

  return (
    <div className="space-y-2">
      <input
        id={id}
        type="text"
        list={listId}
        value={value}
        onChange={(e) => onChange(e.target.value.toLowerCase())}
        placeholder={placeholder ?? "jobshout"}
        disabled={disabled}
        autoFocus={autoFocus}
        spellCheck={false}
        autoComplete="off"
        aria-invalid={error}
        className={cn(inputCls, borderCls)}
      />
      <datalist id={listId}>
        {suggestions.map((s) => (
          <option key={s} value={s} />
        ))}
      </datalist>

      {ns && !valid && (
        <p className="text-xs text-destructive" role="alert">
          A namespace is the OpsAPI slug: lower-case letters, digits and hyphens, e.g. jobshout.
        </p>
      )}
      {valid && checking && (
        <p className="text-xs text-muted-foreground">Checking {ns}…</p>
      )}
      {connected && check && (
        <p
          className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-foreground"
          data-testid="website-connected"
        >
          <span className="font-medium">{check.namespace}</span> is connected
          {check.ring ? " as this ring's CMS" : check.name ? ` as “${check.name}”` : ""}. Its
          saved key is used; you don&apos;t need to enter it again.
        </p>
      )}
      {needsKey && check && (
        <div className="space-y-2 rounded-md border border-border bg-background/40 p-3">
          <p className="text-xs text-muted-foreground">
            {rekey
              ? `“${check.name}” is saved for ${check.namespace}, but its stored key can't be used. Enter a new key to replace it.`
              : `${check.namespace} isn't connected yet. Enter its OpsAPI API key to connect it; the article is published there.`}
          </p>
          {cannotSeal && (
            <p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
              This server can&apos;t store keys encrypted yet (SECRETS_ENCRYPTION_KEY is not set),
              so it won&apos;t save one. Ask an operator to set it, or pick a connected namespace.
            </p>
          )}
          <label htmlFor={`${id}-api-key`} className="text-xs text-muted-foreground">
            OpsAPI API key
          </label>
          <div className="flex gap-2">
            <input
              id={`${id}-api-key`}
              type="password"
              autoComplete="new-password"
              spellCheck={false}
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              onKeyDown={(e) => {
                // Inside the task form: Enter connects instead of submitting it.
                if (e.key === "Enter") {
                  e.preventDefault();
                  connect();
                }
              }}
              placeholder="opsk_… with cms create, update and read"
              disabled={disabled || saving || cannotSeal}
              className={cn(inputCls, "border-input")}
            />
            <button
              type="button"
              onClick={connect}
              disabled={disabled || saving || cannotSeal || !apiKey.trim()}
              className={buttonCls}
            >
              {saving ? "Connecting…" : rekey ? "Save key" : "Connect"}
            </button>
          </div>
          <p className="text-2xs text-muted-foreground">
            The key is sent once, stored encrypted on the server and never shown again. Runs
            for an unconnected namespace are refused.
          </p>
        </div>
      )}
      {message && (
        <p className="text-xs text-destructive" role="alert">
          {message}
        </p>
      )}
    </div>
  );
}
