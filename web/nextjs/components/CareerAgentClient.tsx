"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";
import { CareerJobsPanel } from "@/components/career/CareerJobsPanel";
import { CareerProfilePanel } from "@/components/career/CareerProfilePanel";
import type { CareerJDPreview } from "@/components/career/CareerJDDialog";
import { mergeJobs, type CareerJob } from "@/components/career/jobs";
import { FieldHintProvider } from "@/components/ui/field-hint";
import { apiErrorMessage } from "@/lib/api/client";
import { splitCareerTailorNote } from "@/lib/career/tailor-note";
import {
  addCareerPortal,
  careerApplyRun,
  careerBatchEvaluate,
  careerCoverLetter,
  careerDoctor,
  careerEmailDraft,
  careerIntake,
  careerPatterns,
  careerScan,
  evaluateCareer,
  getCareerEvaluation,
  getCareerProfile,
  listCareerArtifacts,
  listCareerBlacklist,
  listCareerEvaluations,
  listCareerPipeline,
  listCareerPortals,
  listCareerStories,
  listCareerTracker,
  patchCareerProfile,
  previewCareerListing,
  setCareerStatus,
  tailorCareerCV,
  downloadCareerPDF,
} from "@/lib/api/career";
import type {
  CareerArtifact,
  CareerBlacklistEntry,
  CareerDoctorReport,
  CareerEvaluation,
  CareerPatterns,
  CareerPipelineItem,
  CareerPortal,
  CareerProfile,
  CareerStory,
  CareerApplication,
} from "@/types/career";
import { toast } from "sonner";
import { CareerNextStep, CareerStepper, type CareerStepId, type CareerStepState } from "./career/CareerStepper";

type Screen = "profile" | "find" | "jobs" | "prepare";

