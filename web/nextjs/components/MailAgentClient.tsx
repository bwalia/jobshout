"use client";

import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { apiClient } from "@/lib/api/client";
import type {
  MailConnectionStatus,
  MailDraft,
  MailThread,
  MailThreadDetail,
  PaginatedMailThreads,
} from "@/types/mail";

function statusLabel(status: string): string {
  switch (status) {
    case "new":
      return "New";
    case "classifying":
      return "Classifying";
    case "researching":
      return "Researching";
    case "draft_ready":
      return "Draft ready";
    case "sent":
      return "Sent";
    case "rejected":
      return "Rejected";
    case "ignored":
      return "Ignored";
    case "failed":
      return "Failed";
    default:
      return status;
  }
}

export function MailAgentClient() {
  const search = useSearchParams();
  const [connection, setConnection] = useState<MailConnectionStatus | null>(null);
  const [threads, setThreads] = useState<MailThread[]>([]);
  const [selected, setSelected] = useState<MailThreadDetail | null>(null);
  const [draftBody, setDraftBody] = useState("");
  const [error, setError] = useState("");
  const [polling, setPolling] = useState(false);
  const [busy, setBusy] = useState(false);
  const [senders, setSenders] = useState("");
  const [prefixes, setPrefixes] = useState("");
  const [labels, setLabels] = useState("");

  const loadConnection = useCallback(async () => {
    const { data } = await apiClient.get<MailConnectionStatus>("/mail/connection");
    setConnection(data);
    setSenders((data.rules?.senders ?? []).join(", "));
    setPrefixes((data.rules?.subject_prefixes ?? []).join(", "));
    setLabels((data.rules?.labels ?? []).join(", "));
  }, []);

  const loadThreads = useCallback(async () => {
    const { data } = await apiClient.get<PaginatedMailThreads>("/mail/threads", {
      params: { per_page: 50 },
    });
    setThreads(Array.isArray(data.data) ? data.data : []);
  }, []);

  const openThread = useCallback(async (id: string) => {
    setError("");
    try {
      const { data } = await apiClient.get<MailThreadDetail>(`/mail/threads/${id}`);
      setSelected(data);
      setDraftBody(data.draft?.body ?? "");
    } catch {
      setError("Could not load that thread.");
    }
  }, []);

  useEffect(() => {
    void (async () => {
      try {
        await loadConnection();
        await loadThreads();
      } catch {
        setError("Failed to load Mail Agent.");
      }
    })();
  }, [loadConnection, loadThreads]);

  useEffect(() => {
    const err = search.get("error");
    if (err) setError(err);
    if (search.get("connected") === "1") {
      void loadConnection();
    }
    const thread = search.get("thread");
    if (thread) void openThread(thread);
  }, [search, loadConnection, openThread]);

  const working =
    polling ||
    threads.some((t) => ["new", "classifying", "researching"].includes(t.status));
  useEffect(() => {
    if (!working) return;
    const id = window.setInterval(() => {
      void loadThreads();
    }, 3000);
    return () => window.clearInterval(id);
  }, [working, loadThreads]);

  async function connect() {
    setBusy(true);
    setError("");
    try {
      const { data } = await apiClient.post<{ authorization_url: string }>(
        "/mail/connection/oauth/start",
      );
      window.location.href = data.authorization_url;
    } catch (e: unknown) {
      const msg =
        (e as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        "Could not start Gmail connect.";
      setError(msg);
      setBusy(false);
    }
  }

  async function disconnect() {
    setBusy(true);
    setError("");
    try {
      await apiClient.delete("/mail/connection");
      await loadConnection();
      setThreads([]);
      setSelected(null);
    } catch {
      setError("Could not disconnect.");
    } finally {
      setBusy(false);
    }
  }

  async function syncNow() {
    setBusy(true);
    setError("");
    try {
      await apiClient.post("/mail/sync");
      setPolling(true);
      window.setTimeout(() => setPolling(false), 20000);
      await loadThreads();
      await loadConnection();
    } catch (e: unknown) {
      const msg =
        (e as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        "Sync failed.";
      setError(msg);
    } finally {
      setBusy(false);
    }
  }

  async function saveRules() {
    setBusy(true);
    setError("");
    try {
      const split = (s: string) =>
        s
          .split(",")
          .map((x) => x.trim())
          .filter(Boolean);
      await apiClient.patch("/mail/connection", {
        rules: {
          senders: split(senders),
          labels: split(labels),
          subject_prefixes: split(prefixes),
        },
      });
      await loadConnection();
    } catch {
      setError("Could not save rules.");
    } finally {
      setBusy(false);
    }
  }

  async function saveDraft() {
    if (!selected?.draft) return;
    setBusy(true);
    try {
      const { data } = await apiClient.patch<MailDraft>(
        `/mail/drafts/${selected.draft.id}`,
        { body: draftBody },
      );
      setSelected({ ...selected, draft: data });
    } catch {
      setError("Could not save the draft.");
    } finally {
      setBusy(false);
    }
  }

  async function approve() {
    if (!selected?.draft) return;
    setBusy(true);
    setError("");
    try {
      if (draftBody !== selected.draft.body) {
        await apiClient.patch(`/mail/drafts/${selected.draft.id}`, { body: draftBody });
      }
      await apiClient.post(`/mail/drafts/${selected.draft.id}/approve`);
      await openThread(selected.thread.id);
      await loadThreads();
    } catch (e: unknown) {
      const msg =
        (e as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        "Send failed.";
      setError(msg);
    } finally {
      setBusy(false);
    }
  }

  async function reject() {
    if (!selected?.draft) return;
    setBusy(true);
    try {
      await apiClient.post(`/mail/drafts/${selected.draft.id}/reject`);
      await openThread(selected.thread.id);
      await loadThreads();
    } catch {
      setError("Could not reject the draft.");
    } finally {
      setBusy(false);
    }
  }

  if (!connection) {
    return (
      <div className="rounded-lg border border-border bg-card p-6 text-center text-muted-foreground">
        Loading Mail Agent…
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {error && (
        <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      <div className="rounded-lg border border-border bg-card p-4 text-card-foreground">
        <h2 className="font-semibold">Gmail</h2>
        <p className="mt-1 text-xs text-muted-foreground">
          <strong className="text-foreground">Draft only.</strong> Approve
          (sends) or Reject — nothing is sent until a person clicks Approve.
        </p>
        {!connection.configured && (
          <p className="mt-2 text-sm text-muted-foreground">
            Gmail OAuth is not configured on this server. An operator needs to set
            GMAIL_CLIENT_ID, GMAIL_CLIENT_SECRET and GMAIL_TOKEN_KEY.
          </p>
        )}
        {connection.configured && !connection.connected && (
          <div className="mt-3 space-y-3">
            <p className="text-sm text-muted-foreground">
              Connect one shared organisation mailbox. The Mail Agent will draft
              replies; a person still has to approve before anything is sent.
            </p>
            <ul className="space-y-1 text-xs text-muted-foreground">
              {(connection.scopes_documented ?? []).map((s) => (
                <li key={s.scope}>
                  <span className="font-mono">{s.scope.split("/").pop()}</span>
                  {" — "}
                  {s.why}
                </li>
              ))}
            </ul>
            <button
              type="button"
              disabled={busy}
              onClick={() => void connect()}
              className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              Connect Gmail
            </button>
          </div>
        )}
        {connection.connected && (
          <div className="mt-3 space-y-3">
            <p className="text-sm">
              Connected as <span className="font-medium">{connection.email}</span>
            </p>
            {connection.last_sync_at && (
              <p className="text-xs text-muted-foreground">
                Last sync {new Date(connection.last_sync_at).toLocaleString()}
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                disabled={busy}
                onClick={() => void syncNow()}
                className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                {busy ? "Working…" : "Sync now"}
              </button>
              <button
                type="button"
                disabled={busy}
                onClick={() => void disconnect()}
                className="rounded-md border border-border px-3 py-1.5 text-sm hover:bg-muted disabled:opacity-50"
              >
                Disconnect
              </button>
            </div>
            <div className="grid gap-2 text-sm">
              <label className="text-xs text-muted-foreground">
                Watch senders (comma-separated, empty = all unread)
                <input
                  value={senders}
                  onChange={(e) => setSenders(e.target.value)}
                  className="mt-1 flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                />
              </label>
              <label className="text-xs text-muted-foreground">
                Subject prefixes
                <input
                  value={prefixes}
                  onChange={(e) => setPrefixes(e.target.value)}
                  className="mt-1 flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                />
              </label>
              <label className="text-xs text-muted-foreground">
                Gmail labels
                <input
                  value={labels}
                  onChange={(e) => setLabels(e.target.value)}
                  className="mt-1 flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                />
              </label>
              <button
                type="button"
                disabled={busy}
                onClick={() => void saveRules()}
                className="w-fit rounded-md border border-border px-3 py-1.5 text-xs hover:bg-muted"
              >
                Save rules
              </button>
            </div>
          </div>
        )}
      </div>

      {connection.connected && !selected && (
        <div className="rounded-lg border border-border bg-card p-4 text-card-foreground">
          <h2 className="font-semibold">Inbox</h2>
          <p className="mb-3 text-sm text-muted-foreground">
            Mail Agent is working on it when a row says classifying or researching.
          </p>
          {threads.length === 0 ? (
            <p className="text-sm text-muted-foreground">No threads yet. Sync now to pull unread mail.</p>
          ) : (
            <ul className="divide-y divide-border">
              {threads.map((t) => (
                <li key={t.id}>
                  <button
                    type="button"
                    onClick={() => void openThread(t.id)}
                    className="flex w-full flex-col items-start gap-0.5 py-3 text-left hover:bg-muted/40"
                  >
                    <span className="text-sm font-medium">{t.subject || "(no subject)"}</span>
                    <span className="text-xs text-muted-foreground">
                      {t.from_name || t.from_email} · {statusLabel(t.status)}
                    </span>
                    <span className="line-clamp-1 text-xs text-muted-foreground">{t.snippet}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {selected && (
        <div className="rounded-lg border border-border bg-card p-4 text-card-foreground">
          <button
            type="button"
            onClick={() => setSelected(null)}
            className="mb-3 text-sm font-medium text-primary hover:underline"
          >
            ← Back to inbox
          </button>
          <h2 className="text-lg font-semibold">{selected.thread.subject || "(no subject)"}</h2>
          <p className="text-sm text-muted-foreground">
            From {selected.thread.from_name || selected.thread.from_email} ·{" "}
            {statusLabel(selected.thread.status)}
          </p>
          {selected.thread.classification && (
            <p className="mt-2 text-sm">
              <span className="font-medium">Triage:</span>{" "}
              {selected.thread.classification.triage_label} — {selected.thread.classification.reason}
              {selected.thread.classification.needs_research ? " Research was requested." : ""}
            </p>
          )}
          <pre className="mt-3 max-h-48 overflow-auto whitespace-pre-wrap rounded-md bg-muted p-3 text-xs">
            {selected.thread.body_text || selected.thread.snippet}
          </pre>
          {selected.thread.research_summary && (
            <div className="mt-3 rounded-md border border-border p-3">
              <p className="text-xs font-semibold uppercase text-muted-foreground">Research</p>
              <p className="mt-1 text-sm">{selected.thread.research_summary}</p>
            </div>
          )}
          {selected.draft && (
            <div className="mt-4 space-y-2">
              <p className="text-sm font-medium">Draft reply to {selected.draft.to_email}</p>
              <textarea
                value={draftBody}
                onChange={(e) => setDraftBody(e.target.value)}
                disabled={selected.draft.status !== "draft"}
                rows={10}
                className="w-full rounded-md border border-input bg-background p-3 text-sm"
              />
              {selected.draft.status === "draft" && (
                <div className="flex flex-wrap gap-2">
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => void saveDraft()}
                    className="rounded-md border border-border px-3 py-1.5 text-sm hover:bg-muted"
                  >
                    Save draft
                  </button>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => void approve()}
                    className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                  >
                    Approve and send
                  </button>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => void reject()}
                    className="rounded-md border border-destructive/40 px-3 py-1.5 text-sm text-destructive hover:bg-destructive/10"
                  >
                    Reject
                  </button>
                </div>
              )}
              {selected.draft.status === "sent" && (
                <p className="text-sm text-emerald-600">Sent after approval.</p>
              )}
              {selected.draft.status === "rejected" && (
                <p className="text-sm text-muted-foreground">Rejected — not sent.</p>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
