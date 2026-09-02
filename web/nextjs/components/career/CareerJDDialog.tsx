"use client";

import { postingHref } from "./jobs";

export type CareerJDPreview = {
  title: string;
  company: string;
  url: string;
  text: string;
  loading: boolean;
  error: string;
};

export function CareerJDDialog({
  preview,
  onClose,
}: {
  preview: CareerJDPreview;
  onClose: () => void;
}) {
  const href = postingHref(preview.url);
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onClick={onClose}
      role="dialog"
      aria-label="Job description"
    >
      <div
        className="flex max-h-[85vh] w-full max-w-2xl flex-col rounded-xl border border-border bg-card p-5"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-3 flex items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">{preview.title || "Job description"}</h2>
            <p className="text-sm text-muted-foreground">{preview.company || "Company"}</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-border px-2 py-1 text-sm"
            aria-label="Close"
          >
            Close
          </button>
        </div>
        {href && (
          <a
            href={href}
            target="_blank"
            rel="noreferrer"
            className="mb-3 self-start rounded-md border border-border px-3 py-1.5 text-sm"
          >
            Go to posting
          </a>
        )}
        {preview.loading ? (
          <p className="text-sm text-muted-foreground">Loading the job description…</p>
        ) : preview.error ? (
          <p className="text-sm text-destructive">{preview.error}</p>
        ) : (
          <pre className="flex-1 overflow-auto whitespace-pre-wrap rounded-md border border-border bg-muted/40 p-3 text-xs leading-relaxed">
            {preview.text || "No job text on this posting."}
          </pre>
        )}
      </div>
    </div>
  );
}
