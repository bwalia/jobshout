"use client";

import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import {
  addCareerBlacklist,
  addCareerPortal,
  careerBatchEvaluate,
  careerCoverLetter,
  careerDoctor,
  careerEmailDraft,
  careerFollowup,
  careerIntake,
  careerInterviewPrep,
  careerOfferPrep,
  careerPatterns,
  careerScan,
  careerSalaryGap,
  evaluateCareer,
  getCareerEvaluation,
  getCareerProfile,
  listCareerArtifacts,
  listCareerBlacklist,
  listCareerEvaluations,
  listCareerFollowups,
  listCareerPipeline,
  listCareerPortals,
  listCareerStories,
  listCareerTracker,
  patchCareerProfile,
  setCareerStatus,
  tailorCareerCV,
  upsertCareerStory,
} from "@/lib/api/career";
import type {
  CareerApplication,
  CareerArtifact,
  CareerBlacklistEntry,
  CareerDoctorReport,
  CareerEvaluation,
  CareerEvaluateResult,
  CareerFollowup,
  CareerPatterns,
  CareerPipelineItem,
  CareerPortal,
  CareerProfile,
  CareerStatus,
  CareerStory,
} from "@/types/career";

type Tab = "today" | "evaluate" | "pipeline" | "tracker" | "profile" | "analytics";

const TABS: { id: Tab; label: string }[] = [
  { id: "today", label: "Today" },
  { id: "evaluate", label: "Evaluate" },
  { id: "pipeline", label: "Pipeline" },
  { id: "tracker", label: "Tracker" },
  { id: "profile", label: "Profile" },
  { id: "analytics", label: "Analytics" },
];

const STATUSES: CareerStatus[] = [
  "evaluated",
  "applied",
  "responded",
  "interview",
  "offer",
  "rejected",
  "discarded",
  "skip",
  "hired",
];

