"use client";

import { CheckCircle2 } from "lucide-react";
import type { CourseQuizQuestion } from "@/lib/types/course";
import { cn } from "@/lib/utils/cn";

/**
 * Reviewer view of a chapter quiz, answers shown. This is JobShout-only: the
 * quiz is never rendered into lesson HTML, so learners never see the key.
 */
export function QuizPreview({ questions }: { questions: CourseQuizQuestion[] }) {
  return (
    <ol className="space-y-4">
      {questions.map((q, qi) => (
        <li key={qi} className="rounded-lg border border-border bg-card p-4">
          <p className="text-sm font-medium">
            {qi + 1}. {q.question}
          </p>
          <ul className="mt-2 space-y-1">
            {q.options.map((opt, oi) => {
              const correct = oi === q.correct_index;
              return (
                <li
                  key={oi}
                  className={cn(
                    "flex items-start gap-2 rounded-md px-2 py-1 text-sm",
                    correct
                      ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
                      : "text-muted-foreground"
                  )}
                >
                  <span className="w-4 shrink-0">
                    {correct ? <CheckCircle2 className="h-4 w-4" aria-label="Correct answer" /> : null}
                  </span>
                  <span>{opt}</span>
                </li>
              );
            })}
          </ul>
          <p className="mt-2 text-xs text-muted-foreground">{q.explanation}</p>
        </li>
      ))}
    </ol>
  );
}
