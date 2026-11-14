"use client";

import { BrandLockup } from "@/components/brand/BrandMark";
import { useHealth } from "@/lib/hooks/useHealth";
import { formatDeployIdentity } from "@/lib/deploy-identity";

export function LoginBrand() {
  const health = useHealth();
  const stamp = formatDeployIdentity(health.data) || "JobShout";

  return (
    <div className="hidden lg:flex lg:w-1/2 lg:flex-col lg:justify-between bg-sidebar p-10">
      <BrandLockup />
      <div className="max-w-md">
        <h2 className="text-3xl font-bold tracking-tight text-foreground">
          AI Team Command Center
        </h2>
        <p className="mt-3 text-base text-muted-foreground">
          Mission control for AI teams. Create agents, build teams, assign
          projects, track work, and automate workflows.
        </p>
      </div>
      <p className="font-mono text-xs text-muted-foreground">{stamp}</p>
    </div>
  );
}
