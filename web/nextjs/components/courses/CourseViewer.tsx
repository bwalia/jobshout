"use client";

import { useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Loader2, XCircle } from "lucide-react";

import { QuizPreview } from "@/components/courses/QuizPreview";
import { StoredImage } from "@/components/image/StoredImage";
import { useCancelCourseRun, useCourseChapters, useCourseRun } from "@/lib/hooks/useCourses";
import { isCourseRunActive, type CourseChapter, type CourseRunStep } from "@/lib/types/course";
import { cn } from "@/lib/utils/cn";

const STEP_LABELS: Record<string, string> = {
  researching: "Research",
  outlining: "Outline",
  writing: "Write theory",
  reviewing: "Review",
  illustrating: "Visuals",
  quizzing: "Quiz",
  saved: "Saved",
};

function StepList({ steps }: { steps: CourseRunStep[] }) {
  return (
    <ol className="flex flex-wrap gap-2">
      {steps.map((s) => (
        <li
          key={s.key}
          title={s.detail}
          className={cn(
            "rounded-full border px-2.5 py-0.5 text-xs",
            s.status === "done" && "border-emerald-500/40 text-emerald-700 dark:text-emerald-300",
            s.status === "running" && "border-primary text-primary",
            s.status === "failed" && "border-destructive text-destructive",
            (s.status === "pending" || s.status === "skipped") && "border-border text-muted-foreground"
          )}
        >
          {STEP_LABELS[s.key] ?? s.key}
          {s.status === "running" && s.detail ? ` · ${s.detail}` : ""}
          {s.status === "skipped" ? " (skipped)" : ""}
        </li>
      ))}
    </ol>
  );
}

function ChapterBody({ chapter }: { chapter: CourseChapter }) {
  const [tab, setTab] = useState<"theory" | "quiz">("theory");
  const questions = chapter.quiz?.questions ?? [];
  return (
    <div className="space-y-4">
      <div className="flex gap-2 border-b border-border">
        {(["theory", "quiz"] as const).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setTab(t)}
            className={cn(
              "-mb-px border-b-2 px-3 py-1.5 text-sm",
              tab === t ? "border-primary font-medium" : "border-transparent text-muted-foreground"
            )}
          >
            {t === "theory" ? "Theory" : `Quiz (${questions.length})`}
          </button>
        ))}
      </div>
      {tab === "theory" ? (
        <article className="prose prose-sm max-w-none dark:prose-invert prose-pre:overflow-x-auto">
          {/* ReactMarkdown ignores raw HTML, so model output cannot inject markup here. */}
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{chapter.markdown}</ReactMarkdown>
          {chapter.images.map((img) => (
            <figure key={img.url}>
              <StoredImage
                src={img.url}
                alt={img.alt}
                loading="lazy"
                className="h-auto max-w-full rounded-lg border border-border"
              />
              {img.caption ? <figcaption>{img.caption}</figcaption> : null}
            </figure>
          ))}
        </article>
      ) : questions.length > 0 ? (
        <QuizPreview questions={questions} />
      ) : (
        <p className="text-sm text-muted-foreground">No quiz was generated for this chapter.</p>
      )}
    </div>
  );
}

export function CourseViewer({ runId }: { runId: string }) {
  const { data: run, isLoading } = useCourseRun(runId);
  const active = run ? isCourseRunActive(run.status) : false;
  const { data: chapters = [] } = useCourseChapters(runId, active);
  const cancel = useCancelCourseRun();
  const [selected, setSelected] = useState(0);

  if (isLoading || !run) {
    return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />;
  }
  const chapter = chapters[Math.min(selected, Math.max(chapters.length - 1, 0))];
  const outline = run.outline;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-lg font-semibold tracking-tight">{outline?.title || run.brief.topic}</h3>
          <p className="text-xs text-muted-foreground">
            {run.brief.level} · {run.brief.chapter_count} chapters · {run.brief.locale} · {run.status}
          </p>
        </div>
        {active ? (
          <button
            type="button"
            onClick={() => cancel.mutate(run.id)}
            disabled={cancel.isPending}
            className="inline-flex h-8 items-center gap-1.5 rounded-md border border-border px-3 text-sm hover:bg-secondary disabled:opacity-50"
          >
            <XCircle className="h-3.5 w-3.5" /> Cancel
          </button>
        ) : null}
      </div>

      <StepList steps={run.steps} />

      {run.error_message ? (
        <p className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
          {run.error_message}
        </p>
      ) : null}

      {outline ? (
        <div className="grid gap-4 md:grid-cols-[1fr_auto]">
          <div className="space-y-2 text-sm">
            <p>{outline.description}</p>
            {outline.learning_outcomes.length > 0 ? (
              <ul className="list-disc pl-5 text-muted-foreground">
                {outline.learning_outcomes.map((o) => (
                  <li key={o}>{o}</li>
                ))}
              </ul>
            ) : null}
            {outline.tags.length > 0 ? (
              <p className="text-xs text-muted-foreground">
                {outline.category} · {outline.tags.join(", ")}
              </p>
            ) : null}
          </div>
          {run.cover_url ? (
            <StoredImage
              src={run.cover_url}
              alt={`${outline.title} cover`}
              className="h-auto w-full max-w-xs rounded-lg border border-border"
            />
          ) : null}
        </div>
      ) : null}

      {chapters.length > 0 ? (
        <div className="grid gap-5 md:grid-cols-[220px_1fr]">
          <nav aria-label="Chapters" className="space-y-1">
            {chapters.map((ch, i) => (
              <button
                key={ch.id}
                type="button"
                onClick={() => setSelected(i)}
                className={cn(
                  "block w-full rounded-md px-2 py-1.5 text-left text-sm",
                  chapter?.id === ch.id ? "bg-secondary font-medium" : "hover:bg-secondary/60"
                )}
              >
                {ch.position}. {ch.title}
              </button>
            ))}
            {active && outline && chapters.length < outline.chapters.length ? (
              <p className="px-2 text-xs text-muted-foreground">
                Writing {chapters.length + 1} of {outline.chapters.length}…
              </p>
            ) : null}
          </nav>
          {chapter ? <ChapterBody key={chapter.id} chapter={chapter} /> : null}
        </div>
      ) : active ? (
        <p className="text-sm text-muted-foreground">Chapters appear here as they are written.</p>
      ) : null}

      {run.warnings && run.warnings.length > 0 ? (
        <details className="text-xs text-muted-foreground">
          <summary>{run.warnings.length} warning(s)</summary>
          <ul className="mt-1 list-disc pl-5">
            {run.warnings.map((w, i) => (
              <li key={i}>{w}</li>
            ))}
          </ul>
        </details>
      ) : null}

      {run.sources && run.sources.length > 0 ? (
        <details className="text-xs text-muted-foreground">
          <summary>{run.sources.length} research source(s)</summary>
          <ul className="mt-1 list-disc pl-5">
            {run.sources.map((s) => (
              <li key={s.url}>
                <a href={s.url} target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">
                  {s.title || s.url}
                </a>
              </li>
            ))}
          </ul>
        </details>
      ) : null}
    </div>
  );
}
