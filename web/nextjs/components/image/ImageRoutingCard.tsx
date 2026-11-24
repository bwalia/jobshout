"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Save } from "lucide-react";
import { toast } from "sonner";
import { getImageRouting, putImageRouting } from "@/lib/api/images";
import { apiErrorMessage } from "@/lib/api/client";
import { ImageModelPicker } from "@/components/image/ImageModelPicker";
import type { ImageRoute } from "@/lib/types/image";

/**
 * The org's image model per task: which provider and model draw article
 * covers, figures, email heroes and so on. A task left on "Platform default"
 * uses the server's default; Gemini still falls back to the workstation.
 */
export function ImageRoutingCard() {
  const queryClient = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ["image-routing"], queryFn: getImageRouting });
  const [routes, setRoutes] = useState<Record<string, ImageRoute>>({});

  useEffect(() => {
    if (data) setRoutes(data.routes);
  }, [data]);

  const save = useMutation({
    mutationFn: () => putImageRouting(routes),
    onSuccess: (r) => {
      queryClient.setQueryData(["image-routing"], r);
      toast.success("Image models saved");
    },
    onError: (err) => toast.error(`Could not save: ${apiErrorMessage(err, "unknown error")}`),
  });

  return (
    <section className="rounded-xl border border-border bg-card p-6">
      <h2 className="text-base font-semibold">Image model per task</h2>
      <p className="mt-1 max-w-2xl text-xs text-muted-foreground">
        Which model draws each kind of image. Hosted providers appear once their key is in Vault.
        A task on its platform default — or whose provider is no longer configured — uses the
        server&apos;s default model.
      </p>

      {isLoading ? (
        <div className="flex justify-center py-6">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <div className="mt-4 space-y-3">
          {data?.tasks.map((t) => (
            <div key={t.source} className="grid items-center gap-2 sm:grid-cols-[14rem_1fr]">
              <label htmlFor={`image-route-${t.source}`} className="text-sm font-medium">
                {t.label}
              </label>
              <ImageModelPicker
                id={`image-route-${t.source}`}
                value={{ provider: routes[t.source]?.provider ?? "", model: routes[t.source]?.model ?? "" }}
                onChange={({ provider, model }) => {
                  const next = { ...routes };
                  if (provider) next[t.source] = { provider, model };
                  else delete next[t.source];
                  setRoutes(next);
                }}
              />
            </div>
          ))}
          <div className="flex justify-end pt-2">
            <button
              type="button"
              onClick={() => save.mutate()}
              disabled={save.isPending}
              className="inline-flex h-9 items-center gap-2 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {save.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
              Save image models
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
