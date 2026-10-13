"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useSearchParams } from "next/navigation";
import {
  AlertTriangle,
  CheckCircle2,
  Clapperboard,
  Download,
  FileText,
  Film,
  Hourglass,
  ImageIcon,
  Loader2,
  Mic,
  Save,
  Scissors,
  XCircle,
  type LucideIcon,
} from "lucide-react";

import { newTaskHref } from "@/lib/agents/links";
import { useCancelVideoRun, useVideoRuns } from "@/lib/hooks/useVideoRuns";
import { isVideoRunActive, type VideoRun, type VideoRunStatus } from "@/lib/types/video";
import { cn } from "@/lib/utils/cn";

const STATUS_META: Record<VideoRunStatus, { label: string; className: string }> = {
  scripting: { label: "Writing script", className: "bg-signal-live/10 text-signal-live" },
  queued: { label: "Waiting", className: "bg-signal/10 text-signal" },
  rendering: { label: "Rendering", className: "bg-signal-live/10 text-signal-live" },
  done: { label: "Ready", className: "bg-status-done/10 text-status-done" },
  failed: { label: "Failed", className: "bg-signal-error/10 text-signal-error" },
  cancelled: { label: "Cancelled", className: "bg-muted text-muted-foreground" },
};

function StatusBadge({ status }: { status: VideoRunStatus }) {
  const meta = STATUS_META[status] ?? STATUS_META.queued;
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-2xs font-medium",
        meta.className
      )}
    >
      {isVideoRunActive(status) ? (
        <Loader2 className="h-3 w-3 animate-spin" />
      ) : status === "done" ? (
        <CheckCircle2 className="h-3 w-3" />
      ) : status === "failed" ? (
        <AlertTriangle className="h-3 w-3" />
      ) : null}
      {meta.label}
    </span>
  );
}

/** The agent's steps, in order. `stages` are the run stages that mean "on this step". */
const STEPS: { key: string; label: string; icon: LucideIcon; stages: string[] }[] = [
  { key: "script", label: "Write the script", icon: FileText, stages: ["script"] },
  { key: "queued", label: "Wait for the workstation", icon: Hourglass, stages: ["queued"] },
  { key: "narration", label: "Record the narration", icon: Mic, stages: ["starting", "narration"] },
  { key: "keyframes", label: "Draw each shot", icon: ImageIcon, stages: ["keyframes"] },
  { key: "motion", label: "Animate each shot", icon: Film, stages: ["motion"] },
  { key: "assembly", label: "Edit: captions, fades and cuts", icon: Scissors, stages: ["assembly"] },
  { key: "saving", label: "Save the video", icon: Save, stages: ["saving"] },
];

function stepsFor(run: VideoRun) {
  return run.brief?.motion || run.motion_model ? STEPS : STEPS.filter((s) => s.key !== "motion");
}

/** Index of the step the run is on; steps.length when every step is done. */
function currentStep(run: VideoRun, steps: typeof STEPS): number {
  if (run.status === "done") return steps.length;
  const i = steps.findIndex((s) => s.stages.includes(run.stage));
  if (i >= 0) return i;
  if (run.status === "scripting") return 0;
  if (run.status === "queued") return 1;
  return 2;
}

const VOICE_LABELS: Record<string, string> = {
  af_heart: "Heart (US female)",
  am_michael: "Michael (US male)",
  af_bella: "Bella (US female)",
  am_fenrir: "Fenrir (US male)",
  bf_emma: "Emma (UK female)",
  bm_george: "George (UK male)",
  hf_alpha: "Alpha (Hindi female)",
  hm_omega: "Omega (Hindi male)",
};

/** Re-renders every interval so elapsed times move between polls. */
function useNow(intervalMs = 30_000) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(t);
  }, [intervalMs]);
  return now;
}

function minutesBetween(fromIso: string | undefined, to: number): number {
  if (!fromIso) return 0;
  return Math.max(0, Math.round((to - Date.parse(fromIso)) / 60_000));
}

function formatMinutes(min: number): string {
  if (min < 60) return `${min} min`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  return m ? `${h} h ${m} min` : `${h} h`;
}

