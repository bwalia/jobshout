"use client";

import { useParams } from "next/navigation";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { getProject } from "@/lib/api/projects";
import { KanbanBoard } from "@/components/kanban/KanbanBoard";

/**
 * Project detail page.
 *
 * Fetches the project metadata (name, description) and renders the full
 * Kanban board for that project beneath a breadcrumb header.
 */
export default function ProjectDetailPage() {
  const params = useParams();
  // Next.js dynamic segments are always string | string[]; we normalise here
  const projectId = Array.isArray(params.id) ? params.id[0] : (params.id ?? "");

  const { data: project, isLoading, isError } = useQuery({
    queryKey: ["projects", projectId],
    queryFn: () => getProject(projectId),
    enabled: Boolean(projectId),
  });

  return (
    <div className="flex h-[calc(100vh-4rem)] flex-col gap-4">
      {/* Breadcrumb + page header */}
      <div className="flex items-center justify-between">
        <div className="space-y-0.5">
          {/* Breadcrumb */}
          <nav className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <Link href="/projects" className="hover:text-foreground">
              Projects
            </Link>
            <span>/</span>
            {isLoading ? (
              <span className="h-4 w-32 animate-pulse rounded bg-muted" />
            ) : (
              <span className="text-foreground font-medium">
                {project?.name ?? "Unknown project"}
              </span>
            )}
          </nav>

          {/* Description */}
          {project?.description && (
            <p className="text-sm text-muted-foreground">
              {project.description}
            </p>
          )}
        </div>
      </div>

      {/* Board area */}
      <div className="flex-1 overflow-hidden">
        {isLoading && (
          <div className="flex h-full items-center justify-center">
            <div className="flex flex-col items-center gap-3 text-muted-foreground">
              <svg
                className="h-8 w-8 animate-spin"
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle
                  className="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  strokeWidth="4"
                />
                <path
                  className="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                />
              </svg>
              <span className="text-sm">Loading project…</span>
            </div>
          </div>
        )}

        {isError && (
          <div className="flex h-full items-center justify-center">
            <div className="rounded-xl border border-destructive/50 bg-destructive/10 p-8 text-center">
              <p className="font-medium text-destructive">
                Failed to load project
              </p>
              <p className="mt-1 text-sm text-muted-foreground">
                Please refresh the page or go back to Projects.
              </p>
              <Link
                href="/projects"
                className="mt-4 inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
              >
                Back to Projects
              </Link>
            </div>
          </div>
        )}

        {!isLoading && !isError && project && (
          <KanbanBoard projectId={project.id} />
        )}
      </div>
    </div>
  );
}
