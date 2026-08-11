"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useGenerateBlog } from "@/lib/hooks/useBlog";

/** Matches blog.HardMaxArticles on the server, which truncates beyond this. */
const HARD_MAX_ARTICLES = 10;

/**
 * Briefs the Article Writer. One topic per line — each becomes its own article
 * and its own file in the content repository.
 */
export function GenerateArticleDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const router = useRouter();
  const generate = useGenerateBlog();
  const [topicsText, setTopicsText] = useState("");
  const [model, setModel] = useState("");

  const topics = topicsText
    .split("\n")
    .map((t) => t.trim())
    .filter(Boolean);

  const tooMany = topics.length > HARD_MAX_ARTICLES;

  if (!open) return null;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (topics.length === 0 || tooMany) return;

    generate.mutate(
      { topics, model: model.trim() || undefined },
      {
        onSuccess: (run) => {
          setTopicsText("");
          setModel("");
          onClose();
          // Go straight to the run so the user watches it work rather than
          // wondering whether anything happened.
          router.push(`/articles/${run.id}`);
        },
      }
    );
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-lg space-y-4 rounded-lg border border-border bg-card p-6 shadow-xl"
      >
        <div>
          <h2 className="text-lg font-semibold text-foreground">
            Write articles
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            One topic per line. The Article Writer produces a markdown article
            for each; nothing is published until you say so.
          </p>
        </div>

        <div>
          <label
            htmlFor="topics"
            className="text-xs text-muted-foreground"
          >
            Topics
          </label>
          <textarea
            id="topics"
            rows={5}
            autoFocus
            value={topicsText}
            onChange={(e) => setTopicsText(e.target.value)}
            placeholder={"Debugging Kubernetes CrashLoopBackOff\nDesigning idempotent migrations"}
            className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm text-foreground placeholder:text-muted-foreground/60"
          />
          <p className="mt-1 text-2xs text-muted-foreground">
            {topics.length} topic{topics.length === 1 ? "" : "s"}
            {tooMany && (
              <span className="text-destructive">
                {" "}
                — maximum is {HARD_MAX_ARTICLES}
              </span>
            )}
          </p>
        </div>

        <div>
          <label htmlFor="model" className="text-xs text-muted-foreground">
            Model <span className="text-muted-foreground/60">(optional)</span>
          </label>
          <input
            id="model"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            placeholder="Leave blank to use the configured default"
            className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm text-foreground placeholder:text-muted-foreground/60"
          />
        </div>

        <div className="flex items-center justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-border px-4 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={generate.isPending || topics.length === 0 || tooMany}
            className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
          >
            {generate.isPending ? "Starting..." : "Write"}
          </button>
        </div>
      </form>
    </div>
  );
}