function formatClock(seconds: number): string {
  const s = Math.round(seconds);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

/** Where the run stands in time, in words. */
function timing(run: VideoRun, now: number): string {
  switch (run.status) {
    case "scripting":
      return `Writing for ${formatMinutes(minutesBetween(run.created_at, now))}`;
    case "queued":
      return "Waiting for the workstation: another video is rendering first";
    case "rendering": {
      const spent = minutesBetween(run.started_at, now);
      const left = run.estimate_minutes - spent;
      return left > 1
        ? `${formatMinutes(spent)} in · about ${formatMinutes(left)} left`
        : `${formatMinutes(spent)} in · finishing up`;
    }
    case "done":
      return run.started_at && run.finished_at
        ? `Rendered in ${formatMinutes(minutesBetween(run.started_at, Date.parse(run.finished_at)))}`
        : "Ready";
    default:
      return run.finished_at ? `Stopped ${new Date(run.finished_at).toLocaleString()}` : "";
  }
}

function Pipeline({ run }: { run: VideoRun }) {
  const steps = stepsFor(run);
  const current = currentStep(run, steps);
  const stopped = run.status === "failed" || run.status === "cancelled";
  return (
    <ol className="space-y-3" aria-label="What the agent is doing">
      {steps.map((step, i) => {
        const state =
          i < current ? "done" : i === current ? (stopped ? "stopped" : "active") : "pending";
        const Icon = step.icon;
        return (
          <li key={step.key} className="flex items-start gap-3">
            <span
              className={cn(
                "mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border",
                state === "done" && "border-status-done/40 bg-status-done/10 text-status-done",
                state === "active" && "border-signal-live/40 bg-signal-live/10 text-signal-live",
                state === "stopped" &&
                  (run.status === "failed"
                    ? "border-signal-error/40 bg-signal-error/10 text-signal-error"
                    : "border-border bg-muted text-muted-foreground"),
                state === "pending" && "border-border text-muted-foreground/60"
              )}
            >
              {state === "done" ? (
                <CheckCircle2 className="h-3.5 w-3.5" />
              ) : state === "active" ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : state === "stopped" ? (
                <XCircle className="h-3.5 w-3.5" />
              ) : (
                <Icon className="h-3.5 w-3.5" />
              )}
            </span>
            <div className="min-w-0">
              <p
                className={cn(
                  "text-sm",
                  state === "pending" ? "text-muted-foreground" : "font-medium text-foreground"
                )}
              >
                {step.label}
              </p>
              {/* A failure's reason is shown once, in the error box above. */}
              {(state === "active" || (state === "stopped" && run.status !== "failed")) && run.message && (
                <p className="text-xs text-muted-foreground">{run.message}</p>
              )}
            </div>
          </li>
        );
      })}
    </ol>
  );
}

function RunDetail({ run }: { run: VideoRun }) {
  const cancel = useCancelVideoRun();
  const now = useNow();
  const active = isVideoRunActive(run.status);
  const percent = Math.round((run.progress ?? 0) * 100);
  const portrait = run.brief?.format === "portrait";

  return (
    <div className="min-w-0 space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="truncate text-lg font-semibold text-foreground">{run.title || run.topic}</h2>
          <p className="text-xs text-muted-foreground">
            {run.title && run.topic && run.title !== run.topic ? `${run.topic} · ` : ""}
            Started {new Date(run.created_at).toLocaleString()}
            {run.task_id && (
              <>
                {" · "}
                <Link href={`/panel/tasks?task=${run.task_id}`} className="text-primary hover:underline">
                  Open the task
                </Link>
              </>
            )}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <StatusBadge status={run.status} />
          {active && (
            <button
              type="button"
              disabled={cancel.isPending}
              onClick={() => {
                if (window.confirm("Cancel this video? The workstation stops working on it.")) {
                  cancel.mutate(run.id);
                }
              }}
              className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card px-3 py-1.5 text-xs font-medium hover:bg-muted disabled:opacity-50"
            >
              {cancel.isPending ? <Loader2 className="h-3 w-3 animate-spin" /> : <XCircle className="h-3 w-3" />}
              Cancel
            </button>
          )}
        </div>
      </header>

      {run.status === "done" && run.video_url && (
        <section className="space-y-2">
          <video
            key={run.video_url}
            controls
            preload="metadata"
            src={run.video_url}
            className={cn(
              "w-full rounded-lg border border-border bg-black",
              portrait ? "mx-auto max-h-[70vh] max-w-sm" : "max-h-[70vh]"
            )}
          />
          <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
            <span>
              {formatClock(run.duration_seconds)} · {run.shots.length} shots ·{" "}
              {run.motion_model ? `animated with ${run.motion_model}` : "still pictures with camera moves"}
            </span>
            <a
              href={run.video_url}
              download
              className="inline-flex items-center gap-1 font-medium text-primary hover:underline"
            >
              <Download className="h-3.5 w-3.5" /> Download MP4
            </a>
          </div>
        </section>
      )}

      {active && (
        <section className="space-y-2 rounded-lg border border-border bg-card/50 p-4">
          <div className="flex items-center justify-between gap-3 text-sm">
            <span className="font-medium text-foreground">{run.message || "Working"}</span>
            {run.status === "rendering" && <span className="text-xs text-muted-foreground">{percent}%</span>}
          </div>
          <div className="h-2 overflow-hidden rounded-full bg-muted">
            <div
              className={cn(
                "h-full rounded-full bg-signal-live transition-all",
                run.status !== "rendering" && "animate-pulse"
              )}
              style={{ width: `${run.status === "rendering" ? Math.max(percent, 2) : 100}%`, opacity: run.status === "rendering" ? 1 : 0.25 }}
            />
          </div>
          <p className="text-xs text-muted-foreground">{timing(run, now)}</p>
        </section>
      )}

      {run.status === "failed" && (
        <p className="flex items-start gap-2 rounded-md border border-signal-error/30 bg-signal-error/5 px-3 py-2 text-sm text-signal-error">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          {run.error || run.message || "The video failed."}
        </p>
      )}

      <section className="space-y-3">
        <h3 className="text-sm font-semibold text-foreground">What the agent is doing</h3>
        <Pipeline run={run} />
        {!active && <p className="text-xs text-muted-foreground">{timing(run, now)}</p>}
      </section>

      {run.notes?.length > 0 && (
        <section className="space-y-1">
          <h3 className="text-sm font-semibold text-foreground">Notes</h3>
          <ul className="list-disc space-y-1 pl-5 text-xs text-muted-foreground">
            {run.notes.map((n) => (
              <li key={n}>{n}</li>
            ))}
          </ul>
        </section>
      )}

      <section className="space-y-2">
        <h3 className="text-sm font-semibold text-foreground">Script</h3>
        {run.shots.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {run.status === "scripting"
              ? "The script appears here as soon as it is written."
              : "No script was recorded for this video."}
          </p>
        ) : (
          <ol className="space-y-3">
            {run.shots.map((shot, i) => (
              <li key={i} className="rounded-md border border-border bg-card/50 p-3">
                <p className="text-sm text-foreground">
                  <span className="mr-2 font-mono text-xs text-muted-foreground">{i + 1}</span>
                  {shot.narration}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">Picture: {shot.visual}</p>
              </li>
            ))}
          </ol>
        )}
      </section>

      <section className="space-y-2">
        <h3 className="text-sm font-semibold text-foreground">Brief</h3>
        <dl className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm sm:grid-cols-3">
          {[
            ["Length", run.brief?.seconds ? `${run.brief.seconds} s` : "—"],
            ["Format", run.brief?.format === "portrait" ? "Portrait 9:16" : "Landscape 16:9"],
            ["Pictures", run.brief?.motion ? "Animated" : "Stills with camera moves"],
            ["Voice", VOICE_LABELS[run.brief?.voice] ?? run.brief?.voice ?? "—"],
            ["Captions", run.brief?.captions ? "Burned in" : "None"],
            ["Style", run.brief?.style || "Chosen by the script"],
          ].map(([k, v]) => (
            <div key={k} className="min-w-0">
              <dt className="text-xs text-muted-foreground">{k}</dt>
              <dd className="truncate text-foreground">{v}</dd>
            </div>
          ))}
        </dl>
      </section>
    </div>
  );
}

