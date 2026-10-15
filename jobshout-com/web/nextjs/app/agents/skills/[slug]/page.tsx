import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ArrowLeftIcon, BotIcon } from "@/components/icons";
import { AgentCard } from "@/components/showcase/AppCard";
import { Badge, EmptyState, buttonClass } from "@/components/ui";
import {
  directoryHref,
  getSkill,
  industryNames,
  listApps,
  listIndustries,
  listSkills,
  skillHref,
  type IndustryNode,
  type ShowcaseApp,
} from "@/lib/showcase";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

type Params = { params: { slug: string } };

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const catalog = await listSkills().catch(() => []);
  const skill = catalog.find((s) => s.slug === params.slug) ?? (await getSkill(params.slug));
  if (!skill) return { title: "Not found" };
  return {
    title: `${skill.name} · Agent skills`,
    description: `AI agents in the JobShout directory that specialise in ${skill.name}.`,
    alternates: { canonical: skillHref(skill.slug) },
  };
}

export default async function SkillDetailPage({ params }: Params) {
  const viewer = await currentViewer();
  const [catalog, tree] = await Promise.all([
    listSkills().catch(() => []),
    listIndustries("agent").catch(() => [] as IndustryNode[]),
  ]);
  const skill = catalog.find((s) => s.slug === params.slug) ?? (await getSkill(params.slug));
  if (!skill) notFound();

  let agents: ShowcaseApp[] = [];
  let total = 0;
  try {
    const list = await listApps({ kind: "agent", skill: skill.slug, sort: "stars", limit: 24 }, viewer);
    agents = list.data;
    total = list.total;
  } catch {
    agents = [];
  }
  const names = industryNames(tree);

  return (
    <div>
      <section className="relative overflow-hidden border-b border-line">
        <div aria-hidden className="pointer-events-none absolute inset-0 halo" />
        <div className="relative mx-auto max-w-board px-5 pb-10 pt-12 sm:px-8 sm:pb-12 sm:pt-16">
          <Link
            href="/agents/skills"
            className="inline-flex min-h-[44px] items-center gap-2 text-sm font-medium text-mute transition-colors duration-200 hover:text-ink"
          >
            <ArrowLeftIcon className="h-4 w-4" />
            All skills
          </Link>
          <Badge tone="signal" className="mt-5">
            <BotIcon className="h-3.5 w-3.5" />
            Skill
          </Badge>
          <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-[3.25rem] sm:leading-[1.05]">
            {skill.name}
          </h1>
          <p className="mt-4 max-w-2xl text-base leading-relaxed text-mute sm:text-lg">
            {skill.description ||
              `${total} ${total === 1 ? "agent" : "agents"} in the directory claim this skill with a proficiency level.`}
          </p>
          <div className="mt-6 flex flex-wrap gap-2.5">
            <Link href={directoryHref({ skill: skill.slug })} className={buttonClass("secondary", "md")}>
              Filter directory
            </Link>
            <Link href="/showcase/new?kind=agent" className={buttonClass("primary", "md")}>
              Add an agent
            </Link>
          </div>
        </div>
      </section>

      <div className="mx-auto max-w-board px-5 pb-16 pt-10 sm:px-8">
        {!agents.length ? (
          <EmptyState
            icon={<BotIcon className="h-6 w-6" />}
            title={`No public agents for ${skill.name} yet`}
            body="When an agent lists this skill, it will show here ordered by how strongly they claim it."
          />
        ) : (
          <ul className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {agents.map((a) => (
              <li key={a.id}>
                <AgentCard app={a} industries={names} />
              </li>
            ))}
          </ul>
        )}
        {total > agents.length ? (
          <p className="mt-8 text-center text-sm text-mute">
            Showing {agents.length} of {total}.{" "}
            <Link href={directoryHref({ skill: skill.slug })} className="font-medium text-ink underline underline-offset-4">
              See all in the directory
            </Link>
          </p>
        ) : null}
      </div>
    </div>
  );
}
