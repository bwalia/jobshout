"use client";

import { useState } from "react";
import { cn } from "@/lib/utils/cn";
import { PluginsView } from "@/components/plugins/PluginsView";
import { SkillsView } from "@/components/skills/SkillsView";

type Tab = "plugins" | "skills";

export function PluginsSkillsPanel() {
  const [tab, setTab] = useState<Tab>("plugins");

  return (
    <div className="space-y-4 p-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Plugins & Skills</h1>
        <p className="text-sm text-muted-foreground">
          Capability bundles and executable plugins for your agents
        </p>
      </div>
      <div className="flex gap-1 border-b border-border">
        {(
          [
            { id: "plugins", label: "Plugins" },
            { id: "skills", label: "Skills" },
          ] as const
        ).map((t) => (
          <button
            key={t.id}
            type="button"
            onClick={() => setTab(t.id)}
            className={cn(
              "border-b-2 px-4 py-2 text-sm font-medium transition-colors",
              tab === t.id
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            )}
          >
            {t.label}
          </button>
        ))}
      </div>
      {tab === "plugins" ? (
        <PluginsView hideHeader />
      ) : (
        <SkillsView hideHeader />
      )}
    </div>
  );
}
