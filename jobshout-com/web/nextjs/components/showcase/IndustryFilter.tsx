import Link from "next/link";
import { cx } from "@/components/ui";
import { CROSS_INDUSTRY, findIndustry, type IndustryNode } from "@/lib/showcase";

const CHIP =
  "inline-flex min-h-[36px] items-center rounded-pill border px-3.5 text-xs font-medium transition-colors duration-200";
const ON = "border-shout/40 bg-shout/10 text-ink";
const OFF = "border-line bg-surface text-body hover:border-edge hover:text-ink";

/** How many industries show before "More industries". */
const SHOWN = 8;

function Chip({ href, active, title, children }: { href: string; active: boolean; title?: string; children: React.ReactNode }) {
  return (
    <Link href={href} aria-current={active ? "true" : undefined} title={title} className={cx(CHIP, active ? ON : OFF)}>
      {children}
      {active ? (
        <span className="ml-1.5 text-mute" aria-label="remove filter">
          ×
        </span>
      ) : null}
    </Link>
  );
}

const plural = (n: number, noun: string) => `${n} ${n === 1 ? noun : `${noun}s`}`;

/**
 * Browse by industry: the sectors that have entries, a "More industries"
 * disclosure for the rest, then the chosen industry's specialisms and the
 * cross-industry toggle. `href(slug, cross)` builds the page's own URL.
 */
export function IndustryFilter({
  tree,
  active,
  cross,
  noun,
  href,
}: {
  tree: IndustryNode[];
  active: string | null;
  cross: boolean;
  noun: string;
  href: (industry: string | null, cross?: boolean) => string;
}) {
  const found = findIndustry(tree, active);
  const parent = found ? (found.parent ?? found.node) : null;
  const sectors = tree.filter((n) => n.slug !== CROSS_INDUSTRY && (n.count > 0 || n.slug === parent?.slug));
  if (!sectors.length && !found) return null;
  let shown = sectors.slice(0, SHOWN);
  let more = sectors.slice(SHOWN);
  // Keep the chosen industry visible even when it would sit under "More".
  if (parent && more.some((n) => n.slug === parent.slug)) {
    shown = [...shown, parent];
    more = more.filter((n) => n.slug !== parent.slug);
  }
  const verticals = parent ? parent.verticals.filter((v) => v.count > 0 || v.slug === active) : [];

  return (
    <div className="flex flex-col gap-3">
      <nav aria-label="Filter by industry">
        <ul className="flex flex-wrap items-center gap-2">
          {shown.map((n) => (
            <li key={n.slug}>
              <Chip
                href={href(parent?.slug === n.slug ? null : n.slug, cross)}
                active={parent?.slug === n.slug}
                title={`${plural(n.count, noun)} · ${n.description}`}
              >
                {n.name}
              </Chip>
            </li>
          ))}
          {more.length ? (
            <li>
              <details className="group relative">
                <summary className={cx(CHIP, OFF, "cursor-pointer list-none [&::-webkit-details-marker]:hidden")}>
                  More industries
                  <span aria-hidden className="ml-1.5 transition-transform group-open:rotate-180">
                    ▾
                  </span>
                </summary>
                <ul className="absolute left-0 z-20 mt-2 grid w-64 gap-0.5 rounded-xl border border-line bg-surface p-1.5 shadow-lift">
                  {more.map((n) => (
                    <li key={n.slug}>
                      <Link
                        href={href(n.slug, cross)}
                        className="flex min-h-[36px] items-center justify-between gap-3 rounded-lg px-3 text-sm text-body hover:bg-raised hover:text-ink"
                      >
                        {n.name}
                        <span className="text-xs tabular-nums text-mute">{n.count}</span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </details>
            </li>
          ) : null}
        </ul>
      </nav>

      {parent ? (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          {verticals.length ? (
            <nav aria-label={`Specialisms in ${parent.name}`}>
              <ul className="flex flex-wrap items-center gap-2">
                <li className="text-xs text-mute">Specialism</li>
                {verticals.map((v) => (
                  <li key={v.slug}>
                    <Chip
                      href={href(active === v.slug ? parent.slug : v.slug, cross)}
                      active={active === v.slug}
                      title={plural(v.count, noun)}
                    >
                      {v.name}
                    </Chip>
                  </li>
                ))}
              </ul>
            </nav>
          ) : null}
          <Link
            href={href(active, !cross)}
            role="switch"
            aria-checked={cross}
            className="inline-flex min-h-[36px] items-center gap-2 text-xs font-medium text-body hover:text-ink"
          >
            <span
              aria-hidden
              className={cx(
                "relative inline-block h-4 w-7 rounded-pill border transition-colors",
                cross ? "border-shout bg-shout" : "border-edge bg-raised",
              )}
            >
              <span
                className={cx(
                  "absolute top-1/2 h-3 w-3 -translate-y-1/2 rounded-pill bg-surface transition-all",
                  cross ? "left-[13px]" : "left-[1px]",
                )}
              />
            </span>
            Include cross-industry tools
          </Link>
        </div>
      ) : null}
    </div>
  );
}

/** Removable chips for every active filter, with Clear all. */
export function ActiveFilters({
  items,
  clearHref,
}: {
  items: Array<{ key: string; label: string; href: string }>;
  clearHref: string;
}) {
  if (!items.length) return null;
  return (
    <div className="flex flex-wrap items-center gap-2" aria-label="Active filters" role="group">
      {items.map((i) => (
        <Link key={i.key} href={i.href} className={cx(CHIP, ON)} aria-label={`Remove filter: ${i.label}`}>
          {i.label}
          <span aria-hidden className="ml-1.5 text-mute">
            ×
          </span>
        </Link>
      ))}
      {items.length > 1 ? (
        <Link href={clearHref} className="min-h-[36px] px-1 text-xs font-medium text-ink underline decoration-line underline-offset-4 hover:text-shout">
          Clear all
        </Link>
      ) : null}
    </div>
  );
}
