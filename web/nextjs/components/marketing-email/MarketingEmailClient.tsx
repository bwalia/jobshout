"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams, useSearchParams } from "next/navigation";
import {
  AlertTriangle,
  Archive,
  CheckCircle2,
  Copy,
  ExternalLink,
  FileStack,
  Loader2,
  Mail,
  RefreshCw,
  Smartphone,
  Monitor,
} from "lucide-react";

import { newTaskHref } from "@/lib/agents/links";
import {
  useMarketingEmailRun,
  useMarketingEmailRuns,
  useRegenerateMarketingEmail,
  useSaveMarketingEmailTemplate,
  useUnpublishMarketingEmail,
} from "@/lib/hooks/useMarketingEmails";
import {
  isMarketingEmailRunActive,
  type MarketingEmailRun,
  type MarketingEmailRunStatus,
} from "@/lib/types/marketing-email";
import { cn } from "@/lib/utils/cn";

const STATUS_META: Record<MarketingEmailRunStatus, { label: string; className: string }> = {
  writing: { label: "Writing", className: "bg-signal-live/10 text-signal-live" },
  publishing: { label: "Publishing", className: "bg-signal/10 text-signal" },
  published: { label: "Live", className: "bg-status-done/10 text-status-done" },
  draft: { label: "Draft", className: "bg-muted text-muted-foreground" },
  failed: { label: "Failed", className: "bg-signal-error/10 text-signal-error" },
  archived: { label: "Archived", className: "bg-muted text-muted-foreground" },
};

function StatusBadge({ status }: { status: MarketingEmailRunStatus }) {
  const meta = STATUS_META[status] ?? STATUS_META.draft;
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-2xs font-medium",
        meta.className
      )}
    >
      {isMarketingEmailRunActive(status) ? (
        <Loader2 className="h-3 w-3 animate-spin" />
      ) : status === "published" ? (
        <CheckCircle2 className="h-3 w-3" />
      ) : status === "failed" ? (
        <AlertTriangle className="h-3 w-3" />
      ) : null}
      {meta.label}
    </span>
  );
}

async function copyText(label: string, text: string) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    throw new Error(`Could not copy ${label}`);
  }
}

function PreviewFrame({ html, width }: { html: string; width: number }) {
  return (
    <div className="overflow-hidden rounded-lg border border-border bg-white">
      <div className="border-b border-border bg-muted/40 px-3 py-1.5 text-2xs text-muted-foreground">
        {width}px
      </div>
      <div className="overflow-auto bg-[#f4f4f5] p-3" style={{ maxHeight: 520 }}>
        <iframe
          title={`Email preview ${width}px`}
          sandbox=""
          srcDoc={html}
          className="mx-auto block border-0 bg-white shadow-sm"
          style={{ width, minHeight: 400, height: 480 }}
        />
      </div>
    </div>
  );
}

/**
 * The "Save as mail-merge template" outcome: the same email saved to OpsAPI
 * Marketing (Studio) with real merge fields, as a draft template.
 */
