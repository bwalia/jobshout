"use client";

import { useState } from "react";
import { MarketplaceCard } from "@/components/marketplace/MarketplaceCard";
import { CategoryFilter } from "@/components/marketplace/CategoryFilter";
import { ImportAgentDialog } from "@/components/marketplace/ImportAgentDialog";

// Represents a marketplace agent listing (not the same as an org-owned Agent)
interface MarketplaceAgent {
  id: string;
  name: string;
  role: string;
  category: string;
  description: string;
  download_count: number;
  star_rating: number;
  author: string;
}

const MOCK_AGENTS: MarketplaceAgent[] = [
  {
    id: "1",
    name: "CodeReviewer Pro",
    role: "Senior Code Reviewer",
    category: "Engineering",
    description: "Automated code review agent with deep understanding of best practices, security vulnerabilities, and performance patterns.",
    download_count: 4821,
    star_rating: 4.8,
    author: "Jobshout Labs",
  },
  {
    id: "2",
    name: "DesignCritic",
    role: "UX Design Analyst",
    category: "Design",
    description: "Reviews Figma designs and provides actionable feedback on accessibility, consistency, and usability.",
    download_count: 2340,
    star_rating: 4.5,
    author: "DesignOps Team",
  },
  {
    id: "3",
    name: "QA Sentinel",
    role: "QA Automation Engineer",
    category: "QA",
    description: "Generates test cases, identifies edge cases, and writes automated test suites for your codebase.",
    download_count: 3105,
    star_rating: 4.7,
    author: "Jobshout Labs",
  },
  {
    id: "4",
    name: "SprintCoach",
    role: "Agile Project Manager",
    category: "Management",
    description: "Manages sprint ceremonies, tracks velocity, identifies blockers, and produces sprint reports automatically.",
    download_count: 1987,
    star_rating: 4.3,
    author: "AgilePlus",
  },
  {
    id: "5",
    name: "K8s Guardian",
    role: "DevOps Engineer",
    category: "DevOps",
    description: "Monitors Kubernetes clusters, diagnoses pod failures, and proposes infrastructure fixes.",
    download_count: 2755,
    star_rating: 4.6,
    author: "InfraTeam",
  },
  {
    id: "6",
    name: "TypeScriptMentor",
    role: "Frontend Engineer",
    category: "Engineering",
    description: "Specializes in TypeScript, React, and Next.js. Helps with type safety, component architecture, and performance tuning.",
    download_count: 5601,
    star_rating: 4.9,
    author: "Jobshout Labs",
  },
  {
    id: "7",
    name: "BrandVoice",
    role: "Brand Designer",
    category: "Design",
    description: "Ensures visual consistency across marketing materials by enforcing brand guidelines and suggesting corrections.",
    download_count: 1123,
    star_rating: 4.1,
    author: "CreativeHub",
  },
  {
    id: "8",
    name: "CI Optimizer",
    role: "DevOps Engineer",
    category: "DevOps",
    description: "Analyzes CI/CD pipelines and recommends optimizations to reduce build times and flaky tests.",
    download_count: 1890,
    star_rating: 4.4,
    author: "PipelinePros",
  },
];

const CATEGORIES = ["All", "Engineering", "Design", "QA", "Management", "DevOps"];

export default function MarketplacePage() {
  const [activeCategory, setActiveCategory] = useState("All");
  const [searchQuery, setSearchQuery] = useState("");
  // The agent pending import confirmation; null when dialog is closed
  const [pendingImport, setPendingImport] = useState<MarketplaceAgent | null>(null);

  const filteredAgents = MOCK_AGENTS.filter((agent) => {
    const matchesCategory =
      activeCategory === "All" || agent.category === activeCategory;
    const searchLower = searchQuery.toLowerCase();
    const matchesSearch =
      searchQuery === "" ||
      agent.name.toLowerCase().includes(searchLower) ||
      agent.role.toLowerCase().includes(searchLower) ||
      agent.description.toLowerCase().includes(searchLower);
    return matchesCategory && matchesSearch;
  });

  function handleImportRequest(agentId: string): void {
    const agent = MOCK_AGENTS.find((a) => a.id === agentId) ?? null;
    setPendingImport(agent);
  }

  function handleImportConfirm(): void {
    // TODO: call POST /api/v1/agents/import with pendingImport details
    setPendingImport(null);
  }

  function handleImportCancel(): void {
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
              placeholder="Search agents by name, role, or description..."
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

      {/* Results count */}
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

      {/* Agent grid */}
      {filteredAgents.length > 0 ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {filteredAgents.map((agent) => (
            <MarketplaceCard
              key={agent.id}
              agent={agent}
              onImport={handleImportRequest}
            />
          ))}
        </div>
      ) : (
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
          agentRole={pendingImport.role}
          agentCategory={pendingImport.category}
          agentDescription={pendingImport.description}
          onConfirm={handleImportConfirm}
          onCancel={handleImportCancel}
        />
      )}
    </div>
  );
}
