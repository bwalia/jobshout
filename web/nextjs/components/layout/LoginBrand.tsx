"use client";

import { Zap } from "lucide-react";
import { useHealth } from "@/lib/hooks/useHealth";

function formatDeployed(iso?: string): string | null {
  if (!iso) return null;
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return null;
  const secs = Math.round((Date.now() - then) / 1000);
  if (secs < 60) return "just now";
  const mins = Math.round(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.round(hrs / 24);
  return `${days}d ago`;
}

export function LoginBrand() {
  const health = useHealth();
  const version = health.data?.version;
  const deployed = formatDeployed(health.data?.deployed_at);
  const stamp = [
    version ? `Jobshout ${version}` : null,
    health.data?.env,
    deployed ? `Deployed ${deployed}` : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <div className="hidden lg:flex lg:w-1/2 lg:flex-col lg:justify-between bg-sidebar p-10">
      <div className="flex items-center gap-2.5">
        <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary text-primary-foreground">
          <Zap className="h-5 w-5" />
        </span>
        <span className="text-lg font-semibold tracking-tight text-foreground">
          Jobshout
        </span>
      </div>
      <div className="max-w-md">
        <h2 className="text-3xl font-bold tracking-tight text-foreground">
          AI Team Command Center
        </h2>
        <p className="mt-3 text-base text-muted-foreground">
          Mission control for AI teams. Create agents, build teams, assign
          projects, track work, and automate workflows.
        </p>
      </div>
      <p className="text-xs text-muted-foreground">{stamp || "Jobshout"}</p>
    </div>
  );
}
