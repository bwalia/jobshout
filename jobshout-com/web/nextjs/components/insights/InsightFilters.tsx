import Link from "next/link";
import { SearchIcon } from "@/components/icons";
import { KindIcon } from "@/components/insights/KindIcon";
import { cx } from "@/components/ui";
import { INSIGHT_KINDS, KIND_META, type InsightKind, type InsightTopic } from "@/lib/insights";

export type InsightFilterState = { kind: InsightKind | null; topic: string | null; q: string };

export function insightHref(f: Partial<InsightFilterState>): string {
  const p = new URLSearchParams();
  if (f.kind) p.set("kind", f.kind);
  if (f.topic) p.set("topic", f.topic);
  if (f.q) p.set("q", f.q);
  const s = p.toString();
  return s ? `/insights?${s}` : "/insights";
}

const TAB =
  "inline-flex min-h-[44px] shrink-0 items-center gap-2 rounded-pill px-4 text-sm transition-colors duration-200";

export function KindTabs({ filters }: { filters: InsightFilterState }) {
  const tabs: Array<{ kind: InsightKind | null; label: string }> = [
    { kind: null, label: "All" },
    ...INSIGHT_KINDS.map((k) => ({ kind: k, label: KIND_META[k].plural })),
  ];
  return (
    <nav aria-label="Filter by format" className="-mx-5 overflow-x-auto px-5 sm:mx-0 sm:px-0">
      <ul className="flex w-max gap-1.5 rounded-pill border border-line bg-surface p-1.5">
        {tabs.map((t) => {
          const active = filters.kind === t.kind;
          return (
            <li key={t.label}>
              <Link
                href={insightHref({ ...filters, kind: t.kind })}
                aria-current={active ? "page" : undefined}
                className={cx(
                  TAB,
                  active ? "bg-ink font-semibold text-bg" : "font-medium text-body hover:bg-raised hover:text-ink",
                )}
              >
                {t.kind ? <KindIcon kind={t.kind} className="h-4 w-4" /> : null}
                {t.label}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

export function TopicChips({
  topics,
  filters,
}: {
  topics: InsightTopic[];
  filters: InsightFilterState;
}) {
  return (
    <nav aria-label="Filter by topic">
      <ul className="flex flex-wrap gap-2">
        {topics.map((t) => {
          const active = filters.topic === t.slug;
          return (
            <li key={t.slug}>
              <Link
                href={insightHref({ ...filters, topic: active ? null : t.slug })}
                aria-current={active ? "true" : undefined}
                title={t.description}
                className={cx(
                  "inline-flex min-h-[36px] items-center rounded-pill border px-3.5 text-xs font-medium transition-colors duration-200",
                  active
                    ? "border-shout/40 bg-shout/10 text-ink"
                    : "border-line bg-surface text-body hover:border-edge hover:text-ink",
                )}
              >
                {t.name}
                {active ? <span className="ml-1.5 text-mute" aria-label="remove filter">×</span> : null}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

export function InsightSearch({ filters }: { filters: InsightFilterState }) {
  return (
    <form action="/insights" role="search" className="relative w-full sm:max-w-xs">
      {filters.kind ? <input type="hidden" name="kind" value={filters.kind} /> : null}
      {filters.topic ? <input type="hidden" name="topic" value={filters.topic} /> : null}
      <label htmlFor="insights-q" className="sr-only">
        Search insights
      </label>
      <SearchIcon className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-mute" />
      <input
        id="insights-q"
        name="q"
        type="search"
        defaultValue={filters.q}
        placeholder="Search AI, hiring, skills…"
        className="h-11 w-full rounded-pill border border-line bg-surface pl-10 pr-4 text-sm text-ink placeholder:text-mute/70 transition-colors duration-200 hover:border-edge focus:border-shout focus:outline-none focus:ring-2 focus:ring-shout/25"
      />
    </form>
  );
}
