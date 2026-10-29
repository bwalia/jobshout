import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { Badge, buttonClass } from "@/components/ui";
import {
  ArrowLeftIcon,
  BriefcaseIcon,
  ClockIcon,
  MapPinIcon,
  WalletIcon,
} from "@/components/icons";
import {
  formatProjectBudget,
  getMockProject,
  projectTypeLabel,
} from "@/lib/projects";

export function generateMetadata({ params }: { params: { id: string } }): Metadata {
  const project = getMockProject(params.id);
  return {
    title: project ? project.title : "Project not found",
    description: project?.summary,
  };
}

export default function ProjectDetailPage({ params }: { params: { id: string } }) {
  const project = getMockProject(params.id);
  if (!project) notFound();

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <Link
        href="/projects"
        className="-mx-2 inline-flex min-h-[44px] items-center gap-2 rounded-pill px-2 text-sm font-medium text-mute transition-colors duration-200 hover:text-shout"
      >
        <ArrowLeftIcon className="h-4 w-4" />
        All projects
      </Link>

      <div className="mt-8 grid gap-10 lg:grid-cols-[1fr_19rem] lg:gap-14">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone="neutral">{projectTypeLabel(project.type)}</Badge>
            <Badge tone="brand">{project.bids} bids</Badge>
          </div>

          <h1 className="mt-3 font-display text-3xl font-semibold tracking-[-0.03em] text-ink sm:text-4xl">
            {project.title}
          </h1>
          <p className="mt-3 max-w-2xl text-base leading-relaxed text-mute">{project.summary}</p>

          <dl className="mt-6 flex flex-wrap gap-x-6 gap-y-3 text-sm">
            <Meta icon={<WalletIcon className="h-4 w-4" />} label="Budget">
              {formatProjectBudget(project.budget)}
            </Meta>
            <Meta icon={<ClockIcon className="h-4 w-4" />} label="Duration">
              {project.duration}
            </Meta>
            <Meta icon={<BriefcaseIcon className="h-4 w-4" />} label="Posted">
              {project.posted}
            </Meta>
          </dl>

          <section className="mt-10">
            <h2 className="font-display text-xl font-semibold text-ink">About the project</h2>
            <p className="mt-3 whitespace-pre-wrap text-base leading-relaxed text-body">
              {project.description}
            </p>
          </section>

          <section className="mt-10">
            <h2 className="font-display text-xl font-semibold text-ink">Deliverables</h2>
            <ul className="mt-4 space-y-2">
              {project.deliverables.map((item) => (
                <li
                  key={item}
                  className="flex items-start gap-2 text-sm leading-relaxed text-body"
                >
                  <span aria-hidden className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-shout" />
                  {item}
                </li>
              ))}
            </ul>
          </section>

          <section className="mt-10">
            <h2 className="font-display text-xl font-semibold text-ink">Skills</h2>
            <ul className="mt-4 flex flex-wrap gap-1.5">
              {project.skills.map((skill) => (
                <li
                  key={skill}
                  className="rounded-pill border border-line bg-raised px-2.5 py-1 text-xs font-medium text-body"
                >
                  {skill}
                </li>
              ))}
            </ul>
          </section>

          <div className="mt-12 flex flex-wrap gap-3">
            <Link href={`/projects/${project.id}/bid`} className={buttonClass("primary", "lg")}>
              Submit a bid
            </Link>
            <Link href="/projects" className={buttonClass("secondary", "lg")}>
              Keep browsing
            </Link>
          </div>
        </div>

        <aside className="lg:sticky lg:top-24 lg:self-start">
          <div className="surface-card p-5">
            <p className="text-xs font-semibold uppercase tracking-[0.14em] text-mute">Client</p>
            <p className="mt-2 font-display text-lg font-semibold text-ink">{project.client.name}</p>
            <dl className="mt-4 space-y-3 border-t border-line pt-4 text-sm">
              <div className="flex items-start gap-2.5">
                <MapPinIcon className="mt-0.5 h-4 w-4 text-mute" />
                <div>
                  <dt className="text-xs text-mute">Location</dt>
                  <dd className="font-medium text-ink">{project.client.location}</dd>
                </div>
              </div>
              <div>
                <dt className="text-xs text-mute">Projects posted</dt>
                <dd className="font-medium text-ink">{project.client.projectsPosted}</dd>
              </div>
              <div>
                <dt className="text-xs text-mute">Hire rate</dt>
                <dd className="font-medium text-ink">{project.client.hireRate}</dd>
              </div>
            </dl>

            <Link
              href={`/projects/${project.id}/bid`}
              className={buttonClass("primary", "md", "mt-5 w-full")}
            >
              Submit bid
            </Link>
          </div>
        </aside>
      </div>
    </div>
  );
}

function Meta({
  icon,
  label,
  children,
}: {
  icon: React.ReactNode;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-mute">{icon}</span>
      <div>
        <dt className="sr-only">{label}</dt>
        <dd className="font-medium text-ink">{children}</dd>
      </div>
    </div>
  );
}
