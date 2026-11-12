import Link from "next/link";
import type { Metadata } from "next";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import {
  ArrowUpRightIcon,
  ClockIcon,
  InboxIcon,
  MapPinIcon,
  WalletIcon,
} from "@/components/icons";
import { listMyApplications, type JobApplicationWithJob } from "@/lib/api";
import { currentViewer } from "@/lib/session";
import {
  APPLICATION_LABELS,
  employmentLabel,
  formatCompensation,
  initials,
  relativeTime,
  shortLocation,
} from "@/lib/format";
import type { ApplicationStatus } from "@/lib/api";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "My applications",
  description: "Track every role you have applied to on JobShout.com.",
};

const STATUS_TONE: Record<ApplicationStatus, "neutral" | "brand" | "signal" | "good" | "warn"> = {
  submitted: "neutral",
  reviewing: "signal",
  shortlisted: "brand",
  interviewing: "brand",
  offered: "good",
  rejected: "warn",
  withdrawn: "neutral",
};

export default async function ApplicationsPage() {
  const viewer = await currentViewer();

  let applications: JobApplicationWithJob[] = [];
  let error = "";
  if (viewer) {
    try {
      applications = await listMyApplications(viewer);
    } catch (e) {
      error = e instanceof Error ? e.message : "Could not load applications";
    }
  }

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header className="max-w-2xl">
        <h1 className="font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
          My applications
        </h1>
        <p className="mt-4 text-base leading-relaxed text-mute">
          Every role you have applied to, newest first, with where each one stands.
        </p>
      </header>

      {error ? (
        <div className="mt-8 max-w-xl">
          <ErrorNotice title={error} body="The marketplace API did not answer. Try again shortly." />
        </div>
      ) : null}

      <div className="mt-10">
        {!viewer ? (
          <EmptyState
            icon={<InboxIcon className="h-6 w-6" />}
            title="Sign in to see your applications"
            body="Applications are private to the signed-in account that submitted them."
            action={
              <Link href="/login?callbackUrl=%2Fapplications" className={buttonClass("secondary", "md")}>
                Sign in
              </Link>
            }
          />
        ) : applications.length === 0 && !error ? (
          <EmptyState
            icon={<InboxIcon className="h-6 w-6" />}
            title="No applications yet"
            body="Find a role you like and apply — it takes about two minutes."
            action={
              <Link href="/jobs" className={buttonClass("primary", "md")}>
                Browse open roles
              </Link>
            }
          />
        ) : (
          <>
            <p className="break-words text-sm text-mute">
              <span className="font-semibold text-ink">{applications.length}</span>{" "}
              {applications.length === 1 ? "application" : "applications"}
            </p>
            <ul className="mt-5 space-y-3">
              {applications.map((application) => (
                <li key={application.id}>
                  <ApplicationRow application={application} />
                </li>
              ))}
            </ul>
          </>
        )}
      </div>
    </div>
  );
}

function ApplicationRow({ application }: { application: JobApplicationWithJob }) {
  const { job } = application;
  const tone = STATUS_TONE[application.status] ?? "neutral";
  return (
    <article className="rounded-2xl border border-line bg-surface p-5 transition-colors hover:border-ink/20 sm:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-3">
            <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-ink/5 font-display text-sm font-semibold text-ink">
              {initials(job.title)}
            </span>
            <div className="min-w-0">
              <Link
                href={`/jobs/${job.id}`}
                className="font-display text-lg font-semibold tracking-tight text-ink hover:text-shout"
              >
                {job.title}
              </Link>
              <p className="mt-0.5 text-sm text-mute">{job.summary || employmentLabel(job.employment_type)}</p>
            </div>
          </div>
        </div>
        <Badge tone={tone}>{APPLICATION_LABELS[application.status] ?? application.status}</Badge>
      </div>
      <dl className="mt-4 flex flex-wrap gap-x-5 gap-y-2 text-sm text-mute">
        <div className="flex items-center gap-1.5">
          <MapPinIcon className="h-3.5 w-3.5" />
          <span>{shortLocation(job.location)}</span>
        </div>
        <div className="flex items-center gap-1.5">
          <WalletIcon className="h-3.5 w-3.5" />
          <span>{formatCompensation(job.compensation)}</span>
        </div>
        <div className="flex items-center gap-1.5">
          <ClockIcon className="h-3.5 w-3.5" />
          <span>Applied {relativeTime(application.created_at)}</span>
        </div>
      </dl>
      <div className="mt-4">
        <Link
          href={`/jobs/${job.id}`}
          className="inline-flex items-center gap-1 text-sm font-medium text-ink underline-offset-4 hover:underline"
        >
          View role
          <ArrowUpRightIcon className="h-3.5 w-3.5" />
        </Link>
      </div>
    </article>
  );
}
