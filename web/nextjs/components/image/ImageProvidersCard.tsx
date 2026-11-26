"use client";

import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Loader2 } from "lucide-react";
import { getImageProviders } from "@/lib/api/images";
import { cn } from "@/lib/utils/cn";
import type { ImageProviderStatus } from "@/lib/types/image";

const STATUS_LABEL: Record<ImageProviderStatus["status"], string> = {
  available: "Available",
  not_configured: "Not configured",
  disabled: "Disabled",
};

const STATUS_STYLE: Record<ImageProviderStatus["status"], string> = {
  available: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  not_configured: "bg-muted text-muted-foreground",
  disabled: "bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400",
};

/**
 * Built-in image providers: Nano Banana, FLUX, Midjourney and the rest. Like
 * the text providers they are set up in the server's settings, so this only
 * reports state: it never shows a key, and says what to set when one is missing.
 */
export function ImageProvidersCard() {
  const { data, isLoading } = useQuery({ queryKey: ["image-providers"], queryFn: getImageProviders });

  return (
    <section className="rounded-xl border border-border bg-card p-6">
      <h2 className="text-base font-semibold">Built-in image providers</h2>
      <p className="mt-1 max-w-2xl text-xs text-muted-foreground">
        Draw blog covers and other pictures. Keys and model names are server settings, not edited here.
        {data && data.fallback_order.length > 0 && (
          <> If one fails or is rate limited, the next is tried: {data.fallback_order.join(" → ")}.</>
        )}
      </p>
      {isLoading ? (
        <div className="flex justify-center py-6">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {data?.providers.map((p) => (
            <div key={p.provider} className="rounded-lg border border-border bg-background p-4">
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-sm font-bold uppercase text-primary">
                  {p.label.charAt(0)}
                </div>
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{p.label}</p>
                  <p className="text-xs text-muted-foreground">
                    Image{p.default ? " · Default" : ""}
                    {p.model ? ` · ${p.model}` : ""}
                    {p.host ? ` · ${p.host}` : ""}
                  </p>
                </div>
              </div>
              <div className="mt-3">
                <span className={cn("rounded-full px-2 py-0.5 text-xs font-medium", STATUS_STYLE[p.status])}>
                  {STATUS_LABEL[p.status]}
                </span>
                {p.status !== "available" && p.hint && (
                  <span className="ml-2 text-xs text-muted-foreground">{p.hint}</span>
                )}
              </div>
              {p.warning && (
                <p className="mt-3 flex items-start gap-1.5 text-xs text-amber-700 dark:text-amber-400">
                  <AlertTriangle className="mt-0.5 h-3 w-3 shrink-0" /> {p.warning}
                </p>
              )}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