export function CareerAgentClient() {
  const search = useSearchParams();
  const [screen, setScreen] = useState<Screen>("profile");
  const [landed, setLanded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [saving, setSaving] = useState(false);
  const [savedFlash, setSavedFlash] = useState(false);

  const [profile, setProfile] = useState<CareerProfile | null>(null);
  const [doctor, setDoctor] = useState<CareerDoctorReport | null>(null);
  const [evals, setEvals] = useState<CareerEvaluation[]>([]);
  const [pipeline, setPipeline] = useState<CareerPipelineItem[]>([]);
  const [tracker, setTracker] = useState<CareerApplication[]>([]);
  const [portals, setPortals] = useState<CareerPortal[]>([]);
  const [blacklist, setBlacklist] = useState<CareerBlacklistEntry[]>([]);
  const [stories, setStories] = useState<CareerStory[]>([]);
  const [patterns, setPatterns] = useState<CareerPatterns | null>(null);
  const [artifacts, setArtifacts] = useState<CareerArtifact[]>([]);
  const [draftNote, setDraftNote] = useState("");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  const [jobUrl, setJobUrl] = useState("");
  const [jdText, setJdText] = useState("");
  const [cvDraft, setCvDraft] = useState("");
  const [fullName, setFullName] = useState("");
  const [sponsorship, setSponsorship] = useState(false);
  const [titles, setTitles] = useState<string[]>([]);
  const [minComp, setMinComp] = useState("");
  const [houseRules, setHouseRules] = useState("");
  const [scanBoard, setScanBoard] = useState("all");
  const [scanSlug, setScanSlug] = useState("");
  const [jdPreview, setJdPreview] = useState<CareerJDPreview | null>(null);

  const jobs = useMemo(() => mergeJobs(pipeline, tracker, evals), [pipeline, tracker, evals]);
  const selected = jobs.find((j) => j.key === selectedKey) ?? null;
  const hasCV = Boolean(profile?.cv_markdown?.trim() || cvDraft.trim());

  // ── Where the user is, and what to do next ────────────────────────────────
  // Both the stepper and the next-step bar read from here, so the two can never
  // disagree about whether a step is finished.
  const scoredCount = jobs.filter((j) => j.evaluation).length;
  const readyToScan = hasCV && titles.length > 0;

  const stepStates: Record<CareerStepId, CareerStepState> = {
    profile: {
      done: readyToScan,
      locked: false,
      detail: !hasCV
        ? "Upload your CV to start"
        : titles.length === 0
          ? "CV saved — now add the job titles you want"
          : `CV saved · ${titles.length} target title${titles.length === 1 ? "" : "s"}`,
    },
    find: {
      done: jobs.length > 0,
      locked: !readyToScan,
      blockedReason: !hasCV ? "Upload your CV first" : "Add target job titles first",
      detail: jobs.length > 0 ? `${jobs.length} job${jobs.length === 1 ? "" : "s"} found` : undefined,
    },
    jobs: {
      done: scoredCount > 0,
      locked: jobs.length === 0,
      blockedReason: "Scan for jobs first",
      detail:
        jobs.length === 0
          ? undefined
          : scoredCount > 0
            ? `${scoredCount} of ${jobs.length} scored`
            : `${jobs.length} waiting to be scored`,
    },
    prepare: {
      done: artifacts.length > 0,
      locked: !selected,
      blockedReason: "Open a job from Score & prepare",
      detail: selected ? `${selected.company || "This job"} — ${selected.role || "role"}` : undefined,
    },
  };

  const nextStep: {
    headline: string;
    detail?: string;
    actionLabel?: string;
    onAction?: () => void;
    tone?: "primary" | "done";
  } = !hasCV
    ? {
        headline: "Start by uploading your CV",
        detail: "PDF only. Everything after this — scores, tailored CVs, cover letters — is built from it.",
        actionLabel: screen === "profile" ? undefined : "Go to Your profile",
        onAction: screen === "profile" ? undefined : () => setScreen("profile"),
      }
    : titles.length === 0
      ? {
          headline: "Add the job titles you are looking for",
          detail: "The scan uses these to filter thousands of postings down to the ones worth reading.",
          actionLabel: screen === "profile" ? undefined : "Go to Your profile",
          onAction: screen === "profile" ? undefined : () => setScreen("profile"),
        }
      : jobs.length === 0
        ? {
            headline: "Scan company job boards",
            detail: "Greenhouse, Ashby and Lever. Takes under a minute and fills your job list.",
            actionLabel: "Scan all companies",
            onAction: () => void scan(true),
          }
        : scoredCount === 0
          ? {
              headline: `${jobs.length} jobs found — score them next`,
              detail: "Scoring rates each posting against your CV so you only spend time on real matches.",
              actionLabel: screen === "jobs" ? undefined : "Go to Score & prepare",
              onAction: screen === "jobs" ? undefined : () => setScreen("jobs"),
            }
          : {
              headline: `${scoredCount} job${scoredCount === 1 ? "" : "s"} scored`,
              detail:
                "Use Prepare to rewrite your CV for a posting and draft the cover letter. Nothing is ever submitted for you.",
              actionLabel: screen === "jobs" ? undefined : "Go to Score & prepare",
              onAction: screen === "jobs" ? undefined : () => setScreen("jobs"),
              tone: "done" as const,
            };

  const loadAll = useCallback(async (opts?: { preserveDrafts?: boolean }) => {
    const [p, d, e, pipe, apps, ports, bl, st, pat] = await Promise.all([
      getCareerProfile(),
      careerDoctor(),
      listCareerEvaluations(),
      listCareerPipeline(),
      listCareerTracker(),
      listCareerPortals(),
      listCareerBlacklist(),
      listCareerStories(),
      careerPatterns(),
    ]);
    setProfile(p);
    setCvDraft(p.cv_markdown ?? "");
    if (opts?.preserveDrafts) {
      // CV upload writes the PDF immediately; keep unsaved titles and rules.
      setFullName((prev) => prev.trim() || p.identity?.full_name || "");
    } else {
      setFullName(p.identity?.full_name ?? "");
      setSponsorship(!!p.work_auth?.needs_sponsorship);
      setTitles(p.targets?.titles ?? []);
      setMinComp(p.targets?.min_comp ?? "");
      setHouseRules(p.house_rules ?? "");
    }
    setDoctor(d);
    setEvals(e.data ?? []);
    setPipeline(pipe.data ?? []);
    setTracker(apps.data ?? []);
    setPortals(ports ?? []);
    setBlacklist(bl ?? []);
    setStories(st ?? []);
    setPatterns(pat);
    return d;
  }, []);

  useEffect(() => {
    void (async () => {
      try {
        await loadAll();
      } catch {
        setError("Failed to load Career Agent.");
      }
    })();
  }, [loadAll]);

  useEffect(() => {
    if (!doctor || landed) return;
    setScreen(doctor.ok ? "find" : "profile");
    setLanded(true);
  }, [doctor, landed]);

  useEffect(() => {
    const evalId = search.get("eval");
    if (!evalId) return;
    void getCareerEvaluation(evalId)
      .then((ev) => {
        setSelectedKey(ev.listing_url || `eval:${ev.id}`);
        setScreen("prepare");
      })
      .catch(() => undefined);
  }, [search]);

  useEffect(() => {
    const applicationId = selected?.application?.id || selected?.evaluation?.application_id;
    if (!applicationId) {
      setArtifacts([]);
      return;
    }
    void listCareerArtifacts(applicationId)
      .then(setArtifacts)
      .catch(() => setArtifacts([]));
  }, [selected?.application?.id, selected?.evaluation?.application_id]);

  // True when the form holds edits the server has not been told about. The
  // scan filters by the SAVED profile, so scanning while this is true both
  // ignores the titles on screen and — once loadAll refreshes from the
  // server — silently replaces them with what was stored.
  const profileDirty =
    fullName !== (profile?.identity?.full_name ?? "") ||
    sponsorship !== !!profile?.work_auth?.needs_sponsorship ||
    minComp !== (profile?.targets?.min_comp ?? "") ||
    houseRules !== (profile?.house_rules ?? "") ||
    titles.join("\u0000") !== (profile?.targets?.titles ?? []).join("\u0000");

  // The PATCH on its own, so scan() can persist without the toasts and the
  // full reload that saveProfile() runs.
  async function persistProfile() {
    const p = await patchCareerProfile({
      ...(cvDraft.trim() ? { cv_markdown: cvDraft } : {}),
      identity: { ...(profile?.identity ?? {}), full_name: fullName },
      work_auth: { ...(profile?.work_auth ?? {}), needs_sponsorship: sponsorship },
      targets: {
        ...(profile?.targets ?? {}),
        titles,
        min_comp: minComp,
      },
      house_rules: houseRules,
    });
    setProfile(p);
    return p;
  }

  async function saveProfile() {
    setBusy(true);
    setSaving(true);
    setSavedFlash(false);
    setError("");
    try {
      await persistProfile();
      const d = await loadAll();
      setSavedFlash(true);
      toast.success("Profile saved.");
      window.setTimeout(() => setSavedFlash(false), 2500);
      if (d.ok) {
        toast.message("Profile is ready. Find jobs next, or open Jobs if you already have some.");
      }
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, "Could not save profile.");
      setError(msg);
      toast.error(msg);
    } finally {
      setBusy(false);
      setSaving(false);
    }
  }

  async function fillFromCV() {
    if (!cvDraft.trim()) {
      toast.error("Upload a PDF CV first.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const prop = await careerIntake(cvDraft);
      const name = prop.patch?.identity?.full_name?.trim() ?? "";
      const email = prop.patch?.identity?.email?.trim() ?? "";
      if (name) setFullName(name);
      if (email) {
        setProfile((prev) =>
          prev ? { ...prev, identity: { ...prev.identity, email } } : prev
        );
      }
      if (name || email) {
        toast.success(
          `Filled ${[name && "name", email && "email"].filter(Boolean).join(" and ")} from the CV. Click Save profile to keep it.`
        );
      } else {
        toast.message("Couldn't pick a name from the top of the CV. Type it in Name, then Save profile.");
      }
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, "Could not read the CV.");
      setError(msg);
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  }

  async function runEvaluate(opts: {
    job_url?: string;
    jd_text?: string;
    tailor_cv?: boolean;
    confirm_blacklist?: boolean;
  }) {
    setBusy(true);
    setError("");
    try {
      const res = await evaluateCareer({
        job_url: opts.job_url?.trim() || undefined,
        jd_text: opts.jd_text?.trim() || undefined,
        mode: "full",
        tailor_cv: !!opts.tailor_cv,
        confirm_blacklist: !!opts.confirm_blacklist,
      });
      if (res.dead) {
        const msg = res.dead_reason || "That posting looks closed.";
        setError(msg);
        toast.error(msg);
        return res;
      }
      if (res.blacklist_hit && !opts.confirm_blacklist) {
        const label = res.blacklist_hit.company || res.blacklist_hit.domain;
        toast.message(`${label} is on your blacklist. Confirm to score it anyway.`);
        const ok = window.confirm(`Score ${label} anyway?`);
        if (ok) {
          return runEvaluate({ ...opts, confirm_blacklist: true });
        }
        return res;
      }
      if (res.evaluation) {
        const url = res.evaluation.listing_url?.trim();
        const key = url
          ? url
          : res.evaluation.application_id
            ? `app:${res.evaluation.application_id}`
            : `eval:${res.evaluation.id}`;
        setSelectedKey(key);
        if (res.artifacts) setArtifacts(res.artifacts);
        await loadAll();
        setScreen("prepare");
      }
      return res;
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, "Could not score that job.");
      setError(msg);
      toast.error(msg);
      return null;
    } finally {
      setBusy(false);
    }
  }

  async function addJob() {
    await runEvaluate({ job_url: jobUrl, jd_text: jdText });
  }

  async function scoreSelected() {
    if (!selected) return;
    await runEvaluate({
      job_url: selected.listing_url,
      jd_text: selected.evaluation?.jd_text,
    });
  }

  async function scoreJobs(picked: CareerJob[]) {
    const urls = Array.from(new Set(picked.map((j) => j.listing_url.trim()).filter(Boolean)));
    const pasteOnly = picked.filter((j) => !j.listing_url.trim() && j.evaluation?.jd_text?.trim());
    if (urls.length === 0 && pasteOnly.length === 0) {
      toast.message("Tick jobs that have a posting URL, then Score selected.");
      return;
    }
    if (urls.length > 8) {
      toast.message("Scoring the first 8 selected jobs. Tick fewer to choose which.");
    }
    setBusy(true);
    setError("");
    try {
      let evaluated = 0;
      if (urls.length > 0) {
        const out = await careerBatchEvaluate({ limit: Math.min(urls.length, 8), urls: urls.slice(0, 8) });
        evaluated += out.evaluated ?? 0;
      }
      for (const job of pasteOnly) {
        const res = await evaluateCareer({
          jd_text: job.evaluation?.jd_text,
          mode: "full",
        });
        if (res.evaluation) evaluated += 1;
      }
      await loadAll();
      toast.success(
        evaluated > 0
          ? `Scored ${evaluated} job${evaluated === 1 ? "" : "s"}. Open a row to prepare.`
          : "No jobs could be scored (closed, blacklist, or fetch failed)."
      );
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, "Could not score the selected jobs.");
      setError(msg);
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  }

  // Dry-run apply over several jobs: score, rewrite the CV for each posting,
  // draft the cover letter, assemble a package. The server refuses to submit,
  // so there is no "really apply" variant of this button to guard against.
  async function applyRun(picked: CareerJob[]) {
    const urls = picked.map((j) => j.listing_url.trim()).filter(Boolean);
    setBusy(true);
    setError("");
    try {
      // Scoring and tailoring read the SAVED profile, so unsaved edits would be
      // ignored here and then overwritten by the reload below.
      if (profileDirty) {
        await persistProfile();
      }
      const out = await careerApplyRun({
        // No selection means "work through the open pipeline", which is what
        // makes this an agent rather than a bulk button.
        urls: urls.length > 0 ? urls : undefined,
        limit: urls.length > 0 ? Math.min(urls.length, 5) : 5,
        concurrency: 4,
      });
      await loadAll();

      const tailored = out.outcomes.filter((o) => o.cv_tailored).length;
      const parts = [`Prepared ${out.prepared} of ${out.considered}`];
      if (out.skipped > 0) parts.push(`${out.skipped} skipped`);
      if (out.failed > 0) parts.push(`${out.failed} failed`);
      parts.push(`${tailored} CV${tailored === 1 ? "" : "s"} rewritten`);
      toast.success(`${parts.join(" · ")}. Nothing was submitted.`);

      if (out.prepared > 0 && tailored === 0) {
        toast.message(
          "No CV could be truthfully rewritten for these postings — the tailored copies match your stored CV."
        );
      }
      if (out.notice) toast.message(out.notice);
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, "Could not prepare these jobs.");
      setError(msg);
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  }

  async function seeJD(job: CareerJob) {
    setSelectedKey(job.key);
    const cached = job.evaluation?.jd_text?.trim() ?? "";
    setJdPreview({
      title: job.role || "Job description",
      company: job.company || "",
      url: job.listing_url,
      text: cached,
      loading: !cached && !!job.listing_url.trim(),
      error: "",
    });
    if (cached || !job.listing_url.trim()) {
      if (!cached) {
        setJdPreview({
          title: job.role || "Job description",
          company: job.company || "",
          url: job.listing_url,
          text: "",
          loading: false,
          error: "No job description stored for this row yet.",
        });
      }
      return;
    }
    try {
      const listing = await previewCareerListing(job.listing_url);
      setJdPreview({
        title: listing.title || job.role || "Job description",
        company: listing.company || job.company || "",
        url: listing.url || job.listing_url,
        text: listing.jd_text,
        loading: false,
        error: listing.jd_text
          ? ""
          : listing.dead_reason || "Could not load the JD. Use Go to posting to read it on the site.",
      });
    } catch (e: unknown) {
      setJdPreview({
        title: job.role || "Job description",
        company: job.company || "",
        url: job.listing_url,
        text: "",
        loading: false,
        error: apiErrorMessage(e, "Could not load the JD. Use Go to posting to read it on the site."),
      });
    }
  }

  async function tailorSelected() {
    if (!selected) return;
    setError("");
    setDraftNote("");
    if (!selected.evaluation) {
      const res = await runEvaluate({
        job_url: selected.listing_url,
        jd_text: undefined,
        tailor_cv: true,
      });
      const art = res?.artifacts?.find((a) => a.kind === "cv");
      if (art) {
        const downloaded = downloadCareerPDF(art);
        const { note } = splitCareerTailorNote(art.body_markdown);
        setDraftNote(note || (downloaded ? "Tailored CV downloaded. A human submits it." : "Tailored CV ready. A human submits."));
        setScreen("prepare");
        toast.success(downloaded ? "Tailored CV downloaded." : "Tailored CV ready.");
      }
      return;
    }
    setBusy(true);
    try {
      const art = await tailorCareerCV(selected.evaluation.id);
      setArtifacts((prev) => [art, ...prev.filter((a) => a.id !== art.id)]);
      const downloaded = downloadCareerPDF(art);
      const { note } = splitCareerTailorNote(art.body_markdown);
      setDraftNote(note || (downloaded ? "Tailored CV downloaded. A human submits it." : "Tailored CV ready. A human submits."));
      setScreen("prepare");
      toast.success(downloaded ? "Tailored CV downloaded." : "Tailored CV ready.");
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, "Could not tailor the CV.");
      setError(msg);
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  }

  async function draftFromEval(
    fn: (id: string) => Promise<CareerArtifact>,
    ok: string
  ) {
    const id = selected?.evaluation?.id;
    if (!id) {
      toast.message("Score this job first.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const art = await fn(id);
      setArtifacts((prev) => [art, ...prev.filter((a) => a.id !== art.id)]);
      setDraftNote(ok);
      toast.success(ok);
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, "Could not draft.");
      setError(msg);
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  }

  async function markApplied() {
    const id = selected?.application?.id;
    if (!id) {
      toast.message("Score this job first so it is on your tracker.");
      return;
    }
    setBusy(true);
    try {
      await setCareerStatus(id, "applied", "marked applied by hand");
      await loadAll();
      toast.success("Marked as applied. Nothing was submitted for you.");
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, "Could not update status.");
      setError(msg);
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  }

  async function scan(allCompanies: boolean) {
    setBusy(true);
    setError("");
    try {
      // Save first when the form is ahead of the server. Without this the scan
      // runs against the stored profile — so freshly typed target titles are
      // not applied as a filter — and the reload below then overwrites them.
      if (profileDirty) {
        await persistProfile();
      }
      const out = (await careerScan(
        allCompanies
          ? { board: "all" }
          : { board: scanBoard || "all", slug: scanSlug.trim() }
      )) as { run?: { added?: number } };
      await loadAll();
      const added = out.run?.added ?? 0;
      setScreen("jobs");
      toast.success(
        added > 0
          ? `Scan finished. ${added} new job${added === 1 ? "" : "s"}. Open a row to prepare.`
          : "Scan finished. No new jobs (already seen, title filter, or empty boards)."
      );
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, "Scan failed.");
      setError(msg);
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  }

  return (
    <FieldHintProvider>
      <div className="space-y-4">
        <CareerStepper current={screen} states={stepStates} onGo={setScreen} />

        <CareerNextStep
          headline={nextStep.headline}
          detail={nextStep.detail}
          actionLabel={nextStep.actionLabel}
          onAction={nextStep.onAction}
          busy={busy}
          tone={nextStep.tone}
        />

        {error && (
          <p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </p>
        )}

        {screen === "profile" && (
          <CareerProfilePanel
            cvDraft={cvDraft}
            setCvDraft={setCvDraft}
            fullName={fullName}
            setFullName={setFullName}
            sponsorship={sponsorship}
            setSponsorship={setSponsorship}
            titles={titles}
            setTitles={setTitles}
            minComp={minComp}
            setMinComp={setMinComp}
            houseRules={houseRules}
            setHouseRules={setHouseRules}
            blacklist={blacklist}
            stories={stories}
            busy={busy}
            saving={saving}
            savedFlash={savedFlash}
            onSave={() => void saveProfile()}
            onFillFromCV={() => void fillFromCV()}
            onReload={async (opts) => {
              await loadAll(opts);
            }}
            onError={setError}
          />
        )}

        {screen !== "profile" && (
          <CareerJobsPanel
            pane={screen}
            doctor={doctor}
            jobs={jobs}
            selected={selected}
            onSelect={(job) => {
              setSelectedKey(job.key);
              setDraftNote("");
              setScreen("prepare");
            }}
            artifacts={artifacts}
            draftNote={draftNote}
            busy={busy}
            hasCV={hasCV}
            jobUrl={jobUrl}
            setJobUrl={setJobUrl}
            jdText={jdText}
            setJdText={setJdText}
            onAddJob={() => void addJob()}
            scanBoard={scanBoard}
            setScanBoard={setScanBoard}
            scanSlug={scanSlug}
            setScanSlug={setScanSlug}
            portals={portals}
            onScanAll={() => void scan(true)}
            onScanOne={() => void scan(false)}
            onSavePortal={() => {
              if (!scanSlug.trim() || scanBoard === "all") return;
              void addCareerPortal({ board: scanBoard, slug: scanSlug })
                .then(() => loadAll())
                .then(() => toast.success("Board saved."))
                .catch((e) => {
                  const msg = apiErrorMessage(e, "Could not save board.");
                  setError(msg);
                  toast.error(msg);
                });
            }}
            patterns={patterns}
            onScore={() => void scoreSelected()}
            onScoreSelected={(picked) => void scoreJobs(picked)}
            onApplyRun={(picked) => void applyRun(picked)}
            onTailor={() => void tailorSelected()}
            onCover={() =>
              void draftFromEval(careerCoverLetter, "Cover letter draft ready. A human sends it.")
            }
            onEmail={() =>
              void draftFromEval(careerEmailDraft, "Email draft ready. A human sends it.")
            }
            onApplied={() => void markApplied()}
            onSeeJD={(job) => void seeJD(job)}
            jdPreview={jdPreview}
            onCloseJD={() => setJdPreview(null)}
            onBackToJobs={() => setScreen("jobs")}
          />
        )}
      </div>
    </FieldHintProvider>
  );
}
