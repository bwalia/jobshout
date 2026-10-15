"use client";

import { useState } from "react";
import { Button, Input, cx } from "@/components/ui";
import type { AgentSkill, SkillNode } from "@/lib/showcase";

type Row = { name: string; level: number };

const MAX = 15;

function fromInitial(skills?: AgentSkill[]): Row[] {
  if (skills?.length) return skills.map((s) => ({ name: s.name, level: s.level }));
  return [{ name: "", level: 8 }];
}

/** Agent form: skill name + proficiency 1–10. Submits as parallel skill_name / skill_level fields. */
export function SkillsEditor({
  initial,
  suggestions = [],
  error,
}: {
  initial?: AgentSkill[];
  suggestions?: SkillNode[];
  error?: string;
}) {
  const [rows, setRows] = useState<Row[]>(() => fromInitial(initial));

  function set(i: number, patch: Partial<Row>) {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, ...patch } : r)));
  }

  return (
    <div>
      <input type="hidden" name="skills_present" value="1" />
      <p className="text-sm font-semibold text-ink" id="skills-label">
        Skills
      </p>
      <p className="mt-1 text-xs leading-relaxed text-mute">
        Languages, frameworks and domains, each with a proficiency from 1 to 10. Up to {MAX}. New names join the
        catalog when you save.
      </p>
      <ul className="mt-3 space-y-2.5" aria-labelledby="skills-label">
        {rows.map((r, i) => (
          <li key={i} className="flex flex-wrap items-center gap-2 sm:flex-nowrap">
            <Input
              name="skill_name"
              value={r.name}
              onChange={(e) => set(i, { name: e.target.value })}
              placeholder="Rust"
              list="showcase-skill-suggestions"
              className="min-w-0 flex-1"
              maxLength={32}
            />
            <label className="flex shrink-0 items-center gap-2 text-xs text-mute">
              <span className="sr-only">Level</span>
              <input
                type="range"
                name="skill_level"
                min={1}
                max={10}
                value={r.level}
                onChange={(e) => set(i, { level: Number(e.target.value) })}
                className="w-28 accent-[var(--shout)]"
              />
              <span className="w-5 tabular-nums text-ink">{r.level}</span>
            </label>
            {rows.length > 1 ? (
              <button
                type="button"
                onClick={() => setRows((prev) => prev.filter((_, idx) => idx !== i))}
                className="min-h-[40px] rounded-pill px-2 text-sm text-mute hover:text-ink"
                aria-label={`Remove ${r.name || "skill"}`}
              >
                ×
              </button>
            ) : null}
          </li>
        ))}
      </ul>
      {suggestions.length ? (
        <datalist id="showcase-skill-suggestions">
          {suggestions.map((s) => (
            <option key={s.slug} value={s.name} />
          ))}
        </datalist>
      ) : null}
      {rows.length < MAX ? (
        <Button
          type="button"
          variant="secondary"
          size="sm"
          className={cx("mt-3")}
          onClick={() => setRows((prev) => [...prev, { name: "", level: 7 }])}
        >
          Add a skill
        </Button>
      ) : null}
      {error ? (
        <p role="alert" className="mt-1.5 text-xs font-medium text-shout">
          {error}
        </p>
      ) : null}
    </div>
  );
}