/**
 * The Video Producer's Workspace tab: every video, what the agent is doing on
 * each right now, and the finished film. New videos start from New task /
 * Run task / chat, which use the one server-side launch schema.
 */
export function VideoProducerClient() {
  const search = useSearchParams();
  const runParam = search.get("run");
  const agentId = useParams<{ id?: string }>()?.id;
  const { data: runs = [], isLoading, isError } = useVideoRuns();
  const [selected, setSelected] = useState<string>(runParam ?? "");

  useEffect(() => {
    if (runParam) setSelected(runParam);
  }, [runParam]);

  if (isLoading) {
    return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />;
  }
  if (isError) {
    return (
      <p className="rounded-md border border-signal-error/30 bg-signal-error/5 px-3 py-2 text-sm text-signal-error">
        Could not load videos. Refresh to try again.
      </p>
    );
  }
  if (runs.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
        <Clapperboard className="mx-auto mb-2 h-6 w-6" />
        No videos yet. Start one from{" "}
        <Link href={newTaskHref(agentId)} className="font-medium text-primary hover:underline">
          New task
        </Link>{" "}
        with the Video Producer, or ask in chat: &ldquo;make a 30-second video about …&rdquo;. Its progress shows
        here, step by step.
      </div>
    );
  }

  const current = runs.find((r) => r.id === selected) ?? runs[0];
  return (
    <div className="grid gap-6 lg:grid-cols-[280px_1fr]">
      <ul className="space-y-1" aria-label="Videos">
        {runs.map((r) => (
          <li key={r.id}>
            <button
              type="button"
              onClick={() => setSelected(r.id)}
              className={cn(
                "w-full space-y-1 rounded-md px-3 py-2 text-left",
                current.id === r.id ? "bg-secondary" : "hover:bg-secondary/60"
              )}
            >
              <span className="flex items-center justify-between gap-2">
                <span className="truncate text-sm font-medium">{r.title || r.topic}</span>
                <StatusBadge status={r.status} />
              </span>
              {r.status === "rendering" && (
                <span className="block h-1 overflow-hidden rounded-full bg-muted">
                  <span
                    className="block h-full rounded-full bg-signal-live"
                    style={{ width: `${Math.max(Math.round(r.progress * 100), 2)}%` }}
                  />
                </span>
              )}
              <span className="block truncate text-xs text-muted-foreground">
                {isVideoRunActive(r.status) && r.message ? r.message : new Date(r.created_at).toLocaleString()}
              </span>
            </button>
          </li>
        ))}
      </ul>
      <RunDetail key={current.id} run={current} />
    </div>
  );
}
