"use client";

import { Suspense } from "react";
import { notFound, useParams } from "next/navigation";
import { DashboardPanel } from "@/components/panels/DashboardPanel";
import { ProjectsPanel } from "@/components/panels/ProjectsPanel";
import { TaskBoardPanel } from "@/components/panels/TaskBoardPanel";
import { TaskManagerPanel } from "@/components/panels/TaskManagerPanel";
import { ArtifactsPanel } from "@/components/panels/ArtifactsPanel";
import { PluginsSkillsPanel } from "@/components/panels/PluginsSkillsPanel";
import { SecurityTesterPanel } from "@/components/panels/SecurityTesterPanel";
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
    case "task-board":
      return <TaskBoardPanel />;
    case "task-manager":
      return <TaskManagerPanel />;
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
