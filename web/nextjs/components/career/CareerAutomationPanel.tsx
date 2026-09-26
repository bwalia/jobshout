"use client";

import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, Clock, Pause, Play, AlertTriangle } from "lucide-react";
import { toast } from "sonner";
import {
  createScheduledTask,
  deleteScheduledTask,
  getScheduledTasks,
  updateScheduledTask,
} from "@/lib/api/scheduler";
import type { ScheduledTask } from "@/lib/types/scheduler";
import type { CareerDoctorReport } from "@/types/career";
import { apiErrorMessage } from "@/lib/api/client";
import { FieldHint, FieldLabel } from "@/components/ui/field-hint";

const TASK_NAME = "Career Agent — daily preparation";

/** Jobs prepared per run. Each one costs three model calls, so this is the
 *  main lever on both time and spend. */
const PER_RUN_CHOICES = [2, 5, 10, 15, 25];

/** How often the run repeats, in seconds. */
const CADENCE_CHOICES: { label: string; seconds: number }[] = [
  { label: "Every 6 hours", seconds: 6 * 3600 },
  { label: "Once a day", seconds: 24 * 3600 },
  { label: "Twice a day", seconds: 12 * 3600 },
  { label: "Every hour", seconds: 3600 },
];

/**
 * Unattended preparation for the Career Agent.
 *
 * The agent prepares and never submits: it scores, rewrites the CV for each
 * posting and drafts the cover letter, then leaves the package in the tracker
 * for you. That is stated plainly here rather than buried, because a panel
 * called "automation" on a job board invites exactly the opposite assumption.
 */
