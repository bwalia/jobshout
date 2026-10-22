"use client";

import Link from "next/link";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useJobSupport } from "./JobSupportProvider";

export function JobSupportPanel({ autoFocus }: { autoFocus?: boolean }) {
  const { messages, send, busy, enabled, reset } = useJobSupport();
  const [draft, setDraft] = useState("");
  const bottomRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, busy]);

  useEffect(() => {
    if (autoFocus) inputRef.current?.focus();
  }, [autoFocus]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    const text = draft;
    setDraft("");
    await send(text);
  }

  return (
    <div className="js-panel flex h-full flex-col">
      <div className="js-panel-scroll flex-1 space-y-3 overflow-y-auto px-4 py-3">
        {enabled === false && (
          <p className="rounded-xl border border-line bg-raised px-3 py-2 text-sm text-muted">
            Recruiting is offline on this ring — browse{" "}
            <Link href="/jobs" className="font-semibold text-shout underline">
              open roles
            </Link>{" "}
            or{" "}
            <Link href="/post-job" className="font-semibold text-shout underline">
              post a job
            </Link>
            .
          </p>
        )}
        {messages.map((m) => (
          <div
            key={m.id}
            className={
              m.role === "user"
                ? "ml-8 rounded-2xl bg-shout px-3 py-2 text-sm text-[rgb(var(--on-brand))]"
                : m.role === "status"
                  ? "text-center text-[0.7rem] uppercase tracking-wide text-muted"
                  : "mr-4 rounded-2xl border border-line bg-surface px-3 py-2 text-sm text-ink"
            }
          >
            <div className="whitespace-pre-wrap">{m.content}</div>
            {m.matches && m.matches.length > 0 && (
              <ul className="mt-2 space-y-2 border-t border-line pt-2">
                {m.matches.map((hit) => (
                  <li key={hit.id} className="rounded-xl bg-raised px-3 py-2">
                    <div className="flex items-baseline justify-between gap-2">
                      <span className="font-semibold text-ink">{hit.title}</span>
                      <span className="font-mono text-[0.65rem] text-muted">
                        {hit.score}
                      </span>
                    </div>
                    <p className="mt-1 text-xs text-muted">{hit.summary}</p>
                    {hit.href && (
                      <Link
                        href={hit.href}
                        className="mt-1 inline-block text-xs font-semibold text-shout underline"
                      >
                        View role
                      </Link>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
        ))}
        {busy && (
          <p className="text-center text-xs text-muted" aria-live="polite">
            Thinking…
          </p>
        )}
        <div ref={bottomRef} />
      </div>
      <form
        onSubmit={onSubmit}
        className="border-t border-line bg-surface px-3 py-3"
      >
        <textarea
          ref={inputRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          rows={2}
          placeholder={
            enabled === false
              ? "Assistant offline"
              : "Answer, CV notes, or name + email…"
          }
          disabled={busy || enabled === false}
          className="w-full resize-none rounded-xl border border-line bg-bg px-3 py-2 text-sm text-ink outline-none ring-shout/40 placeholder:text-muted focus:ring-2"
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              void onSubmit(e);
            }
          }}
        />
        <div className="mt-2 flex items-center justify-between gap-2">
          <button
            type="button"
            onClick={reset}
            className="text-xs font-medium text-muted underline"
          >
            New chat
          </button>
          <button
            type="submit"
            disabled={busy || !draft.trim() || enabled === false}
            className="rounded-pill bg-shout px-4 py-2 text-sm font-semibold text-[rgb(var(--on-brand))] disabled:opacity-40"
          >
            Send
          </button>
        </div>
        <p className="mt-2 text-[0.65rem] leading-snug text-muted">
          Matching runs in NebulaDB on our cluster. Contact details go to OpsAPI
          CRM — never into the vector index or public models.
        </p>
      </form>
    </div>
  );
}
