import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { BidForm } from "@/components/BidForm";
import { Badge } from "@/components/ui";
import {
  ArrowLeftIcon,
  BriefcaseIcon,
  ClockIcon,
  WalletIcon,
} from "@/components/icons";
import {
  formatProjectBudget,
  getMockProject,
  projectTypeLabel,
} from "@/lib/projects";

export function generateMetadata({ params }: { params: { id: string } }): Metadata {
  const project = getMockProject(params.id);
  return { title: project ? `Bid · ${project.title}` : "Submit bid" };
}

export default function ProjectBidPage({ params }: { params: { id: string } }) {
  const project = getMockProject(params.id);
  if (!project) notFound();

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <Link
        href={`/projects/${project.id}`}
        className="-mx-2 inline-flex min-h-[44px] items-center gap-2 rounded-pill px-2 text-sm font-medium text-mute transition-colors duration-200 hover:text-shout"
      >
        <ArrowLeftIcon className="h-4 w-4" />
        Back to the project
      </Link>

      <div className="mt-8 grid gap-10 lg:grid-cols-[1fr_19rem] lg:gap-14">
        <div className="min-w-0">
          <h1 className="font-display text-3xl font-semibold tracking-[-0.03em] text-ink sm:text-4xl">
            Submit a bid
          </h1>
          <p className="mt-3 max-w-xl text-base leading-relaxed text-mute">
            Propose your price, timeline, and approach for{" "}
            <span className="font-semibold text-ink">{project.title}</span>. The client sees this
            alongside other freelancers’ bids.
          </p>

          <div className="mt-9">
            <BidForm project={project} />
          </div>
        </div>

        <aside className="lg:sticky lg:top-24 lg:self-start">
          <div className="surface-card p-5">
            <div className="flex flex-wrap gap-1.5">
              <Badge tone="neutral">{projectTypeLabel(project.type)}</Badge>
              <Badge tone="brand">{project.bids} bids so far</Badge>
            </div>
            <p className="mt-3 break-words font-display text-base font-semibold leading-snug text-ink">
              {project.title}
            </p>
            <p className="mt-2 text-sm leading-relaxed text-mute">{project.summary}</p>

            <dl className="mt-5 space-y-3 border-t border-line pt-4 text-sm">
              <Row icon={<WalletIcon className="h-4 w-4" />} label="Budget">
                {formatProjectBudget(project.budget)}
              </Row>
              <Row icon={<ClockIcon className="h-4 w-4" />} label="Duration">
                {project.duration}
              </Row>
              <Row icon={<BriefcaseIcon className="h-4 w-4" />} label="Client">
                {project.client.name}
              </Row>
            </dl>

            {project.deliverables.length > 0 ? (
              <div className="mt-5 border-t border-line pt-4">
                <p className="text-xs font-semibold uppercase tracking-[0.14em] text-mute">
                  Deliverables
                </p>
                <ul className="mt-3 space-y-1.5">
                  {project.deliverables.slice(0, 4).map((d) => (
                    <li key={d} className="text-xs leading-relaxed text-body">
                      · {d}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
          </div>
        </aside>
      </div>
    </div>
  );
}

function Row({
  icon,
  label,
  children,
}: {
  icon: React.ReactNode;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-start gap-2.5">
      <span className="mt-0.5 text-mute">{icon}</span>
      <div className="min-w-0">
        <dt className="text-xs text-mute">{label}</dt>
        <dd className="text-sm font-medium text-ink">{children}</dd>
      </div>
    </div>
  );
}
