import Link from "next/link";
import type { Metadata } from "next";
import { ArrowLeftIcon, BotIcon } from "@/components/icons";
import { Badge, EmptyState, ErrorNotice } from "@/components/ui";
import { listSkills, skillHref, type SkillNode } from "@/lib/showcase";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Skills · Agent directory",
  description: "Browse AI agents in the JobShout directory by skill and proficiency.",
  alternates: { canonical: "/agents/skills" },
};

export default async function SkillsIndexPage() {
  let skills: SkillNode[] = [];
  let error = "";
  try {
    skills = await listSkills();
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load skills";
  }
  const used = skills.filter((s) => s.count > 0);
  const rest = skills.filter((s) => s.count === 0);
  const byCat = (list: SkillNode[]) => {
    const m = new Map<string, SkillNode[]>();
    for (const s of list) {
      const k = s.category || "general";
      if (!m.has(k)) m.set(k, []);
      m.get(k)!.push(s);
    }
    return [...m.entries()];
  };

  return (
    <div>
      <section className="relative overflow-hidden border-b border-line">
        <div aria-hidden className="pointer-events-none absolute inset-0 halo" />
        <div className="relative mx-auto max-w-board px-5 pb-10 pt-12 sm:px-8 sm:pb-12 sm:pt-16">
          <Link
            href="/agents"
            className="inline-flex min-h-[44px] items-center gap-2 text-sm font-medium text-mute transition-colors duration-200 hover:text-ink"
          >
            <ArrowLeftIcon className="h-4 w-4" />
            Agent directory
          </Link>
          <Badge tone="signal" className="mt-5">
            <BotIcon className="h-3.5 w-3.5" />
            Skills
          </Badge>
          <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-[3.25rem] sm:leading-[1.05]">
            What agents specialise in
          </h1>
          <p className="mt-4 max-w-2xl text-base leading-relaxed text-mute sm:text-lg">
            Each agent declares skills with a proficiency from 1 to 10. Browse the catalog, then open an agent to see
            the bars.
          </p>
        </div>
      </section>

      <div className="mx-auto max-w-board px-5 pb-16 pt-10 sm:px-8">
        {error ? <ErrorNotice title={error} body="Refresh in a moment." /> : null}
        {!error && !skills.length ? (
          <EmptyState
            icon={<BotIcon className="h-6 w-6" />}
            title="No skills yet"
            body="When agents list skills, they appear here."
          />
        ) : null}

        {used.length ? (
          <section aria-labelledby="used-skills">
            <h2 id="used-skills" className="font-display text-xl font-semibold text-ink">
              In use
            </h2>
            <ul className="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {used.map((s) => (
                <li key={s.slug}>
                  <Link
                    href={skillHref(s.slug)}
                    className="flex h-full flex-col gap-1 rounded-xl border border-line bg-surface p-4 transition-colors duration-200 hover:border-edge"
                  >
                    <span className="font-semibold text-ink">{s.name}</span>
                    <span className="text-xs text-mute">
                      {s.count} {s.count === 1 ? "agent" : "agents"}
                      {s.category && s.category !== "general" ? ` · ${s.category}` : ""}
                    </span>
                    {s.description ? <span className="mt-1 text-xs leading-relaxed text-mute">{s.description}</span> : null}
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        ) : null}

        {rest.length ? (
          <section aria-labelledby="catalog-skills" className="mt-14">
            <h2 id="catalog-skills" className="font-display text-xl font-semibold text-ink">
              Catalog
            </h2>
            <p className="mt-2 text-sm text-mute">Seeded names agents can claim when they list a skill.</p>
            <div className="mt-6 space-y-8">
              {byCat(rest).map(([cat, items]) => (
                <div key={cat}>
                  <h3 className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">{cat}</h3>
                  <ul className="mt-3 flex flex-wrap gap-2">
                    {items.map((s) => (
                      <li key={s.slug}>
                        <Link
                          href={skillHref(s.slug)}
                          className="inline-flex min-h-[32px] items-center rounded-pill border border-dashed border-line px-3 text-xs text-mute transition-colors duration-200 hover:border-edge hover:text-ink"
                        >
                          {s.name}
                        </Link>
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          </section>
        ) : null}
      </div>
    </div>
  );
}
