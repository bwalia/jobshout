"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useId, useRef, useState, useTransition } from "react";
import { Button, cx } from "@/components/ui";
import { SearchIcon } from "@/components/icons";
import {
  boardPathForKind,
  searchKindLabel,
  type SearchHit,
  type SearchKind,
  type UnifiedSearchResponse,
} from "@/lib/search-shared";

export type SearchScope = "jobs" | "agents" | "talents" | "groups" | "insights";

const SCOPES: Array<{
  id: SearchScope;
  label: string;
  kind: SearchKind;
  path: string;
  placeholder: string;
  /** Extra query params when routing (e.g. location stays jobs-only). */
  locationField?: boolean;
}> = [
  {
    id: "jobs",
    label: "Jobs",
    kind: "job",
    path: "/jobs",
    placeholder: "Job title, skill, or keyword",
    locationField: true,
  },
  {
    id: "agents",
    label: "Agents",
    kind: "agent",
    path: "/agents",
    placeholder: "Agent, skill, or tool",
  },
  {
    id: "talents",
    label: "Talents",
    kind: "talent",
    path: "/members",
    placeholder: "Name or headline",
  },
  {
    id: "groups",
    label: "Groups",
    kind: "group",
    path: "/groups",
    placeholder: "Group or topic",
  },
  {
    id: "insights",
    label: "Insights",
    kind: "insight",
    path: "/insights",
    placeholder: "Trend, topic, or keyword",
  },
];

/**
 * Search-first hero control: scoped submit into existing boards, plus a
 * shared typeahead that fans out across agents, jobs, talents, groups, and
 * insights.
 */
