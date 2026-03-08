"use client";

import { useState } from "react";
import { Search, Plus, SlidersHorizontal } from "lucide-react";
import { AgentCard } from "@/components/agent/AgentCard";
import { CreateAgentDialog } from "@/components/agent/CreateAgentDialog";
import { useAgents } from "@/lib/hooks/useAgents";
import type { AgentStatus } from "@/lib/types/common";

const STATUS_FILTERS: { label: string; value: AgentStatus | "all" }[] = [
  { label: "All", value: "all" },
  { label: "Active", value: "active" },
  { label: "Idle", value: "idle" },
  { label: "Paused", value: "paused" },
  { label: "Offline", value: "offline" },
];

export default function AgentsPage() {
  const [searchQuery, setSearchQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<AgentStatus | "all">("all");
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);

  const { data, isLoading, isError } = useAgents({
    status: statusFilter === "all" ? undefined : statusFilter,
    search: searchQuery.trim() || undefined,
    per_page: 50,
  });

  const agents = data?.data ?? [];

  return (
    <>
      <div className="space-y-6">
        {/* Page heading */}
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-foreground">
              Agents
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Manage your AI agents — create, configure, and monitor their
              performance.
            </p>
          </div>

          <button
            type="button"
            onClick={() => setIsCreateDialogOpen(true)}
            className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            <Plus className="h-4 w-4" />
            New Agent
          </button>
        </div>

        {/* Filter / search bar */}
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          {/* Search input */}
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <input
              type="search"
              placeholder="Search agents by name or role…"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="flex h-9 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          {/* Status filter pills */}
          <div className="flex items-center gap-1.5 flex-wrap">
            <SlidersHorizontal className="h-4 w-4 text-muted-foreground flex-shrink-0" />
            {STATUS_FILTERS.map(({ label, value }) => (
              <button
                key={value}
                type="button"
                onClick={() => setStatusFilter(value)}
                className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
                  statusFilter === value
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                }`}
              >
                {label}
              </button>
            ))}
          </div>
        </div>

        {/* Result count */}
        {!isLoading && !isError && (
          <p className="text-xs text-muted-foreground">
            {data?.total ?? 0} agent{(data?.total ?? 0) !== 1 ? "s" : ""} found
          </p>
        )}

        {/* Loading skeleton */}
        {isLoading && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {Array.from({ length: 8 }).map((_, i) => (
              <div
                key={i}
                className="h-40 animate-pulse rounded-lg bg-muted"
              />
            ))}
          </div>
        )}

        {/* Error state */}
        {isError && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-5 py-4 text-sm text-destructive">
            Failed to load agents. Please try refreshing the page.
          </div>
        )}

        {/* Empty state */}
        {!isLoading && !isError && agents.length === 0 && (
          <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-16 text-center">
            <p className="text-base font-medium text-foreground">
              No agents found
            </p>
            <p className="mt-1 text-sm text-muted-foreground">
              {searchQuery || statusFilter !== "all"
                ? "Try adjusting your search or filter."
                : "Create your first agent to get started."}
            </p>
            {!searchQuery && statusFilter === "all" && (
              <button
                type="button"
                onClick={() => setIsCreateDialogOpen(true)}
                className="mt-4 inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
              >
                <Plus className="h-4 w-4" />
                New Agent
              </button>
            )}
          </div>
        )}

        {/* Agent grid */}
        {!isLoading && !isError && agents.length > 0 && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {agents.map((agent) => (
              <AgentCard key={agent.id} agent={agent} />
            ))}
          </div>
        )}
      </div>

      {/* Create agent dialog – rendered outside the main flow so it overlays */}
      <CreateAgentDialog
        open={isCreateDialogOpen}
        onClose={() => setIsCreateDialogOpen(false)}
      />
    </>
  );
}
