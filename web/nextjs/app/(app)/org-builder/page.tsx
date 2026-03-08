"use client";

import { OrgChart } from "@/components/orgchart/OrgChart";
import { useAgents } from "@/lib/hooks/useAgents";

/**
 * Org Builder page — renders the full React Flow org chart canvas.
 *
 * Fetches the agent list via useAgents and passes it down to OrgChart so the
 * chart can be initialised with the current manager hierarchy.
 */
export default function OrgBuilderPage() {
  const { data, isLoading, isError } = useAgents();
  const agents = data?.data ?? [];

  return (
    <div className="flex h-[calc(100vh-4rem)] flex-col gap-4">
      {/* Page header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">
            Organisation Builder
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Drag to connect agents and define your reporting hierarchy.
          </p>
        </div>

        {/* Agent count badge */}
        {!isLoading && (
          <span className="inline-flex items-center rounded-full border border-border bg-card px-3 py-1 text-xs font-medium text-muted-foreground">
            {agents.length} agent{agents.length !== 1 ? "s" : ""}
          </span>
        )}
      </div>

      {/* Canvas area */}
      <div className="flex-1 overflow-hidden">
        {isLoading && (
          <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-border">
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
              <span className="text-sm">Loading agents…</span>
            </div>
          </div>
        )}

        {isError && (
          <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-destructive/50">
            <div className="flex flex-col items-center gap-2 text-center">
              <p className="font-medium text-destructive">
                Failed to load agents
              </p>
              <p className="text-sm text-muted-foreground">
                Please refresh the page or try again later.
              </p>
            </div>
          </div>
        )}

        {!isLoading && !isError && agents.length === 0 && (
          <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-border">
            <div className="flex flex-col items-center gap-2 text-center">
              <p className="font-medium">No agents yet</p>
              <p className="text-sm text-muted-foreground">
                Create agents first, then come back to build your org chart.
              </p>
            </div>
          </div>
        )}

        {!isLoading && !isError && agents.length > 0 && (
          <OrgChart agents={agents} />
        )}
      </div>
    </div>
  );
}
