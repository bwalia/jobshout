"use client";

import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { apiClient, apiErrorMessage } from "@/lib/api/client";
import {
  cancelSEORun,
  createSEORun,
  getSEORun,
  listSEORuns,
  type SEOIssue,
  type SEORun,
  type SEOSuggestion,
} from "@/lib/api/seo";

type Tab = "run" | "history";

const inputClass =
  "mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50";

interface AgentSummary {
  id: string;
  metadata?: { builtin?: string };
}

function severityClass(sev: string) {
  switch (sev) {
    case "critical":
    case "high":
      return "text-destructive";
    case "medium":
      return "text-amber-700 dark:text-amber-300";
    default:
      return "text-muted-foreground";
  }
}

function ScoreRing({ score }: { score: number }) {
  const tone =
    score >= 80 ? "text-emerald-600" : score >= 50 ? "text-amber-600" : "text-destructive";
  return (
    <div className={`flex h-16 w-16 flex-col items-center justify-center rounded-full border-2 border-current ${tone}`}>
      <span className="text-xl font-semibold leading-none">{score}</span>
      <span className="text-[10px] uppercase tracking-wide opacity-70">SEO</span>
    </div>
  );
}

export function SeoAgentClient() {
  const searchParams = useSearchParams();
  const runParam = searchParams.get("run");

  const [agentId, setAgentId] = useState("");
  const [loadError, setLoadError] = useState("");
  const [tab, setTab] = useState<Tab>("run");
  const [selected, setSelected] = useState<SEORun | null>(null);
  const [history, setHistory] = useState<SEORun[]>([]);
  const [detailError, setDetailError] = useState("");

  const [url, setUrl] = useState("");
  const [mode, setMode] = useState("analyze");
  const [platform, setPlatform] = useState("auto");
  const [keywords, setKeywords] = useState("");
  const [gitRepo, setGitRepo] = useState("");
  const [gitBranch, setGitBranch] = useState("main");
  const [sitemapPath, setSitemapPath] = useState("sitemap.xml");
  const [instruction, setInstruction] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState("");

  useEffect(() => {
    void (async () => {
      try {
        const { data } = await apiClient.get<{ data: AgentSummary[] }>("/agents", {
          params: { per_page: 100 },
        });
        const agents = Array.isArray(data.data) ? data.data : [];
        const agent = agents.find((a) => a.metadata?.builtin === "seo_analyst");
        if (!agent) {
          setLoadError("No SEO Analyst agent found for this organization.");
          return;
        }
        setAgentId(agent.id);
      } catch {
        setLoadError("Failed to load the SEO Analyst agent.");
      }
    })();
  }, []);

  useEffect(() => {
    if (!runParam) return;
    let cancelled = false;
    void (async () => {
      try {
        const run = await getSEORun(runParam);
        if (cancelled) return;
        setSelected(run);
        setTab("run");
      } catch {
        // History may still list it.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [runParam]);

  useEffect(() => {
    if (!selected || selected.status === "completed" || selected.status === "failed" || selected.status === "cancelled") {
      return;
    }
    let cancelled = false;
    const tick = async () => {
      try {
        const run = await getSEORun(selected.id);
        if (!cancelled) setSelected(run);
      } catch (e) {
        if (!cancelled) setDetailError(apiErrorMessage(e, "Failed to refresh run"));
      }
    };
    const id = window.setInterval(() => void tick(), 2000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [selected]);

  useEffect(() => {
    if (tab !== "history") return;
    let cancelled = false;
    void (async () => {
      try {
        const page = await listSEORuns(1, 20);
        if (!cancelled) setHistory(page.data ?? []);
      } catch (e) {
        if (!cancelled) toast.error(apiErrorMessage(e, "Failed to load runs"));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [tab]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!agentId) return;
    setFormError("");
    setSubmitting(true);
    try {
      const run = await createSEORun({
        agent_id: agentId,
        url: url.trim(),
        mode,
        platform,
        keywords: keywords.trim() || undefined,
        git_repo: gitRepo.trim() || undefined,
        git_branch: gitBranch.trim() || "main",
        sitemap_path: sitemapPath.trim() || "sitemap.xml",
        instruction: instruction.trim() || undefined,
      });
      setSelected(run);
      toast.success("SEO run queued");
    } catch (err) {
      setFormError(apiErrorMessage(err, "Failed to start SEO run"));
    } finally {
      setSubmitting(false);
    }
  };

  const onCancel = async () => {
    if (!selected) return;
    try {
      const run = await cancelSEORun(selected.id);
      setSelected(run);
      toast.success("Run cancelled");
    } catch (e) {
      toast.error(apiErrorMessage(e, "Cancel failed"));
    }
  };

  if (loadError) {
    return (
      <div className="p-4">
        <p className="text-sm text-destructive">{loadError}</p>
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4">
      <header className="space-y-1">
        <h2 className="text-lg font-semibold">SEO Analyst</h2>
        <p className="text-sm text-muted-foreground">
          Crawl a URL for title, meta, headings, robots, and sitemap health. Improve mode proposes
          slug/meta/sitemap fixes; publish applies via WordPress, OpsAPI, or a git PR when a repo is
          set.
        </p>
      </header>

      <div className="flex gap-2 border-b border-border pb-2">
        {(["run", "history"] as Tab[]).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setTab(t)}
            className={`rounded-md px-3 py-1.5 text-sm font-medium capitalize ${
              tab === t ? "bg-primary text-primary-foreground" : "hover:bg-muted"
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      {tab === "run" && (
        <div className="grid gap-6 lg:grid-cols-2">
          <form onSubmit={submit} className="space-y-3">
            <label className="block text-sm font-medium">
              Website URL
              <input
                className={inputClass}
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://example.com"
                required
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block text-sm font-medium">
                Mode
                <select className={inputClass} value={mode} onChange={(e) => setMode(e.target.value)}>
                  <option value="analyze">Analyse only</option>
                  <option value="improve">Analyse + improve plan</option>
                  <option value="publish">Improve + publish</option>
                </select>
              </label>
              <label className="block text-sm font-medium">
                CMS / site type
                <select
                  className={inputClass}
                  value={platform}
                  onChange={(e) => setPlatform(e.target.value)}
                >
                  <option value="auto">Auto-detect</option>
                  <option value="wordpress">WordPress</option>
                  <option value="opsapi">OpsAPI contents API</option>
                  <option value="static">Static / HTML (git)</option>
                </select>
              </label>
            </div>
            <label className="block text-sm font-medium">
              Focus keywords
              <input
                className={inputClass}
                value={keywords}
                onChange={(e) => setKeywords(e.target.value)}
                placeholder="devops, kubernetes, hiring"
              />
            </label>
            <label className="block text-sm font-medium">
              GitHub repo (PR fallback)
              <input
                className={inputClass}
                value={gitRepo}
                onChange={(e) => setGitRepo(e.target.value)}
                placeholder="owner/repo"
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block text-sm font-medium">
                Base branch
                <input
                  className={inputClass}
                  value={gitBranch}
                  onChange={(e) => setGitBranch(e.target.value)}
                />
              </label>
              <label className="block text-sm font-medium">
                Sitemap path
                <input
                  className={inputClass}
                  value={sitemapPath}
                  onChange={(e) => setSitemapPath(e.target.value)}
                />
              </label>
            </div>
            <label className="block text-sm font-medium">
              Note
              <textarea
                className={inputClass}
                rows={2}
                value={instruction}
                onChange={(e) => setInstruction(e.target.value)}
                placeholder="e.g. prefer UK English; keep brand voice"
              />
            </label>
            {formError && <p className="text-sm text-destructive">{formError}</p>}
            <button
              type="submit"
              disabled={!agentId || submitting}
              className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
            >
              {submitting ? "Starting…" : "Run SEO analysis"}
            </button>
          </form>

          <div className="space-y-3">
            {!selected && (
              <p className="text-sm text-muted-foreground">Results appear here after you start a run.</p>
            )}
            {selected && (
              <RunDetail
                run={selected}
                detailError={detailError}
                onCancel={
                  selected.status === "queued" || selected.status === "running" ? onCancel : undefined
                }
              />
            )}
          </div>
        </div>
      )}

      {tab === "history" && (
        <ul className="divide-y divide-border rounded-md border border-border">
          {history.length === 0 && (
            <li className="px-3 py-4 text-sm text-muted-foreground">No runs yet.</li>
          )}
          {history.map((r) => (
            <li key={r.id}>
              <button
                type="button"
                className="flex w-full items-center justify-between gap-3 px-3 py-2.5 text-left text-sm hover:bg-muted/50"
                onClick={() => {
                  setSelected(r);
                  setTab("run");
                }}
              >
                <span className="truncate font-medium">{r.url}</span>
                <span className="shrink-0 text-xs text-muted-foreground">
                  {r.mode} · {r.status}
                  {r.score ? ` · ${r.score.overall}` : ""}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function RunDetail({
  run,
  detailError,
  onCancel,
}: {
  run: SEORun;
  detailError: string;
  onCancel?: () => void;
}) {
  const busy = run.status === "queued" || run.status === "running";
  const report = run.report;
  const issues: SEOIssue[] = report?.issues ?? [];
  const suggestions: SEOSuggestion[] = report?.suggestions ?? [];

  return (
    <div className="space-y-4 rounded-xl border border-border p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <p className="truncate text-sm font-medium">{run.url}</p>
          <p className="text-xs text-muted-foreground">
            {run.mode} · {run.status}
            {run.detected_cms ? ` · detected ${run.detected_cms}` : ""}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {busy && <Loader2 className="h-4 w-4 animate-spin text-primary" />}
          {run.score && <ScoreRing score={run.score.overall} />}
          {onCancel && (
            <button
              type="button"
              onClick={onCancel}
              className="rounded-md border border-border px-2 py-1 text-xs hover:bg-muted"
            >
              Cancel
            </button>
          )}
        </div>
      </div>

      {detailError && <p className="text-sm text-destructive">{detailError}</p>}
      {run.error_message && <p className="text-sm text-destructive">{run.error_message}</p>}

      {report && (
        <dl className="grid gap-2 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-xs text-muted-foreground">Title</dt>
            <dd className="break-words">{report.title || "—"}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Meta description</dt>
            <dd className="break-words">{report.meta_description || "—"}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Canonical</dt>
            <dd className="break-all text-xs">{report.canonical || "—"}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Sitemap</dt>
            <dd className="text-xs">
              {report.sitemap_found ? report.sitemap_url || "found" : "missing"}
            </dd>
          </div>
          {report.proposed_slug && (
            <div className="sm:col-span-2">
              <dt className="text-xs text-muted-foreground">Proposed slug</dt>
              <dd className="font-mono text-xs">{report.proposed_slug}</dd>
            </div>
          )}
        </dl>
      )}

      {issues.length > 0 && (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold">Issues</h3>
          <ul className="space-y-2">
            {issues.map((i) => (
              <li key={i.id} className="rounded-md border border-border px-3 py-2 text-sm">
                <p className={`font-medium ${severityClass(i.severity)}`}>
                  {i.severity}: {i.title}
                </p>
                <p className="text-muted-foreground">{i.detail}</p>
                {i.fix_hint && <p className="mt-1 text-xs">Fix: {i.fix_hint}</p>}
              </li>
            ))}
          </ul>
        </section>
      )}

      {suggestions.length > 0 && (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold">Suggestions</h3>
          <ul className="space-y-2">
            {suggestions.map((s) => (
              <li key={s.id} className="rounded-md border border-border px-3 py-2 text-sm">
                <p className="font-medium">
                  [{s.kind}] {s.title}
                </p>
                <p className="text-muted-foreground">{s.detail}</p>
                {s.proposed && (
                  <pre className="mt-1 overflow-x-auto rounded bg-muted/50 p-2 text-xs">{s.proposed}</pre>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}

      {run.publish && (
        <section className="rounded-md border border-border bg-muted/30 px-3 py-2 text-sm">
          <p className="font-medium">
            Publish · {run.publish.channel} · {run.publish.success ? "ok" : "failed"}
          </p>
          <p className="text-muted-foreground">{run.publish.message}</p>
          {run.publish.pr_url && (
            <a
              className="mt-1 inline-block underline underline-offset-2"
              href={run.publish.pr_url}
              target="_blank"
              rel="noreferrer"
            >
              Open pull request #{run.publish.pr_number || ""}
            </a>
          )}
        </section>
      )}
    </div>
  );
}
