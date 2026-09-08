"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useState } from "react";
import { Button, Checkbox, Select, cx } from "@/components/ui";
import { SlidersIcon, XIcon } from "@/components/icons";
import { EMPLOYMENT_TYPES, type EmploymentType } from "@/lib/api";
import { CATEGORY_NAMES, employmentLabel } from "@/lib/format";
import { SORT_OPTIONS, activeFilterCount, type JobFilterState } from "@/lib/filter";

const SALARY_STEPS = [
  { value: "", label: "Any salary" },
  { value: "30000", label: "£30k+" },
  { value: "50000", label: "£50k+" },
  { value: "80000", label: "£80k+" },
  { value: "120000", label: "£120k+" },
];

export function JobFilters({
  filters,
  resultCount,
}: {
  filters: JobFilterState;
  resultCount: number;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [openOnMobile, setOpenOnMobile] = useState(false);

  /** Every control writes straight to the URL — state stays shareable. */
  const setParam = useCallback(
    (key: string, value: string | null) => {
      const params = new URLSearchParams(searchParams.toString());
      if (value === null || value === "") params.delete(key);
      else params.set(key, value);
      router.push(`${pathname}${params.toString() ? `?${params}` : ""}`, { scroll: false });
    },
    [pathname, router, searchParams],
  );

  function toggleType(type: EmploymentType) {
    const next = filters.types.includes(type)
      ? filters.types.filter((t) => t !== type)
      : [...filters.types, type];
    setParam("type", next.join(","));
  }

  const count = activeFilterCount(filters);

  return (
    <>
      {/* Mobile: filters collapse behind a button so results stay above the fold. */}
      <div className="flex items-center gap-3 lg:hidden">
        <Button
          variant="secondary"
          size="md"
          onClick={() => setOpenOnMobile((v) => !v)}
          aria-expanded={openOnMobile}
          className="flex-1"
        >
          <SlidersIcon className="h-4 w-4" />
          Filters
          {count > 0 ? (
            <span className="ml-1 rounded-pill bg-shout px-2 py-0.5 text-xs text-[rgb(var(--on-brand))]">
              {count}
            </span>
          ) : null}
        </Button>
        <SortSelect value={filters.sort} onChange={(v) => setParam("sort", v)} />
      </div>

      <aside
        className={cx(
          "space-y-6 lg:sticky lg:top-24 lg:block lg:space-y-7",
          openOnMobile ? "block" : "hidden",
        )}
        aria-label="Job filters"
      >
        <div className="hidden items-center justify-between lg:flex">
          <h2 className="font-display text-base font-semibold text-ink">Filters</h2>
          {count > 0 ? (
            <button
              type="button"
              onClick={() => router.push(pathname, { scroll: false })}
              className="inline-flex cursor-pointer items-center gap-1 text-xs font-medium text-mute transition-colors duration-200 hover:text-shout"
            >
              <XIcon className="h-3.5 w-3.5" />
              Clear all
            </button>
          ) : null}
        </div>

        <p className="text-sm text-mute lg:hidden">
          {resultCount} {resultCount === 1 ? "role" : "roles"} match
        </p>

        <FilterGroup label="Work type">
          <div className="-my-1">
            {EMPLOYMENT_TYPES.map((type) => (
              <Checkbox
                key={type}
                label={employmentLabel(type)}
                checked={filters.types.includes(type)}
                onChange={() => toggleType(type)}
              />
            ))}
          </div>
        </FilterGroup>

        <FilterGroup label="Location">
          <Checkbox
            label="Remote only"
            checked={filters.remote}
            onChange={(e) => setParam("remote", e.target.checked ? "1" : null)}
          />
        </FilterGroup>

        <FilterGroup label="Category" htmlFor="filter-category">
          <Select
            id="filter-category"
            value={filters.category}
            onChange={(e) => setParam("category", e.target.value)}
          >
            <option value="">All categories</option>
            {CATEGORY_NAMES.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </Select>
        </FilterGroup>

        <FilterGroup label="Minimum salary" htmlFor="filter-salary">
          <Select
            id="filter-salary"
            value={filters.minSalary?.toString() ?? ""}
            onChange={(e) => setParam("min", e.target.value)}
          >
            {SALARY_STEPS.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </Select>
          <p className="mt-2 text-xs leading-relaxed text-mute">
            Roles without a listed salary are hidden while a minimum is set.
          </p>
        </FilterGroup>

        {count > 0 ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => router.push(pathname, { scroll: false })}
            className="w-full lg:hidden"
          >
            Clear all filters
          </Button>
        ) : null}
      </aside>
    </>
  );
}

function FilterGroup({
  label,
  htmlFor,
  children,
}: {
  label: string;
  htmlFor?: string;
  children: React.ReactNode;
}) {
  const Heading = htmlFor ? "label" : "p";
  return (
    <div>
      <Heading
        {...(htmlFor ? { htmlFor } : {})}
        className="mb-2.5 block text-xs font-semibold uppercase tracking-[0.14em] text-mute"
      >
        {label}
      </Heading>
      {children}
    </div>
  );
}

export function SortSelect({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="flex items-center gap-2 text-sm text-mute">
      <span className="hidden sm:inline">Sort</span>
      <Select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label="Sort jobs"
        className="h-11 w-auto py-0"
      >
        {SORT_OPTIONS.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </Select>
    </label>
  );
}

/** Sort control for the results header — writes to the same URL params. */
export function BoardSort({ value }: { value: string }) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  return (
    <SortSelect
      value={value}
      onChange={(v) => {
        const params = new URLSearchParams(searchParams.toString());
        params.set("sort", v);
        router.push(`${pathname}?${params}`, { scroll: false });
      }}
    />
  );
}
