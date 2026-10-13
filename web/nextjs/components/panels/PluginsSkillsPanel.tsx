"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { cn } from "@/lib/utils/cn";
import { PluginsView } from "@/components/plugins/PluginsView";
import { SkillsView } from "@/components/skills/SkillsView";

type Tab = "plugins" | "skills";

export function PluginsSkillsPanel() {
  // The tab lives in ?tab= so a link can open Skills directly.
  const router = useRouter();
  const pathname = usePathname();
  const tab: Tab = useSearchParams().get("tab") === "skills" ? "skills" : "plugins";
  const setTab = (t: Tab) =>
    router.replace(t === "plugins" ? pathname : `${pathname}?tab=${t}`, { scroll: false });

  return (
    <div className="space-y-4 p-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Skills & plugins</h1>
        <p className="text-sm text-muted-foreground">
          A skill is a tool or prompt pattern you switch on for an agent to shape how it works; a
          plugin is a packaged workflow that runs on its own, sandboxed, when you execute it.
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
