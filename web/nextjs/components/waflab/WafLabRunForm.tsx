"use client";

import { useEffect, useState } from "react";
import { apiErrorMessage } from "@/lib/api/client";
import {
  createWafLabRun,
  wafLabStatus,
  type CreateWAFLabRunRequest,
  type WAFLabRun,
} from "@/lib/api/waf-lab";

const inputClass =
  "mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50";

interface WafLabRunFormProps {
  agentId: string;
  onRunCreated?: (run: WAFLabRun) => void;
}

export function WafLabRunForm({ agentId, onRunCreated }: WafLabRunFormProps) {
  const [wslproxyBaseUrl, setWslproxyBaseUrl] = useState("https://lon1.pop0.uk");
  const [secureHost, setSecureHost] = useState("payments-secure.fictionally.org");
  const [openHost, setOpenHost] = useState("payments-open.fictionally.org");
  const [originUpstream, setOriginUpstream] = useState("127.0.0.1:30084");
  const [policyId, setPolicyId] = useState("waf-policy-payments-hard");
  const [mode, setMode] = useState("provision_and_test");
  const [manageDns, setManageDns] = useState("off");
  const [dnsZone, setDnsZone] = useState("fictionally.org");
  const [attackSet, setAttackSet] = useState("full");
  const [instruction, setInstruction] = useState("");
  const [statusNote, setStatusNote] = useState("");
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const st = await wafLabStatus();
        if (cancelled) return;
        setEnabled(Boolean(st.enabled));
        if (typeof st.wslproxy_base_url === "string" && st.wslproxy_base_url) {
          setWslproxyBaseUrl(st.wslproxy_base_url);
        }
        const cf = st.cloudflare_configured ? "Cloudflare DNS available" : "DNS phase will skip without CLOUDFLARE_API_TOKEN";
        setStatusNote(
          st.enabled
            ? `wslproxy ready · ${cf}`
            : "WAF Efficacy Lab is not configured (set WSLPROXY_BASE_URL and credentials)."
        );
      } catch (err) {
        if (!cancelled) {
          setEnabled(false);
          setStatusNote(apiErrorMessage(err, "Could not load WAF lab status"));
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const body: CreateWAFLabRunRequest = {
        agent_id: agentId,
        wslproxy_base_url: wslproxyBaseUrl.trim(),
        secure_host: secureHost.trim(),
        open_host: openHost.trim(),
        origin_upstream: originUpstream.trim(),
        policy_id: policyId.trim(),
        mode,
        manage_dns: manageDns,
        dns_zone: manageDns === "cloudflare" ? dnsZone.trim() : "",
        attack_set: attackSet,
        instruction: instruction.trim() || undefined,
      };
      const run = await createWafLabRun(body);
      onRunCreated?.(run);
    } catch (err) {
      setError(apiErrorMessage(err, "Failed to start WAF lab run"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={submit} className="space-y-4">
      {statusNote && (
        <p
          className={`rounded-md border px-3 py-2 text-sm ${
            enabled
              ? "border-border bg-muted/40 text-muted-foreground"
              : "border-destructive/40 bg-destructive/10 text-destructive"
          }`}
        >
          {statusNote}
        </p>
      )}

      <Field label="wslproxy base URL">
        <input
          className={inputClass}
          value={wslproxyBaseUrl}
          onChange={(e) => setWslproxyBaseUrl(e.target.value)}
          required
        />
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Secure host (WAF on)">
          <input
            className={inputClass}
            value={secureHost}
            onChange={(e) => setSecureHost(e.target.value)}
            required
          />
        </Field>
        <Field label="Open host (WAF off)">
          <input
            className={inputClass}
            value={openHost}
            onChange={(e) => setOpenHost(e.target.value)}
            required
          />
        </Field>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Origin upstream">
          <input
            className={inputClass}
            value={originUpstream}
            onChange={(e) => setOriginUpstream(e.target.value)}
          />
        </Field>
        <Field label="WAF policy id">
          <input className={inputClass} value={policyId} onChange={(e) => setPolicyId(e.target.value)} />
        </Field>
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="Mode">
          <select className={inputClass} value={mode} onChange={(e) => setMode(e.target.value)}>
            <option value="provision_and_test">Provision and test</option>
            <option value="provision_only">Provision only</option>
            <option value="test_only">Test only</option>
          </select>
        </Field>
        <Field label="Manage DNS">
          <select className={inputClass} value={manageDns} onChange={(e) => setManageDns(e.target.value)}>
            <option value="off">Off (manual DNS)</option>
            <option value="cloudflare">Cloudflare via wslproxy</option>
          </select>
        </Field>
        <Field label="Attack set">
          <select className={inputClass} value={attackSet} onChange={(e) => setAttackSet(e.target.value)}>
            <option value="full">Full matrix</option>
            <option value="owasp_core">OWASP core</option>
            <option value="modern_api">Modern / API</option>
            <option value="stages_only">v2 stages only</option>
          </select>
        </Field>
      </div>
      {manageDns === "cloudflare" && (
        <Field label="DNS zone">
          <input className={inputClass} value={dnsZone} onChange={(e) => setDnsZone(e.target.value)} />
        </Field>
      )}
      <Field label="Note (optional)">
        <textarea
          className={inputClass}
          rows={3}
          value={instruction}
          onChange={(e) => setInstruction(e.target.value)}
          placeholder="Carried into the report"
        />
      </Field>

      {error && <p className="text-sm text-destructive">{error}</p>}

      <button
        type="submit"
        disabled={submitting || enabled === false}
        className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
      >
        {submitting ? "Starting…" : "Start WAF lab"}
      </button>
    </form>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block text-sm">
      <span className="font-medium text-foreground">{label}</span>
      {children}
    </label>
  );
}
