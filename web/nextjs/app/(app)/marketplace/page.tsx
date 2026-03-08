"use client";

import { useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { MarketplaceCard } from "@/components/marketplace/MarketplaceCard";
import { CategoryFilter } from "@/components/marketplace/CategoryFilter";
import { ImportAgentDialog } from "@/components/marketplace/ImportAgentDialog";
import {
  getMarketplaceAgents,
  importMarketplaceAgent,
  type MarketplaceAgent,
} from "@/lib/api/marketplace";

const CATEGORIES = ["All", "Engineering", "Design", "QA", "Management", "DevOps"];

// Skeleton card shown while the agent list is loading
function AgentCardSkeleton() {
  return (
    <div className="flex flex-col rounded-xl border border-border bg-card p-5 shadow-sm animate-pulse">
      <div className="flex items-start justify-between">
        <div className="h-12 w-12 rounded-full bg-muted" />
        <div className="h-5 w-20 rounded-full bg-muted" />
      </div>
      <div className="mt-3 space-y-2">
        <div className="h-4 w-3/4 rounded bg-muted" />
        <div className="h-3 w-1/2 rounded bg-muted" />
      </div>
      <div className="mt-2 space-y-1.5 flex-1">
        <div className="h-3 w-full rounded bg-muted" />
        <div className="h-3 w-full rounded bg-muted" />
        <div className="h-3 w-2/3 rounded bg-muted" />
      </div>
      <div className="mt-4 h-3 w-1/3 rounded bg-muted" />
      <div className="mt-4 h-9 w-full rounded-md bg-muted" />
    </div>
  );
}

export default function MarketplacePage() {
  const [activeCategory, setActiveCategory] = useState("All");
  const [searchQuery, setSearchQuery] = useState("");
  // The agent pending import confirmation; null when the dialog is closed
  const [pendingImport, setPendingImport] = useState<MarketplaceAgent | null>(null);

  const {
    data: agents = [],
    isLoading,
    isError,
    error,
    refetch,
  } = useQuery({
    queryKey: ["marketplace-agents"],
    queryFn: () => getMarketplaceAgents(),
  });

  const importMutation = useMutation({
    mutationFn: (agentId: string) => importMarketplaceAgent(agentId),
    onSuccess: (_data, _agentId) => {
      toast.success(`"${pendingImport?.name}" has been added to your team.`);
      setPendingImport(null);
    },
    onError: (err: unknown) => {
      const message =
        err instanceof Error ? err.message : "Something went wrong. Please try again.";
      toast.error(`Import failed: ${message}`);
    },
  });

  // Client-side filtering applied on top of the live data
  const filteredAgents = agents.filter((agent) => {
    const matchesCategory =
      activeCategory === "All" || agent.category === activeCategory;
    const searchLower = searchQuery.toLowerCase();
    const matchesSearch =
      searchQuery === "" ||
      agent.name.toLowerCase().includes(searchLower) ||
      agent.model_name.toLowerCase().includes(searchLower) ||
      agent.description.toLowerCase().includes(searchLower);
    return matchesCategory && matchesSearch;
  });

  function handleImportRequest(agentId: string): void {
    const agent = agents.find((a) => a.id === agentId) ?? null;
    setPendingImport(agent);
  }

  function handleImportConfirm(): void {
    if (!pendingImport) return;
    importMutation.mutate(pendingImport.id);
  }

  function handleImportCancel(): void {
    // Prevent closing while the import request is in-flight
    if (importMutation.isPending) return;
    setPendingImport(null);
  }

  return (
    <div className="space-y-8">
      {/* Hero section */}
      <div className="rounded-xl border border-border bg-gradient-to-br from-primary/10 to-accent/10 px-8 py-10">
        <h1 className="text-3xl font-bold tracking-tight">
          AI Agent Marketplace
        </h1>
        <p className="mt-2 max-w-xl text-muted-foreground">
          Discover and import pre-built AI agents crafted by the community.
          One click to add them to your team.
        </p>

        {/* Search bar */}
        <div className="mt-6 max-w-md">
          <div className="relative">
            <svg
              className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M21 21l-4.35-4.35M17 11A6 6 0 1 1 5 11a6 6 0 0 1 12 0z"
              />
            </svg>
            <input
              type="search"
              placeholder="Search agents by name, model, or description..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="flex h-10 w-full rounded-md border border-input bg-background pl-9 pr-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
        </div>
      </div>

      {/* Category filter */}
      <CategoryFilter
        categories={CATEGORIES}
        activeCategory={activeCategory}
        onCategoryChange={setActiveCategory}
      />

      {/* Error state */}
      {isError && (
        <div className="flex flex-col items-center justify-center rounded-xl border border-destructive/40 bg-destructive/10 py-12 text-center">
          <p className="text-base font-medium text-destructive">
            Failed to load marketplace agents
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            {error instanceof Error ? error.message : "An unexpected error occurred."}
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-4 inline-flex h-9 items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Try again
          </button>
        </div>
      )}

      {/* Results count — only shown when data is available */}
      {!isLoading && !isError && (
        <p className="text-sm text-muted-foreground">
          Showing{" "}
          <span className="font-medium text-foreground">{filteredAgents.length}</span>{" "}
          agent{filteredAgents.length !== 1 ? "s" : ""}
          {activeCategory !== "All" && (
            <> in <span className="font-medium text-foreground">{activeCategory}</span></>
          )}
          {searchQuery && (
            <> matching &ldquo;<span className="font-medium text-foreground">{searchQuery}</span>&rdquo;</>
          )}
        </p>
      )}

      {/* Loading skeletons */}
      {isLoading && (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <AgentCardSkeleton key={i} />
          ))}
        </div>
      )}

      {/* Agent grid */}
      {!isLoading && !isError && filteredAgents.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {filteredAgents.map((agent) => (
            <MarketplaceCard
              key={agent.id}
              agent={agent}
              onImport={handleImportRequest}
            />
          ))}
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !isError && filteredAgents.length === 0 && (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border py-20 text-center">
          <p className="text-lg font-medium">No agents found</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Try adjusting your search or category filter.
          </p>
        </div>
      )}

      {/* Import confirmation dialog */}
      {pendingImport && (
        <ImportAgentDialog
          agentName={pendingImport.name}
          agentModelProvider={pendingImport.model_provider}
          agentModelName={pendingImport.model_name}
          agentCategory={pendingImport.category}
          agentDescription={pendingImport.description}
          isImporting={importMutation.isPending}
          onConfirm={handleImportConfirm}
          onCancel={handleImportCancel}
        />
      )}
    </div>
  );
}
