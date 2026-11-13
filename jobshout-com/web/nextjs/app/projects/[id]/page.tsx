import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { BidForm } from "@/components/BidForm";
import { ArrowLeftIcon } from "@/components/icons";
import { Badge, buttonClass } from "@/components/ui";
import {
  formatProjectBudget,
  getProject,
  projectTypeLabel,
} from "@/lib/projects";

export const dynamic = "force-dynamic";

type Params = { params: { id: string } };

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const p = await getProject(params.id).catch(() => null);
  if (!p) return { title: "Project" };
  return {
    title: p.title,
    description: p.summary || p.description.slice(0, 160),
  };
}

export default async function ProjectDetailPage({ params }: Params) {
  const project = await getProject(params.id);
  if (!project) notFound();

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-8 sm:px-8">
      <Link href="/projects" className="inline-flex items-center gap-1.5 text-sm font-medium text-mute hover:text-ink">
        <ArrowLeftIcon className="h-4 w-4" />
        Projects
      </Link>

      <div className="mt-8 grid gap-10 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <Badge tone="signal">{projectTypeLabel(project.project_type)}</Badge>
          <h1 className="mt-4 font-display text-3xl font-semibold tracking-[-0.03em] text-ink sm:text-4xl">
            {project.title}
          </h1>
          <p className="mt-3 text-lg text-mute">{formatProjectBudget(project)}</p>
          <div className="prose-job mt-8 whitespace-pre-wrap text-body leading-relaxed">
            {project.description}
          </div>
          {project.deliverables?.length ? (
            <section className="mt-10">
              <h2 className="font-display text-xl font-semibold text-ink">Deliverables</h2>
              <ul className="mt-3 list-inside list-disc text-body">
                {project.deliverables.map((d) => (
                  <li key={d}>{d}</li>
                ))}
              </ul>
            </section>
          ) : null}
          {project.skills_required?.length ? (
            <section className="mt-10">
              <h2 className="font-display text-xl font-semibold text-ink">Skills</h2>
              <ul className="mt-3 flex flex-wrap gap-2">
                {project.skills_required.map((s) => (
                  <li key={s} className="rounded-pill border border-line px-3 py-1 text-sm text-mute">
                    {s}
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
        </div>
        <aside>
          <div className="rounded-card border border-line bg-surface p-5">
            <p className="text-sm text-mute">
              {project.bid_count ?? 0} bids · {project.duration_estimate || "Flexible timeline"}
            </p>
            <Link href={`/projects/${project.id}/bid`} className={`${buttonClass("primary", "md")} mt-4 w-full justify-center`}>
              Submit a proposal
            </Link>
          </div>
          <div className="mt-6">
            <BidForm project={project} />
          </div>
        </aside>
      </div>
    </div>
  );
}
