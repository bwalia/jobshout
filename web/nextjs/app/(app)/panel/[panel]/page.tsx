"use client";

import { Suspense } from "react";
import { notFound, useParams } from "next/navigation";
import { DashboardPanel } from "@/components/panels/DashboardPanel";
import { ProjectsPanel } from "@/components/panels/ProjectsPanel";
import { TasksView } from "@/components/tasks/TasksView";
import { LegacyTaskRedirect } from "@/components/tasks/LegacyTaskRedirect";
import { ArtifactsPanel } from "@/components/panels/ArtifactsPanel";
import { PluginsSkillsPanel } from "@/components/panels/PluginsSkillsPanel";
import { SecurityTesterPanel } from "@/components/panels/SecurityTesterPanel";
import { PanelRedirect } from "@/components/layout/PanelRedirect";
import { PANELS, type PanelId } from "@/lib/panels";

import SchedulerPage from "@/app/(app)/scheduler/page";
import SprintsPage from "@/app/(app)/sprints/page";
import SessionsPage from "@/app/(app)/sessions/page";
import WorkflowsPage from "@/app/(app)/workflows/page";
import OrgBuilderPage from "@/app/(app)/org-builder/page";
import MarketplacePage from "@/app/(app)/marketplace/page";
import LLMProvidersPage from "@/app/(app)/llm-providers/page";
import SettingsPage from "@/app/(app)/settings/page";

const VALID = new Set(
  PANELS.map((p) => p.id).filter((id): id is Exclude<PanelId, "chat"> => id !== "chat")
);

export default function PanelPage() {
  const params = useParams<{ panel: string }>();
  const panel = params.panel;

  // An empty slug is the client hook hydrating, not a missing page. notFound()
  // here is sticky and is what made /panel/artifacts flash a 404 locally.
  if (!panel) {
    return (
      <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
        Loading…
      </div>
    );
  }
  if (!VALID.has(panel as Exclude<PanelId, "chat">)) notFound();

  return (
    <Suspense
      fallback={
        <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
          Loading…
        </div>
      }
    >
      <PanelBody panel={panel as Exclude<PanelId, "chat">} />
    </Suspense>
  );
}

function PanelBody({ panel }: { panel: Exclude<PanelId, "chat"> }) {
  switch (panel) {
    case "dashboard":
      return <DashboardPanel />;
    case "projects":
      return <ProjectsPanel />;
    case "tasks":
      return (
        <div className="flex h-full min-h-0 flex-col">
          <div className="px-6 pt-5">
            <h1 className="text-2xl">Tasks</h1>
            <p className="text-sm text-muted-foreground">
              Every task in one place. Filter, search, group by status, agent or project, and switch
              between a board and a list.
            </p>
          </div>
          <div className="min-h-0 flex-1">
            <TasksView />
          </div>
        </div>
      );
    case "task-board":
      return <LegacyTaskRedirect from="task-board" />;
    case "task-manager":
      return <LegacyTaskRedirect from="task-manager" />;
    case "agents":
      // /panel/agents is its own static route; this only satisfies the type.
      return null;
    case "security-tester":
      return (
        <div className="p-6">
          <SecurityTesterPanel />
        </div>
      );
    case "artifacts":
      return <ArtifactsPanel />;
    case "scheduler":
      return (
        <div className="p-6">
          <SchedulerPage />
        </div>
      );
    case "sprints":
      return (
        <div className="p-6">
          <SprintsPage />
        </div>
      );
    case "sessions":
      return (
        <div className="p-6">
          <SessionsPage />
        </div>
      );
    case "workflows":
      return (
        <div className="p-6">
          <WorkflowsPage />
        </div>
      );
    case "org-builder":
      return (
        <div className="h-full min-h-0 p-6">
          <OrgBuilderPage />
        </div>
      );
    case "marketplace":
      return (
        <div className="p-6">
          <MarketplacePage />
        </div>
      );
    case "plugins-skills":
      return <PluginsSkillsPanel />;
    case "llm-providers":
      return (
        <div className="p-6">
          <LLMProvidersPage />
        </div>
      );
    case "llm-benchmarks":
      // Benchmarks is a tab of Models & providers now.
      return <PanelRedirect to="/panel/llm-providers?tab=benchmarks" />;
    case "settings":
      return (
        <div className="p-6">
          <SettingsPage />
        </div>
      );
    default:
      notFound();
  }
}
