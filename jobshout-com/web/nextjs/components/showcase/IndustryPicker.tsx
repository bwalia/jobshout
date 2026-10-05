"use client";

import { useId, useState } from "react";
import { cx } from "@/components/ui";
import { CROSS_INDUSTRY, MAX_INDUSTRIES, MAX_VERTICALS, type IndustryNode } from "@/lib/showcase";

const BOX =
  "inline-flex min-h-[40px] cursor-pointer items-center gap-2 rounded-pill border border-line px-3.5 text-sm text-body transition-colors duration-200 hover:border-edge has-[:checked]:border-shout/50 has-[:checked]:bg-shout/10 has-[:checked]:font-semibold has-[:checked]:text-ink has-[:focus-visible]:outline has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-shout has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50";

/**
 * Who an entry was built for: up to three industries, up to three
 * specialisms in each, or "works across industries". The API enforces the
 * same limits; this keeps them visible while choosing.
 */
export function IndustryPicker({
  tree,
  initial,
  noun,
  error,
}: {
  tree: IndustryNode[];
  initial: string[];
  noun: string;
  error?: string;
}) {
  const [chosen, setChosen] = useState<Set<string>>(() => new Set(initial));
  const [said, setSaid] = useState("");
  // Several pickers can share a page (the editors' queue).
  const uid = useId();
  const sectors = tree.filter((n) => n.slug !== CROSS_INDUSTRY);
  const cross = tree.find((n) => n.slug === CROSS_INDUSTRY);
  const isCross = chosen.has(CROSS_INDUSTRY);
  const parents = sectors.filter((n) => chosen.has(n.slug));
  const full = parents.length >= MAX_INDUSTRIES;

  function toggle(slug: string, parent?: IndustryNode) {
    const next = new Set(chosen);
    if (slug === CROSS_INDUSTRY) {
      if (next.has(slug)) next.delete(slug);
      else {
        next.clear();
        next.add(slug);
      }
      setChosen(next);
      setSaid(next.has(slug) ? "Works across industries. Other industries cleared." : "");
      return;
    }
    next.delete(CROSS_INDUSTRY);
    if (next.has(slug)) {
      next.delete(slug);
      // Dropping an industry drops its specialisms.
      if (!parent) tree.find((n) => n.slug === slug)?.verticals.forEach((v) => next.delete(v.slug));
    } else {
      next.add(slug);
      if (parent) next.add(parent.slug);
    }
    const count = sectors.filter((n) => next.has(n.slug)).length;
    setChosen(next);
    setSaid(count >= MAX_INDUSTRIES ? `${MAX_INDUSTRIES} industries chosen, the most allowed.` : "");
  }

  if (!tree.length) return null;

  return (
    <div>
      {/* Tells the action the picker was shown, so an empty choice clears rather than keeps. */}
      <input type="hidden" name="industries_present" value="1" />
      {[...chosen].map((s) => (
        <input key={s} type="hidden" name="industries" value={s} />
      ))}
      <p className="text-sm font-semibold text-ink" id={`${uid}-label`}>
        Industries
      </p>
      <p className="mt-1 text-sm text-mute" id={`${uid}-hint`}>
        The sectors this {noun} was built for, not every sector it could serve. Up to {MAX_INDUSTRIES}, each with up to{" "}
        {MAX_VERTICALS} specialisms.
      </p>
      <div role="group" aria-labelledby={`${uid}-label`} aria-describedby={`${uid}-hint`} className="mt-3 flex flex-wrap gap-2">
        {sectors.map((n) => {
          const on = chosen.has(n.slug);
          return (
            <label key={n.slug} className={BOX} title={n.description}>
              <input
                type="checkbox"
                checked={on}
                disabled={!on && full}
                onChange={() => toggle(n.slug)}
                className="sr-only"
              />
              {n.name}
            </label>
          );
        })}
      </div>

      {parents.map((p) =>
        p.verticals.length ? (
          <div key={p.slug} className="mt-4 border-l-2 border-shout/30 pl-4">
            <p className="text-xs font-semibold uppercase tracking-[0.12em] text-mute" id={`${uid}-${p.slug}`}>
              Specialisms in {p.name} <span className="font-normal normal-case tracking-normal">(optional)</span>
            </p>
            <div role="group" aria-labelledby={`${uid}-${p.slug}`} className="mt-2 flex flex-wrap gap-2">
              {p.verticals.map((v) => {
                const on = chosen.has(v.slug);
                const picked = p.verticals.filter((x) => chosen.has(x.slug)).length;
                return (
                  <label key={v.slug} className={cx(BOX, "min-h-[36px] text-xs")} title={v.description}>
                    <input
                      type="checkbox"
                      checked={on}
                      disabled={!on && picked >= MAX_VERTICALS}
                      onChange={() => toggle(v.slug, p)}
                      className="sr-only"
                    />
                    {v.name}
                  </label>
                );
              })}
            </div>
          </div>
        ) : null,
      )}

      {cross ? (
        <label className={cx(BOX, "mt-4")} title={cross.description}>
          <input type="checkbox" checked={isCross} onChange={() => toggle(CROSS_INDUSTRY)} className="sr-only" />
          Works across industries
          <span className="font-normal text-mute">(general-purpose)</span>
        </label>
      ) : null}

      <p aria-live="polite" className="mt-2 text-xs text-mute">
        {said}
      </p>
      {error ? (
        <p className="mt-1.5 text-xs font-medium text-shout" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}
