import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { JobCard } from "@/components/JobCard";
import { Badge, buttonClass } from "@/components/ui";
import {
  ArrowLeftIcon,
  BriefcaseIcon,
  CheckIcon,
  ClockIcon,
  GlobeIcon,
  MapPinIcon,
  SendIcon,
  TargetIcon,
  WalletIcon,
} from "@/components/icons";
import { getJob, listJobs, type Job } from "@/lib/api";
import { similarJobs } from "@/lib/filter";
import {
  employmentLabel,
  formatCompensation,
  formatLocation,
  hasSalary,
  initials,
  isFresh,
  jobCategory,
  relativeTime,
} from "@/lib/format";

export const dynamic = "force-dynamic";

export async function generateMetadata({
  params,
}: {
  params: { id: string };
}): Promise<Metadata> {
  const job = await getJob(params.id).catch(() => null);
  if (!job) return { title: "Role not found" };
  return {
    title: job.title,
    description: job.summary || job.description.slice(0, 155),
  };
}

export default async function JobDetailPage({ params }: { params: { id: string } }) {
  const job = await getJob(params.id);
  if (!job) notFound();

  let related: Job[] = [];
  try {
    related = similarJobs(await listJobs(60), job);
  } catch {
    related = [];
  }

  return (
    <div className="pb-24">
      {/* ------------------------------------------------------------- Header */}
      <div className="border-b border-line bg-subtle">
        <div className="mx-auto max-w-board px-5 pb-10 pt-8 sm:px-8">
          <Link
            href="/jobs"
            className="-mx-2 inline-flex min-h-[44px] items-center gap-2 rounded-pill px-2 text-sm font-medium text-mute transition-colors duration-200 hover:text-shout"
          >
            <ArrowLeftIcon className="h-4 w-4" />
            All open roles
          </Link>

          <div className="mt-7 flex flex-wrap items-start gap-5">
            <span
              aria-hidden
              className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl border border-line bg-surface font-display text-xl font-semibold text-ink"
            >
              {initials(job.title)}
            </span>

            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <Badge tone="neutral">{jobCategory(job)}</Badge>
                {isFresh(job) ? <Badge tone="brand">New this week</Badge> : null}
                {job.location.remote ? (
                  <Badge tone="signal">
                    <GlobeIcon className="h-3 w-3" />
                    Remote
                  </Badge>
                ) : null}
                {job.status !== "published" ? (
                  <Badge tone="warn">{job.status}</Badge>
                ) : null}
              </div>

              <h1 className="break-words mt-3 font-display text-3xl font-semibold leading-[1.1] tracking-[-0.03em] text-ink sm:text-[2.75rem]">
                {job.title}
              </h1>

              <dl className="mt-4 flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
                <Meta icon={<MapPinIcon className="h-4 w-4" />} label="Location">
                  {formatLocation(job.location)}
                </Meta>
                <Meta icon={<BriefcaseIcon className="h-4 w-4" />} label="Work type">
                  {employmentLabel(job.employment_type)}
                </Meta>
                <Meta
                  icon={<WalletIcon className="h-4 w-4" />}
                  label="Salary"
                  emphasis={hasSalary(job.compensation)}
                >
                  {formatCompensation(job.compensation)}
                </Meta>
                <Meta icon={<ClockIcon className="h-4 w-4" />} label="Posted">
                  {relativeTime(job.published_at || job.created_at)}
                </Meta>
              </dl>
            </div>
          </div>
        </div>
      </div>

      {/* -------------------------------------------------- Body + apply panel */}
      <div className="mx-auto grid max-w-board gap-10 px-5 pt-12 sm:px-8 lg:grid-cols-[1fr_20rem] lg:gap-14">
        <div className="min-w-0">
          {job.summary ? (
            <p className="border-l-2 border-shout pl-5 font-display text-xl leading-relaxed text-ink">
              {job.summary}
            </p>
          ) : null}

          <article className="job-prose mt-10">{job.description}</article>

          {job.requirements.length > 0 ? (
            <section className="mt-12 border-t border-line pt-10">
              <h2 className="font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
                What they are looking for
              </h2>
              <ul className="mt-6 grid gap-3 sm:grid-cols-2">
                {job.requirements.map((r) => (
                  <li
                    key={r}
                    className="flex items-start gap-3 rounded-xl border border-line bg-surface px-4 py-3"
                  >
                    <CheckIcon className="mt-0.5 h-4 w-4 shrink-0 text-good" />
                    <span className="text-sm text-body">{r}</span>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}

          <section className="mt-12 flex flex-wrap items-center gap-4 border-t border-line pt-10">
            <Link href={`/jobs/${job.id}/apply`} className={buttonClass("primary", "lg")}>
              <SendIcon className="h-4 w-4" />
              Apply for this role
            </Link>
            <Link href="/profile" className={buttonClass("secondary", "lg")}>
              <TargetIcon className="h-4 w-4" />
              Score it against my profile
            </Link>
          </section>
        </div>

        {/* Sticky on desktop so the CTA is always one click away. */}
        <aside className="lg:sticky lg:top-24 lg:self-start">
          <div className="surface-card p-6 shadow-card">
            <p className="text-xs font-semibold uppercase tracking-[0.14em] text-mute">
              Compensation
            </p>
            <p className="mt-2 font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
              {formatCompensation(job.compensation)}
            </p>

            <dl className="mt-5 space-y-3 border-t border-line pt-5 text-sm">
              <PanelRow label="Work type">{employmentLabel(job.employment_type)}</PanelRow>
              <PanelRow label="Location">{formatLocation(job.location)}</PanelRow>
              <PanelRow label="Remote">{job.location.remote ? "Yes" : "On site"}</PanelRow>
              <PanelRow label="Posted">
                {relativeTime(job.published_at || job.created_at)}
              </PanelRow>
            </dl>

            <Link
              href={`/jobs/${job.id}/apply`}
              className={buttonClass("primary", "md", "mt-6 w-full")}
            >
              Apply now
            </Link>
            <p className="mt-3 text-center text-xs leading-relaxed text-mute">
              Takes about two minutes. Nothing is sent until you press send.
            </p>
          </div>
        </aside>
      </div>

      {/* ------------------------------------------------------------ Related */}
      {related.length > 0 ? (
        <section className="mx-auto max-w-board px-5 pt-20 sm:px-8">
          <h2 className="font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
            Similar roles
          </h2>
          <ul className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {related.map((j) => (
              <li key={j.id}>
                <JobCard job={j} />
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}

function Meta({
  icon,
  label,
  emphasis,
  children,
}: {
  icon: React.ReactNode;
  label: string;
  emphasis?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-center gap-2">
      <dt className="sr-only">{label}</dt>
      <span className={emphasis ? "text-good" : "text-mute"}>{icon}</span>
      <dd className={emphasis ? "font-semibold text-ink" : "text-body"}>{children}</dd>
    </div>
  );
}

function PanelRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <dt className="text-mute">{label}</dt>
      <dd className="text-right font-medium text-ink">{children}</dd>
    </div>
  );
}