export function CareerAgentClient() {
  const search = useSearchParams();
  const [tab, setTab] = useState<Tab>("today");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const [profile, setProfile] = useState<CareerProfile | null>(null);
  const [doctor, setDoctor] = useState<CareerDoctorReport | null>(null);
  const [evals, setEvals] = useState<CareerEvaluation[]>([]);
  const [selected, setSelected] = useState<CareerEvaluation | null>(null);
  const [pipeline, setPipeline] = useState<CareerPipelineItem[]>([]);
  const [tracker, setTracker] = useState<CareerApplication[]>([]);
  const [portals, setPortals] = useState<CareerPortal[]>([]);
  const [blacklist, setBlacklist] = useState<CareerBlacklistEntry[]>([]);
  const [stories, setStories] = useState<CareerStory[]>([]);
  const [followups, setFollowups] = useState<CareerFollowup[]>([]);
  const [patterns, setPatterns] = useState<CareerPatterns | null>(null);
  const [artifacts, setArtifacts] = useState<CareerArtifact[]>([]);
  const [draftNote, setDraftNote] = useState("");

  const [jobUrl, setJobUrl] = useState("");
  const [jdText, setJdText] = useState("");
  const [mode, setMode] = useState("full");
  const [tailor, setTailor] = useState(false);
  const [lastResult, setLastResult] = useState<CareerEvaluateResult | null>(null);

  const [cvDraft, setCvDraft] = useState("");
  const [fullName, setFullName] = useState("");
  const [sponsorship, setSponsorship] = useState(false);
  const [intakeText, setIntakeText] = useState("");
  const [titles, setTitles] = useState("");
  const [minComp, setMinComp] = useState("");
  const [houseRules, setHouseRules] = useState("");
  const [storyTitle, setStoryTitle] = useState("");
  const [storySit, setStorySit] = useState("");

  const [scanBoard, setScanBoard] = useState("greenhouse");
  const [scanSlug, setScanSlug] = useState("");

  const loadAll = useCallback(async () => {
    const [p, d, e, pipe, apps, ports, bl, st, fu, pat] = await Promise.all([
      getCareerProfile(),
      careerDoctor(),
      listCareerEvaluations(),
      listCareerPipeline(),
      listCareerTracker(),
      listCareerPortals(),
      listCareerBlacklist(),
      listCareerStories(),
      listCareerFollowups(),
      careerPatterns(),
    ]);
    setProfile(p);
    setCvDraft(p.cv_markdown ?? "");
    setFullName(p.identity?.full_name ?? "");
    setSponsorship(!!p.work_auth?.needs_sponsorship);
    setTitles((p.targets?.titles ?? []).join(", "));
    setMinComp(p.targets?.min_comp ?? "");
    setHouseRules(p.house_rules ?? "");
    setDoctor(d);
    setEvals(e.data ?? []);
    setPipeline(pipe.data ?? []);
    setTracker(apps.data ?? []);
    setPortals(ports ?? []);
    setBlacklist(bl ?? []);
    setStories(st ?? []);
    setFollowups(fu ?? []);
    setPatterns(pat);
  }, []);

  useEffect(() => {
    void (async () => {
      try {
        await loadAll();
      } catch {
        setError("Failed to load CareerOps.");
      }
    })();
  }, [loadAll]);

  useEffect(() => {
    const evalId = search.get("eval");
    if (!evalId) return;
    void getCareerEvaluation(evalId)
      .then((ev) => {
        setSelected(ev);
        setTab("evaluate");
      })
      .catch(() => undefined);
  }, [search]);

  async function runEvaluate(confirmBlacklist = false) {
    setBusy(true);
    setError("");
    try {
      const res = await evaluateCareer({
        job_url: jobUrl.trim() || undefined,
        jd_text: jdText.trim() || undefined,
        mode,
        tailor_cv: tailor,
        confirm_blacklist: confirmBlacklist,
      });
      setLastResult(res);
      if (res.dead) {
        setError(res.dead_reason || "That posting looks closed.");
        return;
      }
      if (res.blacklist_hit && !confirmBlacklist) {
        return;
      }
      if (res.evaluation) {
        setSelected(res.evaluation);
        if (res.artifacts) setArtifacts(res.artifacts);
        await loadAll();
      }
    } catch (e: unknown) {
      setError(apiErr(e, "Evaluation failed."));
    } finally {
      setBusy(false);
    }
  }

  async function loadArtifacts(applicationId?: string | null) {
    if (!applicationId) {
      setArtifacts([]);
      return;
    }
    try {
      setArtifacts(await listCareerArtifacts(applicationId));
    } catch {
      setArtifacts([]);
    }
  }

  useEffect(() => {
    void loadArtifacts(selected?.application_id);
  }, [selected?.application_id]);

  async function runDraft(
    fn: () => Promise<CareerArtifact>,
    ok = "Draft saved. A human submits or sends."
  ) {
    if (!selected) return;
    setBusy(true);
    setError("");
    setDraftNote("");
    try {
      await fn();
      await loadArtifacts(selected.application_id);
      setDraftNote(ok);
    } catch (e: unknown) {
      setError(apiErr(e, "Could not draft."));
    } finally {
      setBusy(false);
    }
  }

  async function saveProfile() {
    setBusy(true);
    setError("");
    try {
      const p = await patchCareerProfile({
        cv_markdown: cvDraft,
        identity: { ...(profile?.identity ?? {}), full_name: fullName },
        work_auth: { ...(profile?.work_auth ?? {}), needs_sponsorship: sponsorship },
        targets: {
          ...(profile?.targets ?? {}),
          titles: titles
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean),
          min_comp: minComp,
        },
        house_rules: houseRules,
      });
      setProfile(p);
      await loadAll();
    } catch (e: unknown) {
      setError(apiErr(e, "Could not save profile."));
    } finally {
      setBusy(false);
    }
  }

  async function runIntake() {
    setBusy(true);
    setError("");
    try {
      const prop = await careerIntake(intakeText);
      if (prop.patch.cv_markdown) setCvDraft(prop.patch.cv_markdown);
      if (prop.patch.identity?.full_name) setFullName(prop.patch.identity.full_name);
      setTab("profile");
    } catch (e: unknown) {
      setError(apiErr(e, "Intake failed."));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-1 border-b border-border">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            onClick={() => setTab(t.id)}
            className={`px-3 py-2 text-sm font-medium ${
              tab === t.id
                ? "border-b-2 border-primary text-foreground"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {error && (
        <p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </p>
      )}

      {tab === "today" && (
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground">
            CareerOps evaluates roles against your profile, drafts materials, and
            tracks the pipeline. A person always submits, sends, or clicks Apply.
          </p>
          {doctor && (
            <div className="rounded-md border border-border p-3 text-sm">
              <p className="font-medium">{doctor.ok ? "Profile looks healthy" : "Profile needs attention"}</p>
              <ul className="mt-2 list-disc space-y-1 pl-5 text-muted-foreground">
                {doctor.warnings.map((w) => (
                  <li key={w}>{w}</li>
                ))}
                {doctor.info.map((w) => (
                  <li key={w}>{w}</li>
                ))}
              </ul>
            </div>
          )}
          <div>
            <h3 className="mb-2 text-sm font-medium">Recent evaluations</h3>
            {evals.length === 0 ? (
              <p className="text-sm text-muted-foreground">None yet. Paste a JD on Evaluate.</p>
            ) : (
              <ul className="space-y-1">
                {evals.slice(0, 8).map((e) => (
                  <li key={e.id}>
                    <button
                      type="button"
                      className="text-left text-sm hover:underline"
                      onClick={() => {
                        setSelected(e);
                        setTab("evaluate");
                      }}
                    >
                      {e.role || "Role"} — {e.company || "Company"}{" "}
                      <span className="text-muted-foreground">
                        {e.score?.overall?.toFixed(1)} / 5
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      )}

      {tab === "evaluate" && (
        <div className="grid gap-6 lg:grid-cols-2">
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              void runEvaluate(false);
            }}
          >
            <label className="block text-sm font-medium">
              Job URL
              <input
                value={jobUrl}
                onChange={(e) => setJobUrl(e.target.value)}
                placeholder="https://boards.greenhouse.io/…"
                className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              />
            </label>
            <label className="block text-sm font-medium">
              Or paste the job description
              <textarea
                value={jdText}
                onChange={(e) => setJdText(e.target.value)}
                rows={10}
                className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                placeholder="Paste the JD. It is treated as untrusted data."
              />
            </label>
            <div className="flex flex-wrap items-center gap-3 text-sm">
              <label className="flex items-center gap-2">
                Mode
                <select
                  value={mode}
                  onChange={(e) => setMode(e.target.value)}
                  className="rounded-md border border-input bg-background px-2 py-1"
                >
                  <option value="full">Full</option>
                  <option value="triage">Triage</option>
                </select>
              </label>
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={tailor}
                  onChange={(e) => setTailor(e.target.checked)}
                />
                Tailor CV if score ≥ 4.0
              </label>
            </div>
            <button
              type="submit"
              disabled={busy || (!jobUrl.trim() && !jdText.trim())}
              className="rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
            >
              {busy ? "Evaluating…" : "Evaluate"}
            </button>
            {lastResult?.blacklist_hit && (
              <div className="rounded-md border border-signal-warn/40 p-3 text-sm">
                <p>
                  {lastResult.blacklist_hit.company || lastResult.blacklist_hit.domain}{" "}
                  is on your blacklist
                  {lastResult.blacklist_hit.reason
                    ? ` (${lastResult.blacklist_hit.reason})`
                    : ""}
                  . Evaluate anyway?
                </p>
                <button
                  type="button"
                  className="mt-2 rounded-md border border-border px-3 py-1"
                  onClick={() => void runEvaluate(true)}
                >
                  Yes, evaluate
                </button>
              </div>
            )}
          </form>
          <div className="min-h-0 overflow-y-auto rounded-md border border-border p-4 text-sm">
            {selected ? (
              <div className="space-y-3">
                <article className="prose prose-sm dark:prose-invert max-w-none whitespace-pre-wrap">
                  {selected.report_markdown || `${selected.role} — ${selected.score?.overall} / 5`}
                </article>
                <p className="text-xs text-muted-foreground">
                  Drafts only. A person always submits, sends, or clicks Apply.
                </p>
                <div className="flex flex-wrap gap-2">
                  <button
                    type="button"
                    className="rounded-md border border-border px-2 py-1 text-xs"
                    disabled={busy}
                    onClick={() => void runDraft(() => careerCoverLetter(selected.id))}
                  >
                    Cover letter
                  </button>
                  <button
                    type="button"
                    className="rounded-md border border-border px-2 py-1 text-xs"
                    disabled={busy}
                    onClick={() => void runDraft(() => tailorCareerCV(selected.id))}
                  >
                    Tailor CV
                  </button>
                  <button
                    type="button"
                    className="rounded-md border border-border px-2 py-1 text-xs"
                    disabled={busy}
                    onClick={() => void runDraft(() => careerEmailDraft(selected.id))}
                  >
                    Email draft
                  </button>
                </div>
                {draftNote && <p className="text-xs text-muted-foreground">{draftNote}</p>}
                {artifacts.length > 0 && (
                  <ul className="space-y-2 border-t border-border pt-2">
                    {artifacts.map((a) => (
                      <li key={a.id}>
                        <p className="text-xs font-medium">
                          {a.kind} — {a.title || "draft"}
                        </p>
                        <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap rounded bg-muted/40 p-2 text-xs">
                          {a.body_markdown}
                        </pre>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            ) : (
              <p className="text-muted-foreground">
                The A–H report appears here. Score below 4.0 means do not apply.
                Block G never changes the score. Nothing is submitted for you.
              </p>
            )}
          </div>
        </div>
      )}

      {tab === "pipeline" && (
        <div className="space-y-4">
          <form
            className="flex flex-wrap items-end gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              void (async () => {
                setBusy(true);
                setError("");
                try {
                  await careerScan({ board: scanBoard, slug: scanSlug });
                  await loadAll();
                } catch (err: unknown) {
                  setError(apiErr(err, "Scan failed."));
                } finally {
                  setBusy(false);
                }
              })();
            }}
          >
            <label className="text-sm">
              Board
              <select
                value={scanBoard}
                onChange={(e) => setScanBoard(e.target.value)}
                className="ml-2 rounded-md border border-input bg-background px-2 py-1"
              >
                <option value="greenhouse">Greenhouse</option>
                <option value="ashby">Ashby</option>
                <option value="lever">Lever</option>
              </select>
            </label>
            <label className="text-sm">
              Slug
              <input
                value={scanSlug}
                onChange={(e) => setScanSlug(e.target.value)}
                className="ml-2 rounded-md border border-input bg-background px-2 py-1"
                placeholder="company-board"
              />
            </label>
            <button
              type="submit"
              disabled={busy || !scanSlug.trim()}
              className="rounded-md border border-border px-3 py-1.5 text-sm"
            >
              Scan into pipeline
            </button>
            <button
              type="button"
              className="rounded-md border border-border px-3 py-1.5 text-sm"
              onClick={() => {
                if (!scanSlug.trim()) return;
                void addCareerPortal({ board: scanBoard, slug: scanSlug }).then(loadAll);
              }}
            >
              Save portal
            </button>
            <button
              type="button"
              className="rounded-md border border-border px-3 py-1.5 text-sm"
              disabled={busy}
              onClick={() => {
                void (async () => {
                  setBusy(true);
                  setError("");
                  try {
                    await careerBatchEvaluate();
                    await loadAll();
                  } catch (err: unknown) {
                    setError(apiErr(err, "Batch evaluate failed."));
                  } finally {
                    setBusy(false);
                  }
                })();
              }}
            >
              Triage open pipeline
            </button>
          </form>
          {portals.length > 0 && (
            <p className="text-xs text-muted-foreground">
              Saved portals: {portals.map((p) => `${p.board}:${p.slug}`).join(", ")}
            </p>
          )}
          {pipeline.length === 0 ? (
            <p className="text-sm text-muted-foreground">Pipeline is empty.</p>
          ) : (
            <ul className="divide-y divide-border rounded-md border border-border text-sm">
              {pipeline.map((it) => (
                <li key={it.id} className="flex items-center justify-between gap-2 px-3 py-2">
                  <div className="min-w-0">
                    <p className="truncate font-medium">{it.title || it.listing_url}</p>
                    <p className="text-muted-foreground">
                      {it.company} · {it.source} · {it.liveness}
                    </p>
                  </div>
                  <button
                    type="button"
                    className="shrink-0 text-xs underline"
                    onClick={() => {
                      setJobUrl(it.listing_url);
                      setTab("evaluate");
                    }}
                  >
                    Evaluate
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {tab === "tracker" && (
        <div className="space-y-3">
          {tracker.length === 0 ? (
            <p className="text-sm text-muted-foreground">No applications yet.</p>
          ) : (
            <ul className="divide-y divide-border rounded-md border border-border text-sm">
              {tracker.map((a) => (
                <li key={a.id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
                  <div>
                    <p className="font-medium">
                      {a.role} — {a.company}
                    </p>
                    <p className="text-muted-foreground">
                      {a.score != null ? `${a.score.toFixed(1)} / 5` : "unscored"} · {a.status}
                    </p>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <select
                      value={a.status}
                      onChange={(e) => {
                        void setCareerStatus(a.id, e.target.value)
                          .then(loadAll)
                          .catch((err) => {
                            setError(apiErr(err, "Could not change status."));
                          });
                      }}
                      className="rounded-md border border-input bg-background px-2 py-1 text-xs"
                    >
                      {STATUSES.map((s) => (
                        <option key={s} value={s}>
                          {s}
                        </option>
                      ))}
                    </select>
                    <TrackerActions
                      app={a}
                      busy={busy}
                      onNote={setDraftNote}
                      onError={(msg) => setError(msg)}
                      onBusy={setBusy}
                    />
                  </div>
                </li>
              ))}
            </ul>
          )}
          {draftNote && tab === "tracker" && (
            <pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded-md border border-border p-3 text-xs">
              {draftNote}
            </pre>
          )}
          {followups.length > 0 && (
            <div>
              <h3 className="mb-1 text-sm font-medium">Follow-ups (draft, not sent)</h3>
              <ul className="text-sm text-muted-foreground">
                {followups.map((f) => (
                  <li key={f.id}>
                    {new Date(f.due_at).toLocaleDateString()} — {f.draft}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}

      {tab === "profile" && (
        <div className="space-y-4">
          <label className="block text-sm font-medium">
            Name
            <input
              value={fullName}
              onChange={(e) => setFullName(e.target.value)}
              className="mt-1 w-full max-w-md rounded-md border border-input bg-background px-3 py-2 text-sm"
            />
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={sponsorship}
              onChange={(e) => setSponsorship(e.target.checked)}
            />
            I need visa sponsorship
          </label>
          <label className="block text-sm font-medium">
            Target titles (comma-separated)
            <input
              value={titles}
              onChange={(e) => setTitles(e.target.value)}
              className="mt-1 w-full max-w-md rounded-md border border-input bg-background px-3 py-2 text-sm"
              placeholder="Head of AI, Staff engineer"
            />
          </label>
          <label className="block text-sm font-medium">
            Target compensation
            <input
              value={minComp}
              onChange={(e) => setMinComp(e.target.value)}
              className="mt-1 w-full max-w-md rounded-md border border-input bg-background px-3 py-2 text-sm"
              placeholder="£180k+"
            />
          </label>
          <label className="block text-sm font-medium">
            House rules
            <textarea
              value={houseRules}
              onChange={(e) => setHouseRules(e.target.value)}
              rows={3}
              className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              placeholder="Scoring overrides. Floors cannot drop below 4.0 / 4.5."
            />
          </label>
          <label className="block text-sm font-medium">
            CV (markdown — source of truth)
            <textarea
              value={cvDraft}
              onChange={(e) => setCvDraft(e.target.value)}
              rows={14}
              className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-sm"
            />
          </label>
          <button
            type="button"
            onClick={() => void saveProfile()}
            disabled={busy}
            className="rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground"
          >
            Save profile
          </button>
          <div className="border-t border-border pt-4">
            <p className="mb-2 text-sm font-medium">Intake (propose, then save)</p>
            <textarea
              value={intakeText}
              onChange={(e) => setIntakeText(e.target.value)}
              rows={6}
              placeholder="Paste a CV or LinkedIn export. Nothing is written until you Save profile."
              className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            />
            <button
              type="button"
              className="mt-2 rounded-md border border-border px-3 py-1.5 text-sm"
              onClick={() => void runIntake()}
              disabled={busy || !intakeText.trim()}
            >
              Propose from document
            </button>
          </div>
          <div className="border-t border-border pt-4">
            <p className="mb-2 text-sm font-medium">Blacklist</p>
            <ul className="mb-2 text-sm text-muted-foreground">
              {blacklist.length === 0 ? (
                <li>Empty — evaluate will never skip a company silently.</li>
              ) : (
                blacklist.map((b) => (
                  <li key={b.id}>
                    {b.company || b.domain}
                    {b.reason ? ` — ${b.reason}` : ""}
                  </li>
                ))
              )}
            </ul>
            <BlacklistForm
              onAdd={async (company, reason) => {
                await addCareerBlacklist({ company, reason });
                await loadAll();
              }}
            />
          </div>
          <div className="border-t border-border pt-4">
            <p className="mb-2 text-sm font-medium">Story bank (STAR+R)</p>
            {stories.length === 0 ? (
              <p className="mb-2 text-sm text-muted-foreground">
                Empty. High-score evaluations add derived-unverified plans you should confirm.
              </p>
            ) : (
              <ul className="mb-2 space-y-1 text-sm text-muted-foreground">
                {stories.map((s) => (
                  <li key={s.id}>
                    {s.title} <span className="text-xs">({s.provenance})</span>
                  </li>
                ))}
              </ul>
            )}
            <form
              className="flex flex-wrap gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                if (!storyTitle.trim()) return;
                void upsertCareerStory({
                  title: storyTitle,
                  situation: storySit,
                  provenance: "user_stated",
                })
                  .then(() => {
                    setStoryTitle("");
                    setStorySit("");
                    return loadAll();
                  })
                  .catch((err) => setError(apiErr(err, "Could not save story.")));
              }}
            >
              <input
                value={storyTitle}
                onChange={(e) => setStoryTitle(e.target.value)}
                placeholder="Story title"
                className="rounded-md border border-input bg-background px-2 py-1 text-sm"
              />
              <input
                value={storySit}
                onChange={(e) => setStorySit(e.target.value)}
                placeholder="Situation"
                className="min-w-[12rem] flex-1 rounded-md border border-input bg-background px-2 py-1 text-sm"
              />
              <button type="submit" className="rounded-md border border-border px-2 py-1 text-sm">
                Add story
              </button>
            </form>
          </div>
        </div>
      )}

      {tab === "analytics" && (
        <div className="space-y-3 text-sm">
          {!patterns ? (
            <p className="text-muted-foreground">No tracker data yet.</p>
          ) : (
            <>
              <p>
                {patterns.applications} applications
                {patterns.avg_score
                  ? ` · average score ${patterns.avg_score.toFixed(1)} / 5`
                  : ""}
              </p>
              <ul className="text-muted-foreground">
                {Object.entries(patterns.by_status ?? {}).map(([k, v]) => (
                  <li key={k}>
                    {k}: {v}
                  </li>
                ))}
              </ul>
              {patterns.skill_gaps && patterns.skill_gaps.length > 0 && (
                <div>
                  <p className="font-medium">Skill-gap tokens from sub-4.0 JDs</p>
                  <p className="text-muted-foreground">{patterns.skill_gaps.join(", ")}</p>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}

function TrackerActions({
  app,
  busy,
  onNote,
  onError,
  onBusy,
}: {
  app: CareerApplication;
  busy: boolean;
  onNote: (s: string) => void;
  onError: (s: string) => void;
  onBusy: (b: boolean) => void;
}) {
  async function run(label: string, fn: () => Promise<{ prep_markdown?: string; draft?: string; note?: string }>) {
    onBusy(true);
    try {
      const out = await fn();
      onNote(out.prep_markdown || out.draft || out.note || `${label} ready. Draft only.`);
    } catch (e: unknown) {
      onError(apiErr(e, `${label} failed.`));
    } finally {
      onBusy(false);
    }
  }
  return (
    <>
      <button
        type="button"
        className="text-xs underline"
        disabled={busy}
        onClick={() => void run("Follow-up", () => careerFollowup(app.id))}
      >
        Follow-up
      </button>
      <button
        type="button"
        className="text-xs underline"
        disabled={busy}
        onClick={() => void run("Interview prep", () => careerInterviewPrep(app.id))}
      >
        Prep
      </button>
      {app.status === "offer" && (
        <>
          <button
            type="button"
            className="text-xs underline"
            disabled={busy}
            onClick={() => void run("Offer prep", () => careerOfferPrep(app.id))}
          >
            Offer walk
          </button>
          <button
            type="button"
            className="text-xs underline"
            disabled={busy}
            onClick={() => void run("Salary gap", () => careerSalaryGap(app.id))}
          >
            Salary gap
          </button>
        </>
      )}
    </>
  );
}

function BlacklistForm({
  onAdd,
}: {
  onAdd: (company: string, reason: string) => Promise<void>;
}) {
  const [company, setCompany] = useState("");
  const [reason, setReason] = useState("");
  return (
    <form
      className="flex flex-wrap gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        void onAdd(company, reason).then(() => {
          setCompany("");
          setReason("");
        });
      }}
    >
      <input
        value={company}
        onChange={(e) => setCompany(e.target.value)}
        placeholder="Company"
        className="rounded-md border border-input bg-background px-2 py-1 text-sm"
      />
      <input
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        placeholder="Reason"
        className="rounded-md border border-input bg-background px-2 py-1 text-sm"
      />
      <button type="submit" className="rounded-md border border-border px-2 py-1 text-sm">
        Add
      </button>
    </form>
  );
}

function apiErr(e: unknown, fallback: string): string {
  return (
    (e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
  );
}
