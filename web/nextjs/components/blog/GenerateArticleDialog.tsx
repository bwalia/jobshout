"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useGenerateBlog } from "@/lib/hooks/useBlog";
import type { BlogBrief } from "@/lib/types/blog";

/** Matches blog.HardMaxArticles on the server, which truncates beyond this. */
const HARD_MAX_ARTICLES = 10;

/** A brief plus the local id React needs to keep inputs stable across edits. */
interface BriefRow extends BlogBrief {
  id: number;
}

let nextRowID = 0;
function newRow(): BriefRow {
  return { id: nextRowID++, topic: "", context: "" };
}

/**
 * Briefs the Article Writer. One box per article: a topic to research and
 * optional guidance shaping how it gets written.
 *
 * This replaced a single textarea split on newlines. That form could only
 * express a list of subjects — there was nowhere to say who the piece is for or
 * what to avoid, and a topic containing a line break silently became two
 * articles.
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
  const [rows, setRows] = useState<BriefRow[]>([newRow()]);
  const [model, setModel] = useState("");

  const filled = rows.filter((r) => r.topic.trim() !== "");
  const tooMany = filled.length > HARD_MAX_ARTICLES;
  const canSubmit = filled.length > 0 && !tooMany && !generate.isPending;

  if (!open) return null;

  function updateRow(id: number, patch: Partial<BlogBrief>) {
    setRows((prev) =>
      prev.map((r) => (r.id === id ? { ...r, ...patch } : r))
    );
  }

  function addRow() {
    setRows((prev) => [...prev, newRow()]);
  }

  function removeRow(id: number) {
    // Never drop to zero rows — an empty dialog gives the user nothing to type
    // into and no obvious way back.
    setRows((prev) => (prev.length === 1 ? prev : prev.filter((r) => r.id !== id)));
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;

    const briefs: BlogBrief[] = filled.map((r) => ({
      topic: r.topic.trim(),
      context: r.context?.trim() || undefined,
    }));

    generate.mutate(
      { briefs, model: model.trim() || undefined },
      {
        onSuccess: (run) => {
          setRows([newRow()]);
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
        className="flex max-h-[90vh] w-full max-w-2xl flex-col rounded-lg border border-border bg-card shadow-xl"
      >
        <div className="border-b border-border p-6 pb-4">
          <h2 className="text-lg font-semibold text-foreground">
            Write articles
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Give each article a topic to research. The writer finds current
            sources, chooses its own title from what it learns, and cites what it
            used. Nothing reaches the CMS until you say so.
          </p>
        </div>

        <div className="flex-1 space-y-4 overflow-y-auto p-6">
          {rows.map((row, i) => (
            <div
              key={row.id}
              className="rounded-md border border-border bg-background/40 p-4"
            >
              <div className="flex items-center justify-between">
                <span className="text-2xs font-medium uppercase tracking-wide text-muted-foreground">
                  Article {i + 1}
                </span>
                {rows.length > 1 && (
                  <button
                    type="button"
                    onClick={() => removeRow(row.id)}
                    className="text-2xs text-muted-foreground transition-colors hover:text-destructive"
                    aria-label={`Remove article ${i + 1}`}
                  >
                    Remove
                  </button>
                )}
              </div>

              <label
                htmlFor={`topic-${row.id}`}
                className="mt-3 block text-xs text-muted-foreground"
              >
                Topic
              </label>
              <input
                id={`topic-${row.id}`}
                autoFocus={i === 0}
                value={row.topic}
                onChange={(e) => updateRow(row.id, { topic: e.target.value })}
                placeholder="Kubernetes Gateway API replacing Ingress"
                className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground/60"
              />

              <label
                htmlFor={`context-${row.id}`}
                className="mt-3 block text-xs text-muted-foreground"
              >
                Context{" "}
                <span className="text-muted-foreground/60">(optional)</span>
              </label>
              <textarea
                id={`context-${row.id}`}
                rows={2}
                value={row.context ?? ""}
                onChange={(e) => updateRow(row.id, { context: e.target.value })}
                placeholder="Angle, audience, points to cover, things to avoid"
                className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground/60"
              />
            </div>
          ))}

          <button
            type="button"
            onClick={addRow}
            disabled={rows.length >= HARD_MAX_ARTICLES}
            className="w-full rounded-md border border-dashed border-border py-2 text-sm text-muted-foreground transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50"
          >
            + Add another article
          </button>

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
        </div>

        <div className="flex items-center justify-between gap-2 border-t border-border p-6 pt-4">
          <p className="text-2xs text-muted-foreground">
            {filled.length} article{filled.length === 1 ? "" : "s"}
            {tooMany && (
              <span className="text-destructive">
                {" "}
                — maximum is {HARD_MAX_ARTICLES}
              </span>
            )}
          </p>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              className="rounded-md border border-border px-4 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!canSubmit}
              className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
            >
              {generate.isPending ? "Starting..." : "Write"}
            </button>
          </div>
        </div>
      </form>
    </div>
  );
}
