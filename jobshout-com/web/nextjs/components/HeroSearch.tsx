"use client";

import { useRouter } from "next/navigation";
import { useId, useState } from "react";
import { Button, cx } from "@/components/ui";
import { SearchIcon } from "@/components/icons";

export type SearchScope = "jobs" | "agents" | "talents" | "groups" | "insights";

const SCOPES: Array<{
  id: SearchScope;
  label: string;
  path: string;
  placeholder: string;
  /** Extra query params when routing (e.g. location stays jobs-only). */
  locationField?: boolean;
}> = [
  {
    id: "jobs",
    label: "Jobs",
    path: "/jobs",
    placeholder: "Job title, skill, or keyword",
    locationField: true,
  },
  {
    id: "agents",
    label: "Agents",
    path: "/agents",
    placeholder: "Agent, skill, or tool",
  },
  {
    id: "talents",
    label: "Talents",
    path: "/members",
    placeholder: "Name or headline",
  },
  {
    id: "groups",
    label: "Groups",
    path: "/groups",
    placeholder: "Group or topic",
  },
  {
    id: "insights",
    label: "Insights",
    path: "/insights",
    placeholder: "Trend, topic, or keyword",
  },
];

/**
 * Search-first hero control: one query field with scope tabs that route into
 * existing Phase boards (jobs, agents, members, groups, insights).
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
  const [scope, setScope] = useState<SearchScope>(defaultScope);
  const [q, setQ] = useState(defaultQuery);
  const [where, setWhere] = useState(defaultLocation);

  const active = SCOPES.find((s) => s.id === scope) ?? SCOPES[0];
  const tall = size === "lg";

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const params = new URLSearchParams();
    if (q.trim()) params.set("q", q.trim());
    if (active.locationField && where.trim()) params.set("where", where.trim());
    const qs = params.toString();
    router.push(`${active.path}${qs ? `?${qs}` : ""}`);
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
          "surface-card flex flex-col gap-2 p-2 shadow-card sm:flex-row sm:items-center",
          tall ? "sm:gap-0" : "",
        )}
      >
        <div className="flex flex-1 items-center gap-3 rounded-xl px-3 transition-shadow duration-200 focus-within:ring-2 focus-within:ring-shout/40">
          <SearchIcon className="h-5 w-5 shrink-0 text-mute" />
          <input
            type="search"
            name="q"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={active.placeholder}
            aria-label={active.placeholder}
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
      </form>
    </div>
  );
}