export function HeroSearch({
  defaultScope = "jobs",
  defaultQuery = "",
  defaultLocation = "",
  size = "lg",
}: {
  defaultScope?: SearchScope;
  defaultQuery?: string;
  defaultLocation?: string;
  size?: "lg" | "md";
}) {
  const router = useRouter();
  const tabsId = useId();
  const listId = useId();
  const [scope, setScope] = useState<SearchScope>(defaultScope);
  const [q, setQ] = useState(defaultQuery);
  const [where, setWhere] = useState(defaultLocation);
  const [open, setOpen] = useState(false);
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [activeIndex, setActiveIndex] = useState(-1);
  const [searching, startSearch] = useTransition();
  const closing = useRef<ReturnType<typeof setTimeout> | null>(null);

  const active = SCOPES.find((s) => s.id === scope) ?? SCOPES[0];
  const tall = size === "lg";
  const showTypeahead = open && q.trim().length >= 2;

  const cancelClose = () => {
    if (closing.current) clearTimeout(closing.current);
    closing.current = null;
  };
  useEffect(() => () => cancelClose(), []);

  useEffect(() => {
    const term = q.trim();
    if (term.length < 2) {
      setHits([]);
      setActiveIndex(-1);
      return;
    }
    const t = setTimeout(() => {
      startSearch(async () => {
        try {
          const res = await fetch(`/api/search?q=${encodeURIComponent(term)}`);
          if (!res.ok) {
            setHits([]);
            return;
          }
          const body = (await res.json()) as UnifiedSearchResponse;
          setHits(body.results ?? []);
          setActiveIndex(-1);
        } catch {
          setHits([]);
        }
      });
    }, 200);
    return () => clearTimeout(t);
  }, [q]);

  function submit(e: React.FormEvent) {
    e.preventDefault();
    if (showTypeahead && activeIndex >= 0 && hits[activeIndex]) {
      router.push(hits[activeIndex].href);
      return;
    }
    const params = new URLSearchParams();
    if (q.trim()) params.set("q", q.trim());
    if (active.locationField && where.trim()) params.set("where", where.trim());
    const qs = params.toString();
    router.push(`${active.path}${qs ? `?${qs}` : ""}`);
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (!showTypeahead || hits.length === 0) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActiveIndex((i) => (i + 1) % hits.length);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActiveIndex((i) => (i <= 0 ? hits.length - 1 : i - 1));
    } else if (e.key === "Escape") {
      setOpen(false);
      setActiveIndex(-1);
    }
  }

  return (
    <div className="w-full">
      <div
        role="tablist"
        aria-label="Search in"
        className="mb-3 flex flex-wrap items-center justify-center gap-1.5"
      >
        {SCOPES.map((s) => {
          const selected = s.id === scope;
          return (
            <button
              key={s.id}
              type="button"
              role="tab"
              id={`${tabsId}-${s.id}`}
              aria-selected={selected}
              onClick={() => setScope(s.id)}
              className={cx(
                "rounded-pill px-3.5 py-2 text-sm transition-colors duration-200",
                selected
                  ? "bg-ink font-semibold text-bg"
                  : "font-medium text-mute hover:bg-raised hover:text-ink",
              )}
            >
              {s.label}
            </button>
          );
        })}
      </div>

      <form
        onSubmit={submit}
        role="search"
        aria-labelledby={`${tabsId}-${scope}`}
        className={cx(
          "relative surface-card flex flex-col gap-2 p-2 shadow-card sm:flex-row sm:items-center",
          tall ? "sm:gap-0" : "",
        )}
      >
        <div className="relative flex flex-1 items-center gap-3 rounded-xl px-3 transition-shadow duration-200 focus-within:ring-2 focus-within:ring-shout/40">
          <SearchIcon className="h-5 w-5 shrink-0 text-mute" />
          <input
            type="search"
            name="q"
            role="combobox"
            aria-expanded={showTypeahead}
            aria-controls={listId}
            aria-autocomplete="list"
            aria-activedescendant={
              activeIndex >= 0 && hits[activeIndex]
                ? `${listId}-opt-${activeIndex}`
                : undefined
            }
            value={q}
            onChange={(e) => {
              cancelClose();
              setQ(e.target.value);
              setOpen(true);
            }}
            onFocus={() => {
              cancelClose();
              setOpen(true);
            }}
            onBlur={() => {
              cancelClose();
              closing.current = setTimeout(() => setOpen(false), 150);
            }}
            onKeyDown={onKeyDown}
            placeholder={active.placeholder}
            aria-label={active.placeholder}
            autoComplete="off"
            className={cx(
              "w-full bg-transparent text-[0.95rem] text-ink placeholder:text-mute/80 focus:outline-none",
              tall ? "py-4" : "py-3",
            )}
          />
        </div>

        {active.locationField ? (
          <>
            <span aria-hidden className="mx-2 hidden h-7 w-px bg-line sm:block" />
            <div className="flex flex-1 items-center gap-3 rounded-xl px-3 transition-shadow duration-200 focus-within:ring-2 focus-within:ring-shout/40 sm:max-w-[15rem]">
              <input
                type="text"
                name="where"
                value={where}
                onChange={(e) => setWhere(e.target.value)}
                placeholder="City or remote"
                aria-label="City or remote"
                className={cx(
                  "w-full bg-transparent text-[0.95rem] text-ink placeholder:text-mute/80 focus:outline-none",
                  tall ? "py-4" : "py-3",
                )}
              />
            </div>
          </>
        ) : null}

        <Button type="submit" size={tall ? "lg" : "md"} className="shrink-0">
          Search {active.label.toLowerCase()}
        </Button>

        {showTypeahead ? (
          <ul
            id={listId}
            role="listbox"
            aria-label="Suggestions across JobShout"
            className="absolute inset-x-2 top-full z-30 mt-1.5 max-h-80 overflow-auto rounded-xl border border-line bg-surface p-1 shadow-lift"
          >
            {hits.length > 0 ? (
              hits.map((hit, i) => (
                <li key={`${hit.kind}-${hit.id}`} role="option" aria-selected={i === activeIndex}>
                  <Link
                    id={`${listId}-opt-${i}`}
                    href={hit.href}
                    onMouseDown={(e) => e.preventDefault()}
                    onMouseEnter={() => setActiveIndex(i)}
                    className={cx(
                      "flex w-full items-start gap-3 rounded-lg px-3 py-2.5 text-left transition-colors duration-150",
                      i === activeIndex ? "bg-raised" : "hover:bg-raised",
                    )}
                  >
                    <span className="mt-0.5 shrink-0 rounded-md bg-shout/10 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-shout">
                      {searchKindLabel(hit.kind)}
                    </span>
                    <span className="min-w-0">
                      <span className="block truncate text-sm font-semibold text-ink">{hit.title}</span>
                      <span className="block truncate text-xs text-mute">{hit.subtitle}</span>
                    </span>
                  </Link>
                </li>
              ))
            ) : (
              <li className="px-3 py-3 text-sm text-mute">
                {searching ? "Searching…" : `No matches yet — press Enter to search ${active.label.toLowerCase()}.`}
              </li>
            )}
            <li className="border-t border-line px-3 py-2">
              <Link
                href={`${boardPathForKind(active.kind)}?q=${encodeURIComponent(q.trim())}`}
                onMouseDown={(e) => e.preventDefault()}
                className="text-sm font-medium text-shout hover:underline"
              >
                See all {active.label.toLowerCase()} for “{q.trim()}”
              </Link>
            </li>
          </ul>
        ) : null}
      </form>
    </div>
  );
}
