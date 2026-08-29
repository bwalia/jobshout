"use client";

import { Suspense } from "react";
import { ArtifactsPanel } from "@/components/panels/ArtifactsPanel";

export default function ArtifactsRoutePage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
          Loading…
        </div>
      }
    >
      <ArtifactsPanel />
    </Suspense>
  );
}