function TemplateOutcome({
  run,
  busy,
  onCopied,
}: {
  run: MarketingEmailRun;
  busy: boolean;
  onCopied: (msg: string) => void;
}) {
  const save = useSaveMarketingEmailTemplate();
  if (!run.slug) return null;
  const saved = run.template_outcome === "saved";
  return (
    <div className="space-y-2 rounded-lg border border-border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm font-medium text-foreground">
          <FileStack className="h-4 w-4" />
          Save as mail-merge template
          {saved ? (
            <span className="inline-flex items-center gap-1 rounded-full bg-status-done/10 px-2 py-0.5 text-2xs font-medium text-status-done">
              <CheckCircle2 className="h-3 w-3" /> Saved
            </span>
          ) : run.template_outcome ? (
            <span className="inline-flex items-center gap-1 rounded-full bg-signal-error/10 px-2 py-0.5 text-2xs font-medium text-signal-error">
              <AlertTriangle className="h-3 w-3" /> Not saved
            </span>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          {run.template_html ? (
            <button
              type="button"
              className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-border px-3 text-xs font-medium hover:bg-muted"
              onClick={() =>
                void copyText("mail-merge HTML", run.template_html!).then(() =>
                  onCopied("Mail-merge HTML copied")
                )
              }
            >
              <Copy className="h-3.5 w-3.5" />
              Copy mail-merge HTML
            </button>
          ) : null}
          <button
            type="button"
            disabled={busy || save.isPending}
            onClick={() => save.mutate(run.id)}
            className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-border px-3 text-xs font-medium hover:bg-muted disabled:opacity-50"
          >
            {save.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <FileStack className="h-3.5 w-3.5" />}
            {saved ? "Save again" : "Save template"}
          </button>
        </div>
      </div>
      {run.template_message ? (
        <p className={cn("text-xs", saved ? "text-muted-foreground" : "text-signal-error")}>
          {run.template_message}
        </p>
      ) : (
        <p className="text-xs text-muted-foreground">
          Saves this email to OpsAPI Marketing as a draft template with {"{{contact.first_name}}"},
          unsubscribe, preferences and view-in-browser merge fields.
        </p>
      )}
      {run.template_uuid ? (
        <p className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <span>
            {run.template_name ? `${run.template_name} · ` : ""}template{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-2xs">{run.template_uuid}</code>
          </span>
          <button
            type="button"
            className="text-signal-live hover:underline"
            onClick={() =>
              void copyText("template id", run.template_uuid!).then(() => onCopied("Template id copied"))
            }
          >
            Copy id
          </button>
          <span>— open it in the OpsAPI dashboard under Marketing → Studio.</span>
        </p>
      ) : null}
    </div>
  );
}

function RunDetail({
  runId,
  agentId,
}: {
  runId: string;
  agentId?: string;
}) {
  const { data: run, isLoading } = useMarketingEmailRun(runId);
  const regenerate = useRegenerateMarketingEmail();
  const unpublish = useUnpublishMarketingEmail();
  const [viewport, setViewport] = useState<"desktop" | "mobile">("desktop");
  const [copyMsg, setCopyMsg] = useState<string | null>(null);

  useEffect(() => {
    if (!copyMsg) return;
    const t = setTimeout(() => setCopyMsg(null), 2000);
    return () => clearTimeout(t);
  }, [copyMsg]);

  if (isLoading || !run) {
    return (
      <div className="flex items-center gap-2 p-6 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading email…
      </div>
    );
  }

  const html = run.content_html ?? "";
  const busy = isMarketingEmailRunActive(run.status);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="truncate text-base font-semibold text-foreground">
              {run.subject || run.title || "Untitled email"}
            </h2>
            <StatusBadge status={run.status} />
          </div>
          {run.preheader ? (
            <p className="text-sm text-muted-foreground">{run.preheader}</p>
          ) : null}
          {run.message ? <p className="text-xs text-muted-foreground">{run.message}</p> : null}
          {run.error ? <p className="text-sm text-signal-error">{run.error}</p> : null}
        </div>
        <div className="flex flex-wrap gap-2">
          <Link
            href={newTaskHref(agentId)}
            className="inline-flex h-8 items-center rounded-lg border border-border px-3 text-xs font-medium hover:bg-muted"
          >
            New email
          </Link>
          <button
            type="button"
            disabled={busy || regenerate.isPending}
            onClick={() => regenerate.mutate(run.id)}
            className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-border px-3 text-xs font-medium hover:bg-muted disabled:opacity-50"
          >
            <RefreshCw className={cn("h-3.5 w-3.5", regenerate.isPending && "animate-spin")} />
            Re-generate
          </button>
          {(run.status === "published" || run.status === "draft") && run.page_uuid ? (
            <button
              type="button"
              disabled={unpublish.isPending}
              onClick={() => {
                if (window.confirm("Archive this email’s CMS page?")) unpublish.mutate(run.id);
              }}
              className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-border px-3 text-xs font-medium hover:bg-muted disabled:opacity-50"
            >
              <Archive className="h-3.5 w-3.5" />
              Unpublish
            </button>
          ) : null}
        </div>
      </div>

      <div className="flex flex-wrap gap-2">
        {run.browser_url ? (
          <>
            <a
              href={run.browser_url}
              target="_blank"
              rel="noreferrer"
              className="inline-flex h-8 items-center gap-1.5 rounded-lg bg-foreground px-3 text-xs font-medium text-background hover:opacity-90"
            >
              <ExternalLink className="h-3.5 w-3.5" />
              Open live page
            </a>
            <button
              type="button"
              className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-border px-3 text-xs font-medium hover:bg-muted"
              onClick={() =>
                void copyText("browser link", run.browser_url!).then(() =>
                  setCopyMsg("Browser link copied")
                )
              }
            >
              <Copy className="h-3.5 w-3.5" />
              Copy browser-version link
            </button>
          </>
        ) : null}
        {html ? (
          <button
            type="button"
            className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-border px-3 text-xs font-medium hover:bg-muted"
            onClick={() =>
              void copyText("HTML", html).then(() => setCopyMsg("HTML copied — paste into your ESP"))
            }
          >
            <Copy className="h-3.5 w-3.5" />
            Copy HTML
          </button>
        ) : null}
        {copyMsg ? <span className="self-center text-2xs text-status-done">{copyMsg}</span> : null}
      </div>

      <TemplateOutcome run={run} busy={busy} onCopied={setCopyMsg} />

      {html ? (
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setViewport("desktop")}
              className={cn(
                "inline-flex h-8 items-center gap-1.5 rounded-lg px-3 text-xs font-medium",
                viewport === "desktop" ? "bg-muted" : "hover:bg-muted/60"
              )}
            >
              <Monitor className="h-3.5 w-3.5" /> 600px
            </button>
            <button
              type="button"
              onClick={() => setViewport("mobile")}
              className={cn(
                "inline-flex h-8 items-center gap-1.5 rounded-lg px-3 text-xs font-medium",
                viewport === "mobile" ? "bg-muted" : "hover:bg-muted/60"
              )}
            >
              <Smartphone className="h-3.5 w-3.5" /> Mobile
            </button>
          </div>
          <PreviewFrame html={html} width={viewport === "desktop" ? 600 : 375} />
        </div>
      ) : busy ? (
        <div className="flex items-center gap-2 rounded-lg border border-dashed border-border p-8 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" /> Writing and publishing…
        </div>
      ) : null}
    </div>
  );
}

function RunRow({
  run,
  selected,
  onSelect,
}: {
  run: MarketingEmailRun;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        "w-full rounded-lg border px-3 py-2.5 text-left transition-colors",
        selected ? "border-foreground/30 bg-muted/50" : "border-transparent hover:bg-muted/40"
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">
            {run.subject || run.title || "Untitled"}
          </p>
          <p className="mt-0.5 truncate text-2xs text-muted-foreground">
            {run.updated_at ? new Date(run.updated_at).toLocaleString() : ""}
            {run.slug ? ` · ${run.slug}` : ""}
          </p>
        </div>
        <StatusBadge status={run.status} />
      </div>
    </button>
  );
}

export function MarketingEmailClient() {
  const params = useParams<{ id?: string }>();
  const search = useSearchParams();
  const agentId = params?.id;
  const { data: runs = [], isLoading } = useMarketingEmailRuns();
  const runFromQuery = search.get("run");
  const [selectedId, setSelectedId] = useState<string | null>(runFromQuery);

  useEffect(() => {
    if (runFromQuery) setSelectedId(runFromQuery);
  }, [runFromQuery]);

  const selected = useMemo(() => {
    if (selectedId) return selectedId;
    return runs[0]?.id ?? null;
  }, [selectedId, runs]);

  return (
    <div className="flex h-full min-h-[28rem] flex-col gap-4 lg:flex-row">
      <aside className="flex w-full shrink-0 flex-col gap-2 lg:w-72">
        <div className="flex items-center justify-between gap-2 px-1">
          <div className="flex items-center gap-2 text-sm font-medium">
            <Mail className="h-4 w-4" />
            Emails
          </div>
          <Link
            href={newTaskHref(agentId)}
            className="text-2xs font-medium text-signal-live hover:underline"
          >
            New
          </Link>
        </div>
        {isLoading ? (
          <div className="flex items-center gap-2 px-1 text-xs text-muted-foreground">
            <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading…
          </div>
        ) : runs.length === 0 ? (
          <p className="px-1 text-xs text-muted-foreground">
            No emails yet. Start one from New task with a brief.
          </p>
        ) : (
          <div className="max-h-[70vh] space-y-1 overflow-y-auto">
            {runs.map((run) => (
              <RunRow
                key={run.id}
                run={run}
                selected={run.id === selected}
                onSelect={() => setSelectedId(run.id)}
              />
            ))}
          </div>
        )}
      </aside>
      <main className="min-w-0 flex-1 rounded-xl border border-border bg-background p-4">
        {selected ? (
          <RunDetail runId={selected} agentId={agentId} />
        ) : (
          <div className="flex h-full min-h-48 flex-col items-center justify-center gap-2 text-center text-sm text-muted-foreground">
            <Mail className="h-8 w-8 opacity-40" />
            <p>Pick an email or start a new one.</p>
            <Link href={newTaskHref(agentId)} className="text-signal-live hover:underline">
              New marketing email
            </Link>
          </div>
        )}
      </main>
    </div>
  );
}
