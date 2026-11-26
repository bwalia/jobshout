"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Loader2, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { apiErrorMessage, isRequestTimeout } from "@/lib/api/client";
import { regenerateBlogCover } from "@/lib/api/blog";
import { blogKeys } from "@/lib/hooks/useBlog";
import { ImageModelPicker, type ImageModelSelection } from "@/components/image/ImageModelPicker";
import type { BlogArticle } from "@/lib/types/blog";

/**
 * "Regenerate cover" for one post, with the image provider chosen for it. Left
 * on "Platform default" the server uses the org's routing and its fallback
 * order, so a failing provider falls through to the next one.
 */
export function RegenerateCoverControl({ article }: { article: BlogArticle }) {
  const qc = useQueryClient();
  const [choice, setChoice] = useState<ImageModelSelection>({ provider: "", model: "" });
  const [busy, setBusy] = useState(false);

  async function run() {
    setBusy(true);
    try {
      const updated = await regenerateBlogCover(article.id, choice);
      await qc.invalidateQueries({ queryKey: blogKeys.articles(article.run_id) });
      const by = updated.cover_image_meta?.provider;
      toast.success("Cover regenerated", { description: by ? `Drawn by ${by}.` : undefined });
    } catch (e) {
      if (isRequestTimeout(e)) {
        toast.info("Still drawing", {
          description: "The image service is slow. Reload this page in a few minutes to see the new cover.",
        });
      } else {
        toast.error(apiErrorMessage(e, "Could not regenerate the cover"));
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <ImageModelPicker value={choice} onChange={setChoice} disabled={busy} className="max-w-xs" />
      <button
        type="button"
        onClick={() => void run()}
        disabled={busy}
        title={article.post_uuid ? "Redraw the cover and update the CMS post" : "Redraw the cover"}
        className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50"
      >
        {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
        {busy ? "Drawing…" : "Regenerate cover"}
      </button>
    </div>
  );
}
