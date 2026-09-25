"use client";

import { useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { GraduationCap, Loader2 } from "lucide-react";

import { CourseViewer } from "@/components/courses/CourseViewer";
import { useCourseRuns } from "@/lib/hooks/useCourses";
import { cn } from "@/lib/utils/cn";

/**
 * Course Generator tab: run history and a reviewer view of each course.
 * New courses start from New task / Run task / chat, which use the one
 * server-side launch schema.
 */
export function CourseGeneratorClient() {
  const searchParams = useSearchParams();
  const runParam = searchParams.get("run");
  const { data, isLoading } = useCourseRuns({ per_page: 50 });
  const runs = data?.data ?? [];
  const firstRunId = data?.data?.[0]?.id ?? "";
  const [selected, setSelected] = useState<string>(runParam ?? "");

  useEffect(() => {
    if (runParam) setSelected(runParam);
  }, [runParam]);
  useEffect(() => {
    if (!selected && firstRunId) setSelected(firstRunId);
  }, [selected, firstRunId]);

  if (isLoading) {
    return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />;
  }
  if (runs.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
        <GraduationCap className="mx-auto mb-2 h-6 w-6" />
        No courses yet. Start one from <span className="font-medium">New task</span> with the
        Course Generator, or ask in chat: &ldquo;create a course on …&rdquo;.
      </div>
    );
  }

  return (
    <div className="grid gap-6 lg:grid-cols-[260px_1fr]">
      <ul className="space-y-1" aria-label="Course runs">
        {runs.map((r) => (
          <li key={r.id}>
            <button
              type="button"
              onClick={() => setSelected(r.id)}
              className={cn(
                "w-full rounded-md px-3 py-2 text-left text-sm",
                selected === r.id ? "bg-secondary" : "hover:bg-secondary/60"
              )}
            >
              <span className="block truncate font-medium">{r.outline?.title || r.brief.topic}</span>
              <span className="text-xs text-muted-foreground">
                {r.status} · {new Date(r.created_at).toLocaleDateString()}
              </span>
            </button>
          </li>
        ))}
      </ul>
      {selected ? <CourseViewer key={selected} runId={selected} /> : null}
    </div>
  );
}
