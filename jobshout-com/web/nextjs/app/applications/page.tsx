import Link from "next/link";
import type { Metadata } from "next";
import { getServerSession } from "next-auth";
import { Badge, EmptyState, ErrorNotice, Input, buttonClass } from "@/components/ui";
import {
  ArrowUpRightIcon,
  ClockIcon,
  InboxIcon,
  MapPinIcon,
  SearchIcon,
  WalletIcon,
} from "@/components/icons";
import { listMyApplications, type JobApplicationWithJob } from "@/lib/api";
import { authOptions } from "@/lib/auth";
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

export default async function ApplicationsPage({
  searchParams,
}: {
  searchParams: { email?: string };
}) {
  const session = await getServerSession(authOptions());
  // Signed-in email wins; ?email= keeps the page usable and shareable without auth.
  const email = (searchParams.email || session?.user?.email || "").trim();

  let applications: JobApplicationWithJob[] = [];
  let error = "";
  if (email) {
    try {
      applications = await listMyApplications(email);
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

      {/* Plain GET form: the email lives in the URL, so the view is shareable. */}
      <form method="get" className="mt-8 flex max-w-xl flex-col gap-3 sm:flex-row">
        <label htmlFor="email" className="sr-only">
          Email you applied with
        </label>
        <Input
          id="email"
          name="email"
          type="email"
          defaultValue={email}
          placeholder="Email you applied with"
          className="flex-1"
        />
        <button type="submit" className={buttonClass("primary", "md")}>
          <SearchIcon className="h-4 w-4" />
          Find applications
        </button>
      </form>

      {error ? (
        <div className="mt-8 max-w-xl">
          <ErrorNotice title={error} body="The marketplace API did not answer. Try again shortly." />
        </div>
      ) : null}

      <div className="mt-10">
        {!email ? (
          <EmptyState
            icon={<InboxIcon className="h-6 w-6" />}
            title="Enter your email to see your applications"
            body="Applications are keyed to the email you applied with. Sign in and this fills itself in."
            action={
              <Link href="/login?callbackUrl=%2Fapplications" className={buttonClass("secondary", "md")}>
                Sign in
              </Link>
            }
          />
        ) : applications.length === 0 && !error ? (
          <EmptyState
            icon={<InboxIcon className="h-6 w-6" />}
            title="No applications for that email yet"
            body={`Nothing has been sent from ${email}. Find a role you like and apply — it takes about two minutes.`}
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
              {applications.length === 1 ? "application" : "applications"} for{" "}
              <span className="break-all">{email}</span>
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

  return (
    <article className="surface-card group relative p-5 transition-all duration-200 ease-out hover:border-edge hover:shadow-lift">
      <div className="flex flex-wrap items-start gap-4">
        <span
          aria-hidden
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-line bg-raised font-display text-sm font-semibold text-ink"
        >
          {initials(job.title)}
        </span>

        <div className="min-w-0 flex-1">
          <h2 className="break-words font-display text-lg font-semibold leading-snug text-ink">
            <Link href={`/jobs/${job.id}`} className="after:absolute after:inset-0">
              {job.title}
            </Link>
          </h2>
          <dl className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1.5 text-sm text-mute">
            <div className="flex items-center gap-1.5">
              <dt className="sr-only">Location</dt>
              <MapPinIcon className="h-4 w-4" />
              <dd>{shortLocation(job.location)}</dd>
            </div>
            <div className="flex items-center gap-1.5">
              <dt className="sr-only">Salary</dt>
              <WalletIcon className="h-4 w-4" />
              <dd>{formatCompensation(job.compensation)}</dd>
            </div>
            <div className="flex items-center gap-1.5">
              <dt className="sr-only">Applied</dt>
              <ClockIcon className="h-4 w-4" />
              <dd>Applied {relativeTime(application.created_at)}</dd>
            </div>
          </dl>
        </div>

        <div className="flex items-center gap-3">
          <Badge tone={STATUS_TONE[application.status]}>
            {APPLICATION_LABELS[application.status]}
          </Badge>
          <ArrowUpRightIcon className="hidden h-5 w-5 text-mute transition-colors duration-200 group-hover:text-shout sm:block" />
        </div>
      </div>

      {application.cover_letter ? (
        <p className="mt-4 line-clamp-2 border-t border-line pt-4 text-sm leading-relaxed text-mute">
          <span className="font-medium text-body">Your message: </span>
          {application.cover_letter}
        </p>
      ) : (
        <p className="mt-4 border-t border-line pt-4 text-xs text-mute">
          {employmentLabel(job.employment_type)}
        </p>
      )}
    </article>
  );
}
