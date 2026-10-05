import Link from "next/link";
import { CROSS_INDUSTRY, industryHref, industryLabels, type IndustryNode } from "@/lib/showcase";

/** "Built for": the entry's industries and specialisms, each linking to its industry page. */
export function IndustryLinks({ industries, tree }: { industries: string[]; tree: IndustryNode[] }) {
  if (!industries.length || !tree.length) return null;
  const cross = industries.includes(CROSS_INDUSTRY);
  const labels = industryLabels(tree, industries.filter((s) => s !== CROSS_INDUSTRY));
  return (
    <section aria-labelledby="industries-heading" className="mt-10">
      <h2 id="industries-heading" className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
        Built for
      </h2>
      {cross ? (
        <p className="mt-3 text-sm text-body">Works across industries: a general-purpose tool rather than one built for a sector.</p>
      ) : (
        <ul className="mt-3 flex flex-wrap gap-2">
          {labels.map((l) => (
            <li key={l.slug}>
              <Link
                href={industryHref(l.slug)}
                className="inline-flex min-h-[36px] items-center rounded-pill border border-line bg-surface px-3.5 text-xs font-medium text-body transition-colors duration-200 hover:border-edge hover:text-ink"
              >
                {l.name}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** schema.org audiences for an entry's top-level industries. */
export function industryAudience(industries: string[], tree: IndustryNode[]) {
  const top = tree.filter((n) => n.slug !== CROSS_INDUSTRY && industries.includes(n.slug));
  if (!top.length) return undefined;
  const list = top.map((n) => ({ "@type": "BusinessAudience", name: n.name }));
  return list.length === 1 ? list[0] : list;
}
