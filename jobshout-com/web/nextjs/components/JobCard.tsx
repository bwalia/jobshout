import Link from "next/link";
import { Badge } from "@/components/ui";
import { ArrowUpRightIcon, GlobeIcon, MapPinIcon, WalletIcon } from "@/components/icons";
import type { Job } from "@/lib/api";
import {
  employmentLabel,
  formatCompensation,
  hasSalary,
  initials,
  isFresh,
  jobCategory,
  relativeTime,
  shortLocation,
} from "@/lib/format";

export function JobCard({ job, priority }: { job: Job; priority?: boolean }) {
  const salary = hasSalary(job.compensation);

  return (
    <article className="group relative">
      <div className="surface-card h-full p-5 transition-all duration-200 ease-out group-hover:-translate-y-0.5 group-hover:border-edge group-hover:shadow-lift sm:p-6">
        <div className="flex items-start gap-4">
          <span
            aria-hidden
            className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-line bg-raised font-display text-sm font-semibold text-ink"
          >
            {initials(job.title)}
          </span>

          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone="neutral">{jobCategory(job)}</Badge>
              {isFresh(job) ? <Badge tone="brand">New</Badge> : null}
              {job.location.remote ? (
                <Badge tone="signal">
                  <GlobeIcon className="h-3 w-3" />
                  Remote
                </Badge>
              ) : null}
            </div>

            <h3 className="break-words mt-3 font-display text-lg font-semibold leading-snug tracking-[-0.01em] text-ink">
              {/* Stretched link: the whole card is the hit target, one tab stop. */}
              <Link href={`/jobs/${job.id}`} className="after:absolute after:inset-0">
                {job.title}
              </Link>
            </h3>

            <p className="mt-2 line-clamp-2 text-sm leading-relaxed text-mute">
              {job.summary || job.description}
            </p>
          </div>

          <ArrowUpRightIcon className="hidden h-5 w-5 shrink-0 text-mute transition-colors duration-200 group-hover:text-shout sm:block" />
        </div>

        <dl className="mt-5 flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-line pt-4 text-sm">
          <div className="flex items-center gap-1.5 text-mute">
            <dt className="sr-only">Location</dt>
            <MapPinIcon className="h-4 w-4" />
            <dd>{shortLocation(job.location)}</dd>
          </div>
          <div className="flex items-center gap-1.5">
            <dt className="sr-only">Salary</dt>
            <WalletIcon className={`h-4 w-4 ${salary ? "text-good" : "text-mute"}`} />
            <dd className={salary ? "font-semibold text-ink" : "text-mute"}>
              {formatCompensation(job.compensation)}
            </dd>
          </div>
          <div className="ml-auto flex items-center gap-3 text-xs text-mute">
            <span>{employmentLabel(job.employment_type)}</span>
            <span aria-hidden className="h-1 w-1 rounded-full bg-edge" />
            <span>{relativeTime(job.published_at || job.created_at)}</span>
          </div>
        </dl>

        {job.requirements.length > 0 ? (
          <ul className="mt-4 flex flex-wrap gap-1.5">
            {job.requirements.slice(0, priority ? 6 : 4).map((r) => (
              <li
                key={r}
                className="rounded-pill border border-line bg-raised px-2.5 py-1 text-xs text-body"
              >
                {r}
              </li>
            ))}
            {job.requirements.length > (priority ? 6 : 4) ? (
              <li className="px-1 py-1 text-xs text-mute">
                +{job.requirements.length - (priority ? 6 : 4)} more
              </li>
            ) : null}
          </ul>
        ) : null}
      </div>
    </article>
  );
}

export function JobCardSkeleton() {
  return (
    <div className="surface-card p-6">
      <div className="flex gap-4">
        <div className="skeleton h-11 w-11 rounded-xl" />
        <div className="flex-1 space-y-3">
          <div className="skeleton h-4 w-24 rounded-pill" />
          <div className="skeleton h-5 w-3/4 rounded" />
          <div className="skeleton h-4 w-full rounded" />
        </div>
      </div>
      <div className="skeleton mt-5 h-4 w-1/2 rounded" />
    </div>
  );
}
