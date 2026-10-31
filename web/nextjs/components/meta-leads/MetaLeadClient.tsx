"use client";

import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { apiErrorMessage } from "@/lib/api/client";
import {
  getMetaLeadStatus,
  listMetaConnections,
  listMetaLeads,
  listMetaRunLeads,
  listMetaRuns,
  upsertMetaConnection,
  type MetaLead,
  type MetaLeadConnection,
  type MetaLeadRun,
  type MetaLeadStatus,
} from "@/lib/api/meta-leads";

const inputClass =
  "mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50";

type Tab = "leads" | "runs" | "connection";

export function MetaLeadClient() {
  const [tab, setTab] = useState<Tab>("leads");
  const [status, setStatus] = useState<MetaLeadStatus | null>(null);
  const [connections, setConnections] = useState<MetaLeadConnection[]>([]);
  const [runs, setRuns] = useState<MetaLeadRun[]>([]);
  const [leads, setLeads] = useState<MetaLead[]>([]);
  const [selectedRun, setSelectedRun] = useState<MetaLeadRun | null>(null);
  const [runLeads, setRunLeads] = useState<MetaLead[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const [name, setName] = useState("");
  const [pageId, setPageId] = useState("");
  const [pageName, setPageName] = useState("");
  const [token, setToken] = useState("");

  async function refresh() {
    setLoading(true);
    try {
      const [st, conns, rs, ls] = await Promise.all([
        getMetaLeadStatus(),
        listMetaConnections(),
        listMetaRuns(),
        listMetaLeads(),
      ]);
      setStatus(st);
      setConnections(conns);
      setRuns(rs);
      setLeads(ls);
    } catch (e) {
      toast.error(apiErrorMessage(e, "Failed to load Meta Lead Analyst"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!selectedRun) {
      setRunLeads([]);
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const rows = await listMetaRunLeads(selectedRun.id);
        if (!cancelled) setRunLeads(rows);
      } catch (e) {
        if (!cancelled) toast.error(apiErrorMessage(e, "Failed to load run leads"));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [selectedRun]);

  async function onSaveConnection(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      await upsertMetaConnection({
        name: name.trim(),
        page_id: pageId.trim(),
        page_name: pageName.trim(),
        access_token: token.trim(),
      });
      setToken("");
      toast.success("Page connection saved");
      await refresh();
      setTab("leads");
    } catch (err) {
      toast.error(apiErrorMessage(err, "Failed to save connection"));
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <div className="flex items-center gap-2 p-6 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading Meta Lead Analyst…
      </div>
    );
  }

  return (
    <div className="space-y-6 p-4 sm:p-6">
      <div>
        <h2 className="text-lg font-semibold text-foreground">Meta Lead Analyst</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Pull Instagram/Facebook Lead Ads Instant Form submissions and review LLM briefs.
          Launch a run from New task with a named connection. Tokens stay server-side.
        </p>
        {status ? (
          <p className="mt-2 text-xs text-muted-foreground">
            Seal tokens: {status.can_seal_tokens ? "yes" : "no"} · Webhook verify:{" "}
            {status.webhook_verify ? "yes" : "no"} · LLM: {status.llm ? "yes" : "heuristic"} · Graph{" "}
            {status.graph_version}
          </p>
        ) : null}
      </div>

      <div className="flex gap-2 border-b border-border pb-2 text-sm">
        {(["leads", "runs", "connection"] as Tab[]).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setTab(t)}
            className={`rounded-md px-3 py-1.5 capitalize ${
              tab === t ? "bg-muted font-medium text-foreground" : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      {tab === "connection" ? (
        <div className="grid gap-6 lg:grid-cols-2">
          <form onSubmit={onSaveConnection} className="space-y-3 rounded-lg border border-border p-4">
            <h3 className="text-sm font-semibold">Add or update Page connection</h3>
            <label className="block text-xs font-medium">
              Connection name
              <input className={inputClass} value={name} onChange={(e) => setName(e.target.value)} required placeholder="workstation-ig" />
            </label>
            <label className="block text-xs font-medium">
              Page ID
              <input className={inputClass} value={pageId} onChange={(e) => setPageId(e.target.value)} required placeholder="1234567890" />
            </label>
            <label className="block text-xs font-medium">
              Page name (optional)
              <input className={inputClass} value={pageName} onChange={(e) => setPageName(e.target.value)} placeholder="Workstation" />
            </label>
            <label className="block text-xs font-medium">
              Page access token
              <input
                className={inputClass}
                type="password"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                required
                placeholder="EAAG…"
                autoComplete="off"
              />
            </label>
            <button
              type="submit"
              disabled={saving || !status?.can_seal_tokens}
              className="inline-flex items-center rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
            >
              {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              Save connection
            </button>
            {!status?.can_seal_tokens ? (
              <p className="text-xs text-destructive">META_TOKEN_KEY is not set on this ring — connections cannot be stored.</p>
            ) : null}
          </form>
          <div className="rounded-lg border border-border p-4">
            <h3 className="text-sm font-semibold">Saved connections</h3>
            {connections.length === 0 ? (
              <p className="mt-2 text-sm text-muted-foreground">None yet.</p>
            ) : (
              <ul className="mt-3 space-y-2 text-sm">
                {connections.map((c) => (
                  <li key={c.id} className="rounded-md border border-border px-3 py-2">
                    <div className="font-medium">{c.name}</div>
                    <div className="text-xs text-muted-foreground">
                      {c.page_name || "Page"} · {c.page_id}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      ) : null}

      {tab === "runs" ? (
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="space-y-2">
            {runs.length === 0 ? (
              <p className="text-sm text-muted-foreground">No runs yet. Launch from New task.</p>
            ) : (
              runs.map((run) => (
                <button
                  key={run.id}
                  type="button"
                  onClick={() => setSelectedRun(run)}
                  className={`w-full rounded-md border px-3 py-2 text-left text-sm ${
                    selectedRun?.id === run.id ? "border-primary bg-muted" : "border-border"
                  }`}
                >
                  <div className="flex justify-between gap-2">
                    <span className="font-medium">{run.source}</span>
                    <span className="text-xs text-muted-foreground">{run.status}</span>
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {run.lead_count} leads · {new Date(run.created_at).toLocaleString()}
                  </div>
                </button>
              ))
            )}
          </div>
          <div className="rounded-lg border border-border p-4 text-sm">
            {!selectedRun ? (
              <p className="text-muted-foreground">Select a run.</p>
            ) : (
              <>
                <h3 className="font-semibold">Run detail</h3>
                {selectedRun.rollup ? (
                  <div className="mt-3 space-y-2">
                    <p>
                      <span className="text-muted-foreground">Quality: </span>
                      {selectedRun.rollup.quality_mix || "—"}
                    </p>
                    {selectedRun.rollup.themes?.length ? (
                      <p>
                        <span className="text-muted-foreground">Themes: </span>
                        {selectedRun.rollup.themes.join(", ")}
                      </p>
                    ) : null}
                    {selectedRun.rollup.recommended_followups?.length ? (
                      <ul className="list-disc pl-5">
                        {selectedRun.rollup.recommended_followups.map((f) => (
                          <li key={f}>{f}</li>
                        ))}
                      </ul>
                    ) : null}
                  </div>
                ) : (
                  <p className="mt-2 text-muted-foreground">No rollup yet.</p>
                )}
                <div className="mt-4 space-y-2">
                  {runLeads.map((lead) => (
                    <LeadCard key={lead.id} lead={lead} />
                  ))}
                </div>
              </>
            )}
          </div>
        </div>
      ) : null}

      {tab === "leads" ? (
        <div className="space-y-2">
          {leads.length === 0 ? (
            <p className="text-sm text-muted-foreground">No analysed leads yet.</p>
          ) : (
            leads.map((lead) => <LeadCard key={lead.id} lead={lead} />)
          )}
        </div>
      ) : null}
    </div>
  );
}

function LeadCard({ lead }: { lead: MetaLead }) {
  const a = lead.analysis;
  return (
    <div className="rounded-md border border-border px-3 py-2 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-medium">{a?.summary || lead.leadgen_id}</span>
        <span className="text-xs text-muted-foreground">
          {a?.fit_score != null ? `fit ${a.fit_score}` : lead.status}
          {a?.spam_risk ? ` · spam ${a.spam_risk}` : ""}
        </span>
      </div>
      {a?.intent ? <p className="mt-1 text-muted-foreground">Intent: {a.intent}</p> : null}
      {a?.next_action ? <p className="text-muted-foreground">Next: {a.next_action}</p> : null}
    </div>
  );
}
