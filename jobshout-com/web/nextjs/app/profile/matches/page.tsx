import Link from "next/link";
import type { Metadata } from "next";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import {
  ArrowUpRightIcon,
  CheckIcon,
  GlobeIcon,
  MapPinIcon,
  TargetIcon,
  WalletIcon,
} from "@/components/icons";
import { listProfileMatches, type JobMatch } from "@/lib/api";
import { formatCompensation, initials, jobCategory, shortLocation } from "@/lib/format";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Ranked matches",
  description: "Open roles ranked against your profile, with the reasoning attached.",
};

export default async function ProfileMatchesPage({
  searchParams,
}: {
  searchParams: { id?: string };
}) {
  const id = searchParams.id ?? "";
  let matches: JobMatch[] = [];
  let error = "";

  if (id) {
    try {
      matches = await listProfileMatches(id);
    } catch (e) {
      error = e instanceof Error ? e.message : "Could not load matches";
    }
  }

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header className="flex flex-wrap items-end justify-between gap-6 border-b border-line pb-8">
        <div className="max-w-2xl">
          <Badge tone="brand">
            <TargetIcon className="h-3.5 w-3.5" />
            Career Agent
          </Badge>
          <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
            Ranked for you
          </h1>
          <p className="mt-4 text-base leading-relaxed text-mute">
            Every score comes with its reasons, so you can tell a real fit from a keyword
            collision.
          </p>
        </div>
        <Link href="/profile" className={buttonClass("secondary", "md")}>
          Edit profile
        </Link>
      </header>

      {error ? (
        <div className="mt-8">
          <ErrorNotice title={error} />
        </div>
      ) : null}

      <div className="mt-8">
        {!id ? (
          <EmptyState
            icon={<TargetIcon className="h-6 w-6" />}
            title="Save a profile first"
            body="Matches are ranked against a saved profile. Fill one in and you will land back here."
            action={
              <Link href="/profile" className={buttonClass("primary", "md")}>
                Build my profile
              </Link>
            }
          />
        ) : matches.length === 0 && !error ? (
          <EmptyState
            icon={<TargetIcon className="h-6 w-6" />}
            title="No strong matches yet"
            body="Nothing on the board clears the bar for your profile. Add more skills or preferred roles, or browse the whole board yourself."
            action={
              <div className="flex flex-wrap justify-center gap-3">
                <Link href="/profile" className={buttonClass("primary", "md")}>
                  Add more skills
                </Link>
                <Link href="/jobs" className={buttonClass("secondary", "md")}>
                  Browse everything
                </Link>
              </div>
            }
          />
        ) : (
          <ul className="space-y-4">
            {matches.map((match) => (
              <li key={match.job.id}>
                <MatchRow match={match} />
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function MatchRow({ match }: { match: JobMatch }) {
  const { job, score, reasons } = match;
  const tone = score >= 75 ? "good" : score >= 50 ? "signal" : "neutral";

  return (
    <article className="surface-card group relative p-5 transition-all duration-200 ease-out hover:border-edge hover:shadow-lift sm:p-6">
      <div className="flex flex-wrap items-start gap-5">
        <ScoreDial score={score} />

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone="neutral">{jobCategory(job)}</Badge>
            {job.location.remote ? (
              <Badge tone="signal">
                <GlobeIcon className="h-3 w-3" />
                Remote
              </Badge>
            ) : null}
            <Badge tone={tone}>{score >= 75 ? "Strong fit" : score >= 50 ? "Worth a look" : "Loose fit"}</Badge>
          </div>

          <h2 className="break-words mt-3 font-display text-xl font-semibold leading-snug tracking-[-0.01em] text-ink">
            <Link href={`/jobs/${job.id}`} className="after:absolute after:inset-0">
              {job.title}
            </Link>
          </h2>

          <dl className="mt-2 flex flex-wrap items-center gap-x-5 gap-y-1.5 text-sm text-mute">
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
          </dl>

          <p className="mt-3 line-clamp-2 text-sm leading-relaxed text-mute">
            {job.summary || job.description}
          </p>

          {reasons.length > 0 ? (
            <ul className="mt-4 grid gap-2 border-t border-line pt-4 sm:grid-cols-2">
              {reasons.map((reason) => (
                <li key={reason} className="flex items-start gap-2.5 text-sm text-body">
                  <CheckIcon className="mt-0.5 h-4 w-4 shrink-0 text-good" />
                  <span>{reason}</span>
                </li>
              ))}
            </ul>
          ) : null}
        </div>

        <ArrowUpRightIcon className="hidden h-5 w-5 shrink-0 text-mute transition-colors duration-200 group-hover:text-shout sm:block" />
      </div>
    </article>
  );
}

/** Ring plus number — the shape reads at a glance, the number is exact. */
function ScoreDial({ score }: { score: number }) {
  const clamped = Math.max(0, Math.min(100, score));
  const radius = 26;
  const circumference = 2 * Math.PI * radius;
  const stroke = (clamped / 100) * circumference;

  return (
    <div className="relative flex h-16 w-16 shrink-0 items-center justify-center">
      <svg viewBox="0 0 64 64" className="h-16 w-16 -rotate-90" aria-hidden>
        <circle
          cx="32"
          cy="32"
          r={radius}
          fill="none"
          strokeWidth="5"
          className="stroke-line"
        />
        <circle
          cx="32"
          cy="32"
          r={radius}
          fill="none"
          strokeWidth="5"
          strokeLinecap="round"
          strokeDasharray={`${stroke} ${circumference}`}
          className={clamped >= 75 ? "stroke-good" : clamped >= 50 ? "stroke-signal" : "stroke-edge"}
        />
      </svg>
      <span className="absolute font-display text-lg font-semibold text-ink">
        {clamped}
        <span className="sr-only"> out of 100 match score</span>
      </span>
    </div>
  );
}
