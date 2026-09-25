/** Course Generator wire types (server: internal/model/course.go). */

export type CourseRunStatus = "queued" | "running" | "completed" | "failed" | "cancelled";

export interface CourseBrief {
  topic: string;
  audience?: string;
  level: string;
  chapter_count: number;
  locale: string;
  context?: string;
  seed_urls?: string[];
  focus?: string[];
}

export interface CourseRunStep {
  key: string;
  status: "pending" | "running" | "done" | "failed" | "skipped";
  detail?: string;
}

export interface CourseOutlineChapter {
  title: string;
  summary: string;
  objectives: string[];
}

export interface CourseOutline {
  title: string;
  description: string;
  level: string;
  category: string;
  tags: string[];
  learning_outcomes: string[];
  chapters: CourseOutlineChapter[];
}

export interface CourseSource {
  url: string;
  title?: string;
}

export interface CourseRun {
  id: string;
  agent_id: string;
  task_id: string | null;
  org_id: string;
  status: CourseRunStatus;
  brief: CourseBrief;
  outline?: CourseOutline;
  steps: CourseRunStep[];
  cover_url?: string;
  sources?: CourseSource[];
  warnings?: string[];
  error_message?: string | null;
  started_at: string | null;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface CourseImage {
  url: string;
  alt: string;
  caption?: string;
}

export interface CourseQuizQuestion {
  question: string;
  options: string[];
  correct_index: number;
  explanation: string;
}

export interface CourseChapter {
  id: string;
  run_id: string;
  locale: string;
  position: number;
  title: string;
  summary: string;
  objectives: string[];
  markdown: string;
  html: string;
  images: CourseImage[];
  quiz?: { questions: CourseQuizQuestion[] } | null;
}

export function isCourseRunActive(status: CourseRunStatus): boolean {
  return status === "queued" || status === "running";
}
