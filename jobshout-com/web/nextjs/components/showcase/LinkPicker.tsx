"use client";

import { useEffect, useId, useRef, useState, useTransition } from "react";
import { linkCandidatesAction } from "@/app/showcase/actions";
import { ArrowLeftIcon, SearchIcon, XIcon } from "@/components/icons";
import { AppLogo } from "@/components/showcase/AppCard";
import { cx } from "@/components/ui";
import type { ShowcaseLink } from "@/lib/showcase";

/**
 * Pick agents (ordered, each with a role) or a single team from the
 * directory. Serialises to a hidden input as JSON [{slug, role}], which the
 * server action parses; the API re-checks every link.
 */
export function LinkPicker({
  name,
  kind,
  label,
  hint,
  initial,
  single = false,
  roles = true,
  error,
}: {
  name: string;
  kind: "agent" | "team";
  label: string;
  hint?: string;
  initial: ShowcaseLink[];
  single?: boolean;
  roles?: boolean;
  error?: string;
}) {
  const id = useId();
  const [chosen, setChosen] = useState<ShowcaseLink[]>(initial);
  const [q, setQ] = useState("");
  const [open, setOpen] = useState(false);
  const [results, setResults] = useState<ShowcaseLink[]>([]);
  const [searching, start] = useTransition();
  // Blur closes the list after a beat (so a click on an option lands); focus
  // coming back inside that beat must cancel it or the list shuts on the user.
  const closing = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cancelClose = () => {
    if (closing.current) clearTimeout(closing.current);
    closing.current = null;
  };
  useEffect(() => cancelClose, []);

  useEffect(() => {
    if (!open) return;
    const t = setTimeout(() => {
      start(async () => setResults(await linkCandidatesAction(kind, q)));
    }, 200);
    return () => clearTimeout(t);
  }, [q, open, kind]);

  const add = (l: ShowcaseLink) => {
    setChosen((c) => (single ? [{ ...l, role: "" }] : c.some((x) => x.slug === l.slug) ? c : [...c, { ...l, role: "" }]));
    setQ("");
    setOpen(false);
  };
  const move = (i: number, by: -1 | 1) =>
    setChosen((c) => {
      const next = [...c];
      const [item] = next.splice(i, 1);
      next.splice(i + by, 0, item);
      return next;
    });
  const available = results.filter((r) => !chosen.some((c) => c.slug === r.slug));
  const noun = kind === "agent" ? "agent" : "team";

  return (
    <div>
      <input
        type="hidden"
        name={name}
        value={JSON.stringify(chosen.map((c) => ({ slug: c.slug, role: c.role })))}
      />
      <p id={`${id}-label`} className="text-sm font-semibold text-ink">
        {label}
        <span className="ml-2 text-xs font-normal text-mute">optional</span>
      </p>
      {hint ? <p className="mt-1 text-xs leading-relaxed text-mute">{hint}</p> : null}

      {chosen.length ? (
        <ol className="mt-3 space-y-2" aria-labelledby={`${id}-label`}>
          {chosen.map((c, i) => (
            <li key={c.slug} className="flex flex-wrap items-center gap-2.5 rounded-xl border border-line bg-surface p-2.5">
              <AppLogo app={{ name: c.name, logo_url: c.logo_url }} size="sm" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-semibold text-ink">{c.name}</span>
                <span className="block truncate text-xs text-mute">{c.tagline || c.slug}</span>
              </span>
              {roles && !single ? (
                <input
                  aria-label={`What ${c.name} did`}
                  value={c.role}
                  maxLength={80}
                  placeholder="Role, e.g. Backend"
                  onChange={(e) =>
                    setChosen((all) => all.map((x) => (x.slug === c.slug ? { ...x, role: e.target.value } : x)))
                  }
                  className="h-9 w-full rounded-lg border border-line bg-surface px-3 text-sm text-ink placeholder:text-mute/70 focus:border-shout focus:outline-none focus:ring-2 focus:ring-shout/25 sm:w-48"
                />
              ) : null}
              {!single ? (
                <span className="flex gap-1">
                  <button
                    type="button"
                    aria-label={`Move ${c.name} up`}
                    disabled={i === 0}
                    onClick={() => move(i, -1)}
                    className="flex h-9 w-9 items-center justify-center rounded-lg border border-line text-mute hover:text-ink disabled:opacity-40"
                  >
                    <ArrowLeftIcon className="h-4 w-4 rotate-90" />
                  </button>
                  <button
                    type="button"
                    aria-label={`Move ${c.name} down`}
                    disabled={i === chosen.length - 1}
                    onClick={() => move(i, 1)}
                    className="flex h-9 w-9 items-center justify-center rounded-lg border border-line text-mute hover:text-ink disabled:opacity-40"
                  >
                    <ArrowLeftIcon className="h-4 w-4 -rotate-90" />
                  </button>
                </span>
              ) : null}
              <button
                type="button"
                aria-label={`Remove ${c.name}`}
                onClick={() => setChosen((all) => all.filter((x) => x.slug !== c.slug))}
                className="flex h-9 w-9 items-center justify-center rounded-lg border border-line text-mute hover:text-shout"
              >
                <XIcon className="h-4 w-4" />
              </button>
            </li>
          ))}
        </ol>
      ) : null}

      {!single || chosen.length === 0 ? (
        <div className="relative mt-3">
          <SearchIcon className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-mute" />
          <input
            type="search"
            role="combobox"
            aria-expanded={open}
            aria-controls={`${id}-results`}
            aria-label={`Search the directory for an ${noun}`}
            value={q}
            onFocus={() => {
              cancelClose();
              setOpen(true);
            }}
            onBlur={() => {
              cancelClose();
              closing.current = setTimeout(() => setOpen(false), 150);
            }}
            onChange={(e) => {
              cancelClose();
              setQ(e.target.value);
              setOpen(true);
            }}
            onKeyDown={(e) => {
              // Enter here would submit the whole form.
              if (e.key === "Enter") {
                e.preventDefault();
                if (available[0]) add(available[0]);
              }
            }}
            placeholder={`Search ${noun}s in the directory…`}
            className="h-11 w-full rounded-xl border border-line bg-surface pl-10 pr-4 text-sm text-ink placeholder:text-mute/70 hover:border-edge focus:border-shout focus:outline-none focus:ring-2 focus:ring-shout/25"
          />
          {open ? (
            <ul
              id={`${id}-results`}
              role="listbox"
              className="absolute z-20 mt-1.5 max-h-72 w-full overflow-auto rounded-xl border border-line bg-surface p-1 shadow-lift"
            >
              {available.length ? (
                available.map((r) => (
                  <li key={r.slug} role="option" aria-selected={false}>
                    <button
                      type="button"
                      onMouseDown={(e) => e.preventDefault()}
                      onClick={() => add(r)}
                      className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left hover:bg-raised"
                    >
                      <AppLogo app={{ name: r.name, logo_url: r.logo_url }} size="sm" />
                      <span className="min-w-0">
                        <span className="block truncate text-sm font-semibold text-ink">{r.name}</span>
                        <span className="block truncate text-xs text-mute">{r.tagline}</span>
                      </span>
                    </button>
                  </li>
                ))
              ) : (
                <li className={cx("px-3 py-2.5 text-sm text-mute")}>
                  {searching ? "Searching…" : `No ${noun}s found. Add it to the directory first, then link it.`}
                </li>
              )}
            </ul>
          ) : null}
        </div>
      ) : null}
      {error ? (
        <p role="alert" className="mt-1.5 text-xs font-medium text-shout">
          {error}
        </p>
      ) : null}
    </div>
  );
}