export function CareerAutomationPanel({
  doctor,
  hasCV,
  titleCount,
  onGoToProfile,
}: {
  doctor: CareerDoctorReport | null;
  hasCV: boolean;
  titleCount: number;
  onGoToProfile: () => void;
}) {
  const [task, setTask] = useState<ScheduledTask | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [perRun, setPerRun] = useState(5);
  const [seconds, setSeconds] = useState(24 * 3600);
  const [minScore, setMinScore] = useState(4);

  // What the run needs before it can do anything useful. Checked here so the
  // answer is on screen before it is switched on, rather than discovered as an
  // empty result the next morning.
  const blockers = useMemo(() => {
    const out: string[] = [];
    if (!hasCV) out.push("Upload your CV — every score and rewrite is built from it.");
    if (titleCount === 0) out.push("Add target job titles — the scan filters on them.");
    return out;
  }, [hasCV, titleCount]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await getScheduledTasks({ page: 1, per_page: 100 });
        const found = (res.data ?? []).find((t) => t.task_type === "career_apply") ?? null;
        if (cancelled) return;
        setTask(found);
        if (found) {
          const j = found.input_json ?? {};
          if (typeof j.limit === "number") setPerRun(j.limit);
          if (typeof j.min_score === "number") setMinScore(j.min_score);
          if (found.interval_seconds) setSeconds(found.interval_seconds);
        }
      } catch {
        /* Listing failing should not block the rest of the tab. */
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const running = task?.status === "active";

  async function enable() {
    setBusy(true);
    try {
      const payload = {
        limit: perRun,
        concurrency: Math.min(4, perRun),
        min_score: minScore,
      };
      if (task) {
        const updated = await updateScheduledTask(task.id, {
          interval_seconds: seconds,
          input_json: payload,
          status: "active" as const,
        });
        setTask(updated);
      } else {
        const created = await createScheduledTask({
          name: TASK_NAME,
          task_type: "career_apply",
          // The run reads everything it needs from input_json and the saved
          // profile; there is no prompt to pass.
          input_prompt: "",
          schedule_type: "interval",
          interval_seconds: seconds,
          input_json: payload,
        });
        setTask(created);
      }
      toast.success(
        `Preparation is on. Up to ${perRun} job${perRun === 1 ? "" : "s"} per run. Nothing is submitted.`
      );
    } catch (e: unknown) {
      toast.error(apiErrorMessage(e, "Could not turn preparation on."));
    } finally {
      setBusy(false);
    }
  }

  async function pause() {
    if (!task) return;
    setBusy(true);
    try {
      const updated = await updateScheduledTask(task.id, { status: "paused" as const });
      setTask(updated);
      toast.message("Preparation paused. Nothing will run until you turn it back on.");
    } catch (e: unknown) {
      toast.error(apiErrorMessage(e, "Could not pause."));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!task) return;
    setBusy(true);
    try {
      await deleteScheduledTask(task.id);
      setTask(null);
      toast.message("Schedule removed.");
    } catch (e: unknown) {
      toast.error(apiErrorMessage(e, "Could not remove the schedule."));
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return <p className="text-base text-muted-foreground">Loading automation…</p>;
  }

  return (
    <div className="space-y-5">
      <section className="rounded-lg border border-border bg-card p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="text-lg font-semibold">Prepare jobs automatically</h3>
            <p className="mt-1 max-w-2xl text-base leading-relaxed text-muted-foreground">
              On a schedule, the agent scores your open jobs, rewrites your CV for each
              posting that clears the threshold, and drafts the cover letter. Everything
              lands in your tracker ready to send.
            </p>
          </div>
          <span
            className={[
              "shrink-0 rounded-full px-3 py-1 text-sm font-semibold",
              running
                ? "bg-signal/15 text-signal"
                : "bg-muted text-muted-foreground",
            ].join(" ")}
          >
            {running ? "On" : task ? "Paused" : "Off"}
          </span>
        </div>

        {/* The single most important thing on this panel. */}
        <p className="mt-4 flex items-start gap-2 rounded-md border border-border bg-muted/50 px-3 py-2.5 text-base">
          <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-signal" aria-hidden />
          <span>
            <span className="font-semibold">It never applies for you.</span> No application
            is submitted to any job board, and no job moves to Applied on its own. You send
            it, from the tracker, after reading it.
          </span>
        </p>
      </section>

      {blockers.length > 0 && (
        <section className="rounded-lg border border-signal-warn/40 bg-signal-warn/5 p-5">
          <h4 className="flex items-center gap-2 text-base font-semibold">
            <AlertTriangle className="h-5 w-5 text-signal-warn" aria-hidden />
            Finish these first
          </h4>
          <ul className="mt-2 space-y-1.5">
            {blockers.map((b) => (
              <li key={b} className="text-base text-muted-foreground">
                {b}
              </li>
            ))}
          </ul>
          <button
            type="button"
            onClick={onGoToProfile}
            className="mt-3 rounded-md bg-primary px-4 py-2.5 text-base font-medium text-primary-foreground hover:opacity-90"
          >
            Go to Your profile
          </button>
        </section>
      )}

      <section className="grid gap-5 rounded-lg border border-border bg-card p-5 sm:grid-cols-3">
        <div>
          <FieldLabel
            htmlFor="auto-per-run"
            label="Jobs per run"
            hint="Each job costs three model calls — scoring, the CV rewrite and the cover letter — so this drives both how long a run takes and what it spends."
          />
          <select
            id="auto-per-run"
            value={perRun}
            onChange={(e) => setPerRun(Number(e.target.value))}
            className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2.5 text-base"
          >
            {PER_RUN_CHOICES.map((n) => (
              <option key={n} value={n}>
                {n} jobs
              </option>
            ))}
          </select>
        </div>

        <div>
          <FieldLabel htmlFor="auto-cadence" label="How often" hint="When the run repeats." />
          <select
            id="auto-cadence"
            value={seconds}
            onChange={(e) => setSeconds(Number(e.target.value))}
            className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2.5 text-base"
          >
            {CADENCE_CHOICES.map((c) => (
              <option key={c.seconds} value={c.seconds}>
                {c.label}
              </option>
            ))}
          </select>
        </div>

        <div>
          <FieldLabel
            htmlFor="auto-min-score"
            label="Only prepare above"
            hint="CareerOps does not recommend applying below 4.0 out of 5. Lowering this still prepares materials, but the evaluation's own recommendation is unchanged — preparing is not recommending."
          />
          <select
            id="auto-min-score"
            value={minScore}
            onChange={(e) => setMinScore(Number(e.target.value))}
            className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2.5 text-base"
          >
            {[4, 3.5, 3, 2.5, 2].map((n) => (
              <option key={n} value={n}>
                {n.toFixed(1)} / 5{n === 4 ? " (recommended)" : ""}
              </option>
            ))}
          </select>
        </div>

        <div className="sm:col-span-3">
          <p className="text-base text-muted-foreground">
            About{" "}
            <span className="font-semibold text-foreground">
              {estimatePerDay(perRun, seconds)} jobs a day
            </span>{" "}
            prepared, at roughly {Math.round((perRun * 3) / 1)} model calls per run.
          </p>
        </div>
      </section>

      <section className="flex flex-wrap items-center gap-3">
        <button
          type="button"
          onClick={() => void enable()}
          disabled={busy || blockers.length > 0}
          className="inline-flex items-center gap-2 rounded-md bg-primary px-5 py-2.5 text-base font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
        >
          <Play className="h-5 w-5" aria-hidden />
          {busy ? "Saving…" : running ? "Update schedule" : "Turn on preparation"}
        </button>
        {running && (
          <button
            type="button"
            onClick={() => void pause()}
            disabled={busy}
            className="inline-flex items-center gap-2 rounded-md border border-border px-5 py-2.5 text-base font-medium hover:bg-muted disabled:opacity-50"
          >
            <Pause className="h-5 w-5" aria-hidden />
            Pause
          </button>
        )}
        {task && (
          <button
            type="button"
            onClick={() => void remove()}
            disabled={busy}
            className="rounded-md px-3 py-2.5 text-base font-medium text-destructive hover:bg-destructive/10 disabled:opacity-50"
          >
            Remove schedule
          </button>
        )}
        {blockers.length > 0 && (
          <FieldHint text="Finish the profile steps above before turning this on." />
        )}
      </section>

      {task && (
        <section className="rounded-lg border border-border bg-card p-5">
          <h4 className="text-base font-semibold">Schedule</h4>
          <dl className="mt-2 grid gap-x-6 gap-y-1.5 text-base sm:grid-cols-2">
            <Row label="Last run" value={formatWhen(task.last_run_at)} icon />
            <Row label="Next run" value={running ? formatWhen(task.next_run_at) : "Paused"} icon />
            <Row label="Runs so far" value={String(task.run_count ?? 0)} />
            <Row label="Submitted" value="0 — this agent never submits" />
          </dl>
          {doctor && !doctor.ok && doctor.warnings.length > 0 && (
            <p className="mt-3 text-base text-muted-foreground">
              Doctor still reports: {doctor.warnings[0]}
            </p>
          )}
        </section>
      )}
    </div>
  );
}

function Row({ label, value, icon }: { label: string; value: string; icon?: boolean }) {
  return (
    <div className="flex items-center gap-2">
      {icon && <Clock className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden />}
      <dt className="text-muted-foreground">{label}:</dt>
      <dd className="font-medium text-foreground">{value}</dd>
    </div>
  );
}

/** Runs per day times jobs per run, rounded to something readable. */
function estimatePerDay(perRun: number, seconds: number) {
  const runsPerDay = (24 * 3600) / seconds;
  return Math.round(perRun * runsPerDay);
}

function formatWhen(iso: string | null) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString(undefined, {
    weekday: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}
