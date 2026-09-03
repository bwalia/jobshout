"use client";

import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { FieldHintProvider, FieldLabel } from "@/components/ui/field-hint";
import { TagInput } from "@/components/ui/TagInput";
import { apiClient, apiErrorMessage } from "@/lib/api/client";
import type {
  MailConnectionStatus,
  MailDraft,
  MailDraftIgnoredResult,
  MailSyncResult,
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

// statusBadge maps a thread status to an at-a-glance pill. Working states
// pulse, a ready draft is green, ignored mail is dimmed so the rows the
// agent acted on stand out.
function statusBadge(status: string): {
  label: string;
  className: string;
  dim: boolean;
} {
  const base = "shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium";
  switch (status) {
    case "new":
      return {
        label: "Queued",
        className: `${base} animate-pulse bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400`,
        dim: false,
      };
    case "classifying":
      return {
        label: "Drafting…",
        className: `${base} animate-pulse bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400`,
        dim: false,
      };
    case "researching":
      return {
        label: "Researching…",
        className: `${base} animate-pulse bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400`,
        dim: false,
      };
    case "draft_ready":
      return {
        label: "Draft ready",
        className: `${base} bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-400`,
        dim: false,
      };
    case "sent":
      return {
        label: "Sent",
        className: `${base} bg-sky-100 text-sky-800 dark:bg-sky-900/30 dark:text-sky-400`,
        dim: false,
      };
    case "rejected":
      return {
        label: "Rejected",
        className: `${base} bg-rose-100 text-rose-800 dark:bg-rose-900/30 dark:text-rose-400`,
        dim: true,
      };
    case "ignored":
      return {
        label: "Ignored",
        className: `${base} bg-muted text-muted-foreground`,
        dim: true,
      };
    case "failed":
      return {
        label: "Failed",
        className: `${base} bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400`,
        dim: false,
      };
    default:
      return {
        label: statusLabel(status),
        className: `${base} bg-muted text-muted-foreground`,
        dim: false,
      };
  }
}

export function MailAgentClient() {
  const search = useSearchParams();
  const [connection, setConnection] = useState<MailConnectionStatus | null>(null);
  const [threads, setThreads] = useState<MailThread[]>([]);
  const [selected, setSelected] = useState<MailThreadDetail | null>(null);
  const [draftBody, setDraftBody] = useState("");
  const [error, setError] = useState("");
  const [syncNote, setSyncNote] = useState("");
  const [saveNote, setSaveNote] = useState("");
  const [watchNote, setWatchNote] = useState("");
  const [polling, setPolling] = useState(false);
  const [busy, setBusy] = useState(false);
  const [savingRules, setSavingRules] = useState(false);
  const [senders, setSenders] = useState<string[]>([]);
  const [prefixes, setPrefixes] = useState<string[]>([]);
  const [labels, setLabels] = useState<string[]>([]);
  const [knowledgeNotes, setKnowledgeNotes] = useState("");
  const [knowledgeUrls, setKnowledgeUrls] = useState("");
  const [researchFocus, setResearchFocus] = useState("");
  const [replyInstructions, setReplyInstructions] = useState("");

  const loadConnection = useCallback(async () => {
    const { data } = await apiClient.get<MailConnectionStatus>("/mail/connection");
    setConnection(data);
    setSenders(data.rules?.senders ?? []);
    setPrefixes(data.rules?.subject_prefixes ?? []);
    setLabels(data.rules?.labels ?? []);
    setKnowledgeNotes(data.knowledge_notes ?? "");
    setKnowledgeUrls((data.knowledge_urls ?? []).join("\n"));
    setResearchFocus(data.research_focus ?? "");
    setReplyInstructions(data.reply_instructions ?? "");
  }, []);

  const loadThreads = useCallback(async () => {
    const { data } = await apiClient.get<PaginatedMailThreads>("/mail/threads", {
      params: { per_page: 50 },
    });
    setThreads(Array.isArray(data.data) ? data.data : []);
  }, []);

  const openThread = useCallback(async (id: string) => {
    setError("");
    setWatchNote("");
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
    setSyncNote("");
    try {
      const { data } = await apiClient.post<MailSyncResult>("/mail/sync");
      if (data.listed === 0) {
        setSyncNote(
          `Gmail returned no conversations for: ${data.query || "in:inbox newer_than:7d"}. ` +
            "Clear watch senders, labels, or subject prefixes if you expected mail, then Sync now again.",
        );
      } else if (data.ingested === 0) {
        setSyncNote(
          `Gmail listed ${data.listed} conversation(s); all were already in this inbox.`,
        );
      } else {
        setSyncNote(
          `Pulled ${data.ingested} new conversation(s) from Gmail. Drafts appear as the agent finishes each one.`,
        );
      }
      setPolling(true);
      window.setTimeout(() => setPolling(false), 20000);
      await loadThreads();
    } catch (e: unknown) {
      const msg =
        (e as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        "Sync failed.";
      setError(msg);
    } finally {
      try {
        await loadConnection();
      } catch {
        /* keep the sync error visible */
      }
      setBusy(false);
    }
  }

  async function saveRules() {
    setBusy(true);
    setSavingRules(true);
    setError("");
    setSaveNote("");
    try {
      await apiClient.patch("/mail/connection", {
        rules: {
          senders,
          labels,
          subject_prefixes: prefixes,
        },
        knowledge_notes: knowledgeNotes.trim(),
        knowledge_urls: knowledgeUrls
          .split("\n")
          .map((x) => x.trim())
          .filter(Boolean),
        research_focus: researchFocus,
        reply_instructions: replyInstructions,
      });
      await loadConnection();
      setSaveNote("Rules saved. The next Sync now will use these filters and playbook.");
    } catch (e: unknown) {
      setError(apiErrorMessage(e, "Could not save rules."));
    } finally {
      setSavingRules(false);
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

  function rememberSender(email: string) {
    const next = email.trim();
    if (!next) return;
    setSenders((prev) => {
      if (prev.some((s) => s.toLowerCase() === next.toLowerCase())) {
        return prev;
      }
      return [...prev, next];
    });
  }

  async function draftIgnored() {
    if (!selected) return;
    setBusy(true);
    setError("");
    setSaveNote("");
    setWatchNote("");
    const from = (selected.thread.from_email || selected.thread.from_name || "").trim();
    try {
      const { data } = await apiClient.post<MailDraftIgnoredResult>(
        `/mail/threads/${selected.thread.id}/draft`,
      );
      const watched = (data.watched_sender || from).trim();
      if (watched) {
        rememberSender(watched);
      }
      setSelected({ thread: data.thread, draft: data.draft });
      setDraftBody(data.draft?.body ?? "");
      await loadThreads();
      if (watched) {
        const note = `Watching ${watched} — similar mail won’t be ignored. Nothing is sent until you Approve.`;
        setSaveNote(note);
        setWatchNote(note);
      }
    } catch (e: unknown) {
      setError(apiErrorMessage(e, "Could not draft a reply."));
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

  const mailboxLinked =
    connection.connected ||
    (connection.status !== "disconnected" && Boolean(connection.email));

  return (
    <FieldHintProvider>
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
        {connection.configured && !mailboxLinked && (
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
        {mailboxLinked && (
          <div className="mt-3 space-y-3">
            <p className="text-sm">
              Connected as <span className="font-medium">{connection.email}</span>
            </p>
            {connection.status === "error" && connection.status_error ? (
              <p className="text-sm text-destructive">{connection.status_error}</p>
            ) : null}
            {syncNote ? (
              <p className="text-sm text-muted-foreground">{syncNote}</p>
            ) : null}
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
            <div className="grid gap-3 text-sm">
              <div>
                <FieldLabel
                  htmlFor="mail-senders"
                  label="Watch senders"
                  hint="Emails or display names to watch. Type one and press Enter. Empty = recent inbox."
                />
                <TagInput
                  id="mail-senders"
                  value={senders}
                  onChange={setSenders}
                  disabled={busy}
                  size="lg"
                  placeholder="ops@example.com"
                />
              </div>
              <div>
                <FieldLabel
                  htmlFor="mail-prefixes"
                  label="Subject prefixes"
                  hint="Only mail whose subject starts with one of these. Type a prefix and press Enter."
                />
                <TagInput
                  id="mail-prefixes"
                  value={prefixes}
                  onChange={setPrefixes}
                  disabled={busy}
                  placeholder="[support]"
                />
              </div>
              <div>
                <FieldLabel
                  htmlFor="mail-labels"
                  label="Gmail labels"
                  hint="Gmail labels to include. Type a label and press Enter."
                />
                <TagInput
                  id="mail-labels"
                  value={labels}
                  onChange={setLabels}
                  disabled={busy}
                  placeholder="INBOX"
                />
              </div>
              <label className="text-xs text-muted-foreground">
                What should the agent know when replying?
                <textarea
                  value={knowledgeNotes}
                  onChange={(e) => setKnowledgeNotes(e.target.value)}
                  rows={8}
                  placeholder={
                    "Mac Studio M5 Max: $2,499\nMac Studio M5 Ultra: $5,499\nRefunds within 30 days, shipping 3–5 working days…"
                  }
                  className="mt-1 w-full min-h-[8rem] resize-y rounded-md border border-input bg-background p-3 font-mono text-sm"
                />
                <span className="mt-1 block text-[11px]">
                  Prices, products, policies — plain text or markdown. Replies
                  quote only what is written here; anything missing gets an
                  honest &quot;we&apos;ll follow up&quot;.
                </span>
              </label>
              <label className="text-xs text-muted-foreground">
                Knowledge links (optional, one URL per line)
                <textarea
                  value={knowledgeUrls}
                  onChange={(e) => setKnowledgeUrls(e.target.value)}
                  rows={2}
                  placeholder="https://example.com/pricing"
                  className="mt-1 w-full rounded-md border border-input bg-background p-3 text-sm"
                />
              </label>
              <label className="text-xs text-muted-foreground">
                What to look for in those pages
                <textarea
                  value={researchFocus}
                  onChange={(e) => setResearchFocus(e.target.value)}
                  rows={2}
                  placeholder="Prices, SLA, refund window…"
                  className="mt-1 w-full rounded-md border border-input bg-background p-3 text-sm"
                />
              </label>
              <label className="text-xs text-muted-foreground">
                How the reply should read
                <textarea
                  value={replyInstructions}
                  onChange={(e) => setReplyInstructions(e.target.value)}
                  rows={2}
                  placeholder="Tone, length, must-include, must-avoid"
                  className="mt-1 w-full rounded-md border border-input bg-background p-3 text-sm"
                />
              </label>
              <p className="text-xs text-muted-foreground">
                Your notes are the source of truth for drafts. Links are
                optional extra pages researched on top of them; with neither,
                drafts fall back to open-web research.
              </p>
              <div className="flex flex-wrap items-center gap-3">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void saveRules()}
                  className="w-fit rounded-md border border-border px-3 py-1.5 text-xs hover:bg-muted disabled:opacity-50"
                >
                  {savingRules ? "Saving…" : "Save rules"}
                </button>
                {saveNote ? (
                  <p className="text-xs text-emerald-700 dark:text-emerald-400">{saveNote}</p>
                ) : null}
              </div>
            </div>
          </div>
        )}
      </div>

      {mailboxLinked && !selected && (
        <div className="rounded-lg border border-border bg-card p-4 text-card-foreground">
          <h2 className="font-semibold">Inbox</h2>
          <p className="mb-3 text-sm text-muted-foreground">
            An amber badge means the agent is still working on that mail; green
            means a draft is ready for review. Ignored mail is dimmed — open it
            to draft a reply and watch that sender.
          </p>
          {threads.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No threads yet. Sync now pulls the last 7 days of inbox mail
              matching your watch rules. Leave senders, labels, and subject
              prefixes empty to pull everything recent.
            </p>
          ) : (
            <ul className="divide-y divide-border">
              {threads.map((t) => {
                const badge = statusBadge(t.status);
                return (
                  <li key={t.id}>
                    <button
                      type="button"
                      onClick={() => void openThread(t.id)}
                      className={`flex w-full flex-col items-start gap-0.5 py-3 text-left hover:bg-muted/40 ${
                        badge.dim ? "opacity-55" : ""
                      }`}
                    >
                      <span className="flex w-full items-center justify-between gap-2">
                        <span className="truncate text-sm font-medium">
                          {t.subject || "(no subject)"}
                        </span>
                        <span className={badge.className}>{badge.label}</span>
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {t.from_name || t.from_email}
                      </span>
                      <span className="line-clamp-1 text-xs text-muted-foreground">{t.snippet}</span>
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      )}

      {selected && (
        <div className="rounded-lg border border-border bg-card p-4 text-card-foreground">
          <button
            type="button"
            onClick={() => {
              setSelected(null);
              setWatchNote("");
            }}
            className="mb-3 text-sm font-medium text-primary hover:underline"
          >
            ← Back to inbox
          </button>
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-semibold">{selected.thread.subject || "(no subject)"}</h2>
            <span className={statusBadge(selected.thread.status).className}>
              {statusBadge(selected.thread.status).label}
            </span>
          </div>
          <p className="text-sm text-muted-foreground">
            From {selected.thread.from_name || selected.thread.from_email}
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
          {watchNote ? (
            <p className="mt-3 text-sm text-emerald-700 dark:text-emerald-400">{watchNote}</p>
          ) : null}
          {!selected.draft &&
            (selected.thread.status === "ignored" || selected.thread.status === "failed") && (
            <div className="mt-4 space-y-2">
              {selected.thread.status === "ignored" ? (
                <p className="text-sm text-muted-foreground">
                  The agent skipped this mail. Draft a reply to watch this sender
                  — nothing is sent until you Approve.
                </p>
              ) : (
                <p className="text-sm text-muted-foreground">
                  Drafting failed
                  {selected.thread.error_message
                    ? `: ${selected.thread.error_message}.`
                    : "."}{" "}
                  Try again — nothing is sent until you Approve.
                </p>
              )}
              <button
                type="button"
                disabled={busy}
                onClick={() => void draftIgnored()}
                className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                {busy ? "Drafting…" : "Draft reply"}
              </button>
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
    </FieldHintProvider>
  );
}
