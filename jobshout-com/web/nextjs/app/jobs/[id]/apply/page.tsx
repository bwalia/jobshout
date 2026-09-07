import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { getServerSession } from "next-auth";
import { ApplyForm } from "@/components/ApplyForm";
import { Badge } from "@/components/ui";
import {
  ArrowLeftIcon,
  BriefcaseIcon,
  GlobeIcon,
  MapPinIcon,
  WalletIcon,
} from "@/components/icons";
import { getJob } from "@/lib/api";
import { authOptions } from "@/lib/auth";
import {
  employmentLabel,
  formatCompensation,
  formatLocation,
  initials,
  jobCategory,
} from "@/lib/format";

export const dynamic = "force-dynamic";

export async function generateMetadata({
  params,
}: {
  params: { id: string };
}): Promise<Metadata> {
  const job = await getJob(params.id).catch(() => null);
  return { title: job ? `Apply · ${job.title}` : "Apply" };
}

export default async function ApplyPage({ params }: { params: { id: string } }) {
  const job = await getJob(params.id);
  if (!job) notFound();

  const session = await getServerSession(authOptions());

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <Link
        href={`/jobs/${job.id}`}
        className="-mx-2 inline-flex min-h-[44px] items-center gap-2 rounded-pill px-2 text-sm font-medium text-mute transition-colors duration-200 hover:text-shout"
      >
        <ArrowLeftIcon className="h-4 w-4" />
        Back to the role
      </Link>

      <div className="mt-8 grid gap-10 lg:grid-cols-[1fr_19rem] lg:gap-14">
        <div className="min-w-0">
          <h1 className="font-display text-3xl font-semibold tracking-[-0.03em] text-ink sm:text-4xl">
            Apply for {job.title}
          </h1>
          <p className="mt-3 max-w-xl text-base leading-relaxed text-mute">
            Four fields and a message. Your details go straight to the hiring team — no agent
            sends anything on your behalf.
          </p>

          <div className="mt-9">
            <ApplyForm
              job={job}
              defaultName={session?.user?.name ?? ""}
              defaultEmail={session?.user?.email ?? ""}
            />
          </div>
        </div>

        {/* The role stays visible while the form is filled in. */}
        <aside className="lg:sticky lg:top-24 lg:self-start">
          <div className="surface-card p-5">
            <div className="flex items-start gap-3">
              <span
                aria-hidden
                className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-line bg-raised font-display text-sm font-semibold text-ink"
              >
                {initials(job.title)}
              </span>
              <div className="min-w-0">
                <p className="break-words font-display text-base font-semibold leading-snug text-ink">
                  {job.title}
                </p>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  <Badge tone="neutral">{jobCategory(job)}</Badge>
                  {job.location.remote ? (
                    <Badge tone="signal">
                      <GlobeIcon className="h-3 w-3" />
                      Remote
                    </Badge>
                  ) : null}
                </div>
              </div>
            </div>

            <dl className="mt-5 space-y-3 border-t border-line pt-4 text-sm">
              <Row icon={<WalletIcon className="h-4 w-4" />} label="Salary">
                {formatCompensation(job.compensation)}
              </Row>
              <Row icon={<MapPinIcon className="h-4 w-4" />} label="Location">
                {formatLocation(job.location)}
              </Row>
              <Row icon={<BriefcaseIcon className="h-4 w-4" />} label="Work type">
                {employmentLabel(job.employment_type)}
              </Row>
            </dl>

            {job.requirements.length > 0 ? (
              <div className="mt-5 border-t border-line pt-4">
                <p className="text-xs font-semibold uppercase tracking-[0.14em] text-mute">
                  Worth mentioning
                </p>
                <ul className="mt-3 flex flex-wrap gap-1.5">
                  {job.requirements.slice(0, 8).map((r) => (
                    <li
                      key={r}
                      className="rounded-pill border border-line bg-raised px-2.5 py-1 text-xs text-body"
                    >
                      {r}
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
