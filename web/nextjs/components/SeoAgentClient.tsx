"use client";

import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { useParams, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { apiClient, apiErrorMessage } from "@/lib/api/client";
import {
  cancelSEORun,
  createSEORun,
  deleteSEOConnection,
  getSEORun,
  getSEOStatus,
  listSEOConnections,
  listSEORuns,
  upsertSEOConnection,
  type SEOGitHubConnection,
  type SEOIssue,
  type SEOPageReport,
  type SEORun,
  type SEOStatus,
  type SEOSuggestion,
} from "@/lib/api/seo";

type Tab = "run" | "history" | "connection";

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
    <div
      className={`flex h-16 w-16 flex-col items-center justify-center rounded-full border-2 border-current ${tone}`}
    >
      <span className="text-xl font-semibold leading-none">{score}</span>
      <span className="text-2xs uppercase tracking-wide opacity-70">SEO</span>
    </div>
  );
}

export function SeoAgentClient() {
  const searchParams = useSearchParams();
  const runParam = searchParams.get("run");
  const pageAgentId = useParams<{ id?: string }>()?.id;

  const [agentId, setAgentId] = useState("");
  const [loadError, setLoadError] = useState("");
  const [tab, setTab] = useState<Tab>("run");
  const [selected, setSelected] = useState<SEORun | null>(null);
  const [history, setHistory] = useState<SEORun[]>([]);
  const [detailError, setDetailError] = useState("");
  const [status, setStatus] = useState<SEOStatus | null>(null);
  const [connections, setConnections] = useState<SEOGitHubConnection[]>([]);

  const [source, setSource] = useState<"urls" | "github">("urls");
  const [urls, setUrls] = useState("");
  const [mode, setMode] = useState("analyze");
  const [platform, setPlatform] = useState("auto");
  const [keywords, setKeywords] = useState("");
  const [githubConnection, setGithubConnection] = useState("");
  const [gitRepo, setGitRepo] = useState("");
  const [gitBranch, setGitBranch] = useState("main");
  const [pathGlob, setPathGlob] = useState("content/**/*.md");
  const [maxPages, setMaxPages] = useState("10");
  const [sitemapPath, setSitemapPath] = useState("sitemap.xml");
  const [instruction, setInstruction] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState("");

  const [connName, setConnName] = useState("");
  const [connRepo, setConnRepo] = useState("");
  const [connBranch, setConnBranch] = useState("main");
  const [connToken, setConnToken] = useState("");
  const [savingConn, setSavingConn] = useState(false);

  useEffect(() => {
    if (pageAgentId) {
      setAgentId(pageAgentId);
      return;
    }
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
  }, [pageAgentId]);

  useEffect(() => {
    void (async () => {
      try {
        const [st, conns] = await Promise.all([getSEOStatus(), listSEOConnections()]);
        setStatus(st);
        setConnections(conns);
        setGithubConnection((prev) => prev || conns[0]?.name || "");
      } catch {
        // Status/connections are optional on older rings.
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
    if (
      !selected ||
      selected.status === "completed" ||
      selected.status === "failed" ||
      selected.status === "cancelled"
    ) {
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
    if (!keywords.trim()) {
      setFormError("Focus keywords are required.");
      return;
    }
    if (source === "urls" && !urls.trim()) {
      setFormError("Add at least one URL.");
      return;
    }
    if (source === "github" && !githubConnection.trim()) {
      setFormError("Pick a named GitHub connection (Workspace → Connection).");
      return;
    }
    if (
      mode === "publish" &&
      !window.confirm(
        source === "github"
          ? "Open a pull request with proposed SEO patches for matching repo files?"
          : `Improve and publish?\n\nFixes apply via WordPress/OpsAPI when configured, or as a GitHub PR when a connection/repo is set.`
      )
    ) {
      return;
    }
    setFormError("");
    setSubmitting(true);
    try {
      const max = Number.parseInt(maxPages, 10);
      const run = await createSEORun({
        agent_id: agentId,
        source,
        urls: source === "urls" ? urls.trim() : undefined,
        mode,
        platform,
        keywords: keywords.trim(),
        github_connection: source === "github" ? githubConnection.trim() : undefined,
        git_repo: gitRepo.trim() || undefined,
        git_branch: gitBranch.trim() || "main",
        path_glob: source === "github" ? pathGlob.trim() || undefined : undefined,
        max_pages: Number.isFinite(max) && max > 0 ? max : 10,
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

  const onSaveConnection = async (e: React.FormEvent) => {
    e.preventDefault();
    setSavingConn(true);
    try {
      await upsertSEOConnection({
        name: connName.trim(),
        repo: connRepo.trim(),
        default_branch: connBranch.trim() || "main",
        access_token: connToken.trim(),
      });
      setConnToken("");
      toast.success("GitHub connection saved");
      const conns = await listSEOConnections();
      setConnections(conns);
      setGithubConnection(connName.trim());
      setTab("run");
    } catch (err) {
      toast.error(apiErrorMessage(err, "Failed to save connection"));
    } finally {
      setSavingConn(false);
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
          Keyword-driven audits: public URLs one-by-one, or files in a GitHub repo via a named
          connection. Tokens stay server-side — never paste a PAT into the launch form.
        </p>
        {status ? (
          <p className="text-xs text-muted-foreground">
            Seal tokens: {status.can_seal_tokens ? "yes" : "no"} · Connections: {connections.length}
          </p>
        ) : null}
      </header>

      <div className="flex gap-2 border-b border-border pb-2">
        {(["run", "history", "connection"] as Tab[]).map((t) => (
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

      {tab === "connection" && (
        <div className="grid gap-6 lg:grid-cols-2">
          <form onSubmit={onSaveConnection} className="space-y-3 rounded-lg border border-border p-4">
            <h3 className="text-sm font-semibold">Add or update GitHub connection</h3>
            <label className="block text-sm font-medium">
              Connection name
              <input
                className={inputClass}
                value={connName}
                onChange={(e) => setConnName(e.target.value)}
                required
                placeholder="marketing-site"
              />
            </label>
            <label className="block text-sm font-medium">
              Repo
              <input
                className={inputClass}
                value={connRepo}
                onChange={(e) => setConnRepo(e.target.value)}
                required
                placeholder="owner/repo"
              />
            </label>
            <label className="block text-sm font-medium">
              Default branch
              <input
                className={inputClass}
                value={connBranch}
                onChange={(e) => setConnBranch(e.target.value)}
              />
            </label>
            <label className="block text-sm font-medium">
              Personal access token
              <input
                className={inputClass}
                type="password"
                value={connToken}
                onChange={(e) => setConnToken(e.target.value)}
                required
                autoComplete="off"
                placeholder="ghp_…"
              />
            </label>
            <button
              type="submit"
              disabled={savingConn || !status?.can_seal_tokens}
              className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
            >
              {savingConn ? "Saving…" : "Save connection"}
            </button>
            {!status?.can_seal_tokens ? (
              <p className="text-xs text-destructive">
                SEO_TOKEN_KEY is not set on this ring — connections cannot be stored.
              </p>
            ) : null}
          </form>
          <div className="rounded-lg border border-border p-4">
            <h3 className="text-sm font-semibold">Saved connections</h3>
            {connections.length === 0 ? (
              <p className="mt-2 text-sm text-muted-foreground">None yet.</p>
            ) : (
              <ul className="mt-2 divide-y divide-border">
                {connections.map((c) => (
                  <li key={c.id} className="flex items-center justify-between gap-2 py-2 text-sm">
                    <div>
                      <p className="font-medium">{c.name}</p>
                      <p className="text-xs text-muted-foreground">
                        {c.repo} · {c.default_branch}
                      </p>
                    </div>
                    <button
                      type="button"
                      className="text-xs text-destructive hover:underline"
                      onClick={async () => {
                        try {
                          await deleteSEOConnection(c.name);
                          setConnections(await listSEOConnections());
                          toast.success("Removed");
                        } catch (err) {
                          toast.error(apiErrorMessage(err, "Delete failed"));
                        }
                      }}
                    >
                      Remove
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      )}

      {tab === "run" && (
        <div className="grid gap-6 lg:grid-cols-2">
          <form onSubmit={submit} className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block text-sm font-medium">
                Source
                <select
                  className={inputClass}
                  value={source}
                  onChange={(e) => setSource(e.target.value as "urls" | "github")}
                >
                  <option value="urls">URL audit</option>
                  <option value="github">GitHub site optimise</option>
                </select>
              </label>
              <label className="block text-sm font-medium">
                Action
                <select className={inputClass} value={mode} onChange={(e) => setMode(e.target.value)}>
                  <option value="analyze">Analyse + report</option>
                  <option value="improve">Analyse + propose fixes</option>
                  <option value="publish">Propose + publish / PR</option>
                </select>
              </label>
            </div>

            <label className="block text-sm font-medium">
              Focus keywords
              <textarea
                className={inputClass}
                rows={3}
                value={keywords}
                onChange={(e) => setKeywords(e.target.value)}
                placeholder={"devops jobs\nsite reliability\nhire engineers"}
                required
              />
            </label>

            {source === "urls" ? (
              <label className="block text-sm font-medium">
                URLs (one per line)
                <textarea
                  className={inputClass}
                  rows={4}
                  value={urls}
                  onChange={(e) => setUrls(e.target.value)}
                  placeholder={"https://example.com/\nhttps://example.com/about"}
                  required
                />
              </label>
            ) : (
              <>
                <label className="block text-sm font-medium">
                  GitHub connection
                  <select
                    className={inputClass}
                    value={githubConnection}
                    onChange={(e) => setGithubConnection(e.target.value)}
                    required
                  >
                    <option value="">Select…</option>
                    {connections.map((c) => (
                      <option key={c.id} value={c.name}>
                        {c.name} ({c.repo})
                      </option>
                    ))}
                  </select>
                </label>
                <label className="block text-sm font-medium">
                  Path glob
                  <input
                    className={inputClass}
                    value={pathGlob}
                    onChange={(e) => setPathGlob(e.target.value)}
                    placeholder="content/**/*.md"
                  />
                </label>
                <label className="block text-sm font-medium">
                  Repo override (optional)
                  <input
                    className={inputClass}
                    value={gitRepo}
                    onChange={(e) => setGitRepo(e.target.value)}
                    placeholder="owner/repo"
                  />
                </label>
              </>
            )}

            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block text-sm font-medium">
                Max pages
                <input
                  className={inputClass}
                  value={maxPages}
                  onChange={(e) => setMaxPages(e.target.value)}
                />
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

            {source === "github" || mode === "publish" ? (
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
            ) : null}

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
                <span className="truncate font-medium">
                  {r.keywords || r.url}
                  {r.source ? ` · ${r.source}` : ""}
                </span>
                <span className="shrink-0 text-xs text-muted-foreground">
                  {r.mode} · {r.status}
                  {r.score ? ` · ${r.score.overall}` : ""}
                  {r.progress ? ` · ${r.progress.completed}/${r.progress.total}` : ""}
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
  const pages: SEOPageReport[] = run.pages ?? [];
  const [pageIdx, setPageIdx] = useState(0);
  const page = pages[pageIdx];

  return (
    <div className="space-y-4 rounded-xl border border-border p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <p className="truncate text-sm font-medium">{run.keywords || run.url}</p>
          <p className="text-xs text-muted-foreground">
            {run.source || "urls"} · {run.mode} · {run.status}
            {run.detected_cms ? ` · detected ${run.detected_cms}` : ""}
            {run.github_connection ? ` · ${run.github_connection}` : ""}
          </p>
          {run.progress ? (
            <p className="text-xs text-muted-foreground">
              {run.progress.phase || "working"} · {run.progress.completed}/{run.progress.total}
              {run.progress.current ? ` · ${run.progress.current}` : ""}
            </p>
          ) : null}
          {report?.rollup_summary ? (
            <p className="text-xs text-muted-foreground">{report.rollup_summary}</p>
          ) : null}
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

      {pages.length > 0 && (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold">Page queue</h3>
          <ul className="max-h-40 space-y-1 overflow-y-auto text-sm">
            {pages.map((p, i) => (
              <li key={`${p.path}-${i}`}>
                <button
                  type="button"
                  onClick={() => setPageIdx(i)}
                  className={`flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left ${
                    i === pageIdx ? "bg-muted" : "hover:bg-muted/50"
                  }`}
                >
                  <span className="truncate font-mono text-xs">{p.path}</span>
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {p.status}
                    {p.score ? ` · ${p.score.overall}` : ""}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}

      {page && page.status === "completed" && (
        <section className="space-y-2 rounded-md border border-border p-3">
          <h3 className="text-sm font-semibold">Page report</h3>
          <dl className="grid gap-2 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-xs text-muted-foreground">Title</dt>
              <dd className="break-words">{page.title || "—"}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Meta description</dt>
              <dd className="break-words">{page.meta_description || "—"}</dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-xs text-muted-foreground">Keywords found</dt>
              <dd className="text-xs">{(page.keywords_found || []).join(", ") || "—"}</dd>
            </div>
          </dl>
          {(page.issues || []).length > 0 && (
            <ul className="space-y-1">
              {(page.issues || []).slice(0, 8).map((i) => (
                <li key={i.id} className={`text-xs ${severityClass(i.severity)}`}>
                  {i.severity}: {i.title}
                </li>
              ))}
            </ul>
          )}
          {page.proposed_diff ? (
            <pre className="max-h-48 overflow-auto rounded bg-muted/50 p-2 text-xs">{page.proposed_diff}</pre>
          ) : null}
        </section>
      )}

      {report && pages.length === 0 && (
        <dl className="grid gap-2 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-xs text-muted-foreground">Title</dt>
            <dd className="break-words">{report.title || "—"}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Meta description</dt>
            <dd className="break-words">{report.meta_description || "—"}</dd>
          </div>
        </dl>
      )}

      {issues.length > 0 && pages.length === 0 && (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold">Issues</h3>
          <ul className="space-y-2">
            {issues.map((i) => (
              <li key={i.id} className="rounded-md border border-border px-3 py-2 text-sm">
                <p className={`font-medium ${severityClass(i.severity)}`}>
                  {i.severity}: {i.title}
                </p>
                <p className="text-muted-foreground">{i.detail}</p>
              </li>
            ))}
          </ul>
        </section>
      )}

      {suggestions.length > 0 && pages.length === 0 && (
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
