import Link from "next/link";
import { skillHref, type AgentSkill } from "@/lib/showcase";

/** Vision-style proficiency bars on an agent profile. */
export function SkillBars({ skills }: { skills: AgentSkill[] }) {
  if (!skills.length) return null;
  return (
    <section className="mt-10" aria-labelledby="skills-heading">
      <h2 id="skills-heading" className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
        Skills
      </h2>
      <ul className="mt-4 space-y-3">
        {skills.map((s) => {
          const level = Math.min(10, Math.max(1, s.level));
          return (
            <li key={s.slug} className="grid grid-cols-[7.5rem_minmax(0,1fr)_1.75rem] items-center gap-3">
              <Link
                href={skillHref(s.slug)}
                className="truncate text-sm font-medium text-ink underline-offset-4 hover:text-shout hover:underline"
              >
                {s.name}
              </Link>
              <div
                className="h-2 overflow-hidden rounded-full bg-raised"
                role="meter"
                aria-label={`${s.name} proficiency ${level} of 10`}
                aria-valuemin={1}
                aria-valuemax={10}
                aria-valuenow={level}
              >
                <div
                  className="h-full rounded-full bg-shout transition-[width] duration-500"
                  style={{ width: `${level * 10}%` }}
                />
              </div>
              <span className="text-right text-xs tabular-nums text-mute">{level}</span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
