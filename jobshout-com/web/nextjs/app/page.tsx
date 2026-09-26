import Link from "next/link";
import { JobCard } from "@/components/JobCard";
import { JobSearchBar } from "@/components/JobSearchBar";
import { Badge, Eyebrow, SectionHeading, buttonClass } from "@/components/ui";
import {
  ArrowRightIcon,
  BriefcaseIcon,
  FileTextIcon,
  GlobeIcon,
  SendIcon,
  ShieldIcon,
  SparkIcon,
  TargetIcon,
  UserIcon,
} from "@/components/icons";
import { listJobs, type Job } from "@/lib/api";
import { jobCategory } from "@/lib/format";

export const dynamic = "force-dynamic";

const POPULAR = ["Rust", "Kubernetes", "Frontend", "Data", "Security", "Remote"];

const CANDIDATE_STEPS = [
  {
    icon: <UserIcon className="h-5 w-5" />,
    title: "Build a profile once",
    body: "Skills, preferred roles, salary floor. Your Career Agent uses it to rank every new posting.",
  },
  {
    icon: <TargetIcon className="h-5 w-5" />,
    title: "See why each role fits",
    body: "Explainable match scores — not a black box. Every score comes with the reasons behind it.",
  },
  {
    icon: <SendIcon className="h-5 w-5" />,
    title: "Apply when you say so",
    body: "Nothing is ever sent on your behalf without you approving it first.",
  },
];

const EMPLOYER_STEPS = [
  {
    icon: <FileTextIcon className="h-5 w-5" />,
    title: "Post in two minutes",
    body: "Title, description, salary, requirements. Publish straight away or save it as a draft.",
  },
  {
    icon: <SparkIcon className="h-5 w-5" />,
    title: "Hiring Agents shortlist",
    body: "Candidates are ranked against your requirements, with the reasoning attached.",
  },
  {
    icon: <ShieldIcon className="h-5 w-5" />,
    title: "Humans stay in the loop",
    body: "Agents propose. Your team decides who moves forward. Every action is logged.",
  },
];

export default async function HomePage() {
  let jobs: Job[] = [];
  try {
    jobs = await listJobs(60);
  } catch {
    // The board is a shop window — a cold API should not blank the landing page.
    jobs = [];
  }

  const featured = jobs.slice(0, 6);
  const remoteCount = jobs.filter((j) => j.location.remote).length;
  const categories = countCategories(jobs);

  return (
    <>
      {/* ---------------------------------------------------------------- Hero */}
      <section className="relative overflow-hidden border-b border-line">
        <div aria-hidden className="pointer-events-none absolute inset-0 halo" />
        <div aria-hidden className="pointer-events-none absolute inset-0 grid-field" />

        <div className="relative mx-auto max-w-board px-5 pb-16 pt-16 sm:px-8 sm:pb-24 sm:pt-24">
          <div className="mx-auto max-w-3xl text-center">
            <div className="flex justify-center">
              <Badge tone="brand">
                <SparkIcon className="h-3.5 w-3.5" />
                AI-native employment marketplace
              </Badge>
            </div>

            <h1 className="mt-7 animate-rise-in font-display text-[clamp(2.5rem,7vw,4.75rem)] font-semibold leading-[1.02] tracking-[-0.035em] text-ink">
              Find work worth
              <br className="hidden sm:block" />{" "}
              <span className="relative inline-block">
                shouting about
                <span
                  aria-hidden
                  className="absolute -bottom-1 left-0 h-[0.18em] w-full rounded-full bg-shout/70"
                />
              </span>
            </h1>

            <p className="mx-auto mt-7 max-w-xl text-base leading-relaxed text-mute sm:text-lg">
              Search every open role on the board, apply in a couple of minutes, and let a Career
              Agent tell you exactly why a job fits — before you spend an evening on it.
            </p>
          </div>

          <div className="mx-auto mt-10 max-w-3xl animate-fade-in">
            <JobSearchBar />
            <div className="mt-5 flex flex-wrap items-center justify-center gap-2 text-sm">
              <span className="text-mute">Popular:</span>
              {POPULAR.map((term) => (
                <Link
                  key={term}
                  href={`/jobs?q=${encodeURIComponent(term)}`}
                  className="rounded-pill border border-line bg-surface px-3 py-1.5 text-xs font-medium text-body transition-colors duration-200 hover:border-shout hover:text-shout"
                >
                  {term}
                </Link>
              ))}
            </div>
          </div>

          <dl className="mx-auto mt-14 grid max-w-2xl grid-cols-1 gap-px overflow-hidden rounded-card border border-line bg-line sm:grid-cols-3">
            <Stat value={jobs.length} label="Open roles" />
            <Stat value={remoteCount} label="Remote-friendly" />
            <Stat value={categories.length} label="Categories" />
          </dl>
        </div>
      </section>

      {/* ------------------------------------------------------ Browse by area */}
      {categories.length > 0 ? (
        <section className="mx-auto max-w-board px-5 py-16 sm:px-8 sm:py-20">
          <SectionHeading
            eyebrow="Browse"
            title="Pick a lane"
            lead="Every posting is grouped by discipline so you can skip straight to your patch of the board."
          />
          <ul className="mt-9 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            {categories.map(({ name, count }) => (
              <li key={name}>
                <Link
                  href={`/jobs?category=${encodeURIComponent(name)}`}
                  className="surface-card group flex items-center gap-4 p-5 transition-all duration-200 ease-out hover:-translate-y-0.5 hover:border-edge hover:shadow-lift"
                >
                  <span className="flex h-10 w-10 items-center justify-center rounded-xl bg-shout/10 text-shout">
                    <BriefcaseIcon className="h-5 w-5" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-display text-base font-semibold text-ink">
                      {name}
                    </span>
                    <span className="text-sm text-mute">
                      {count} {count === 1 ? "role" : "roles"}
                    </span>
                  </span>
                  <ArrowRightIcon className="h-4 w-4 text-mute transition-transform duration-200 group-hover:translate-x-0.5 group-hover:text-shout" />
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {/* ------------------------------------------------------ Featured roles */}
      <section className="border-y border-line bg-subtle">
        <div className="mx-auto max-w-board px-5 py-16 sm:px-8 sm:py-20">
          <SectionHeading
            eyebrow="Fresh on the board"
            title="Latest open roles"
            lead="Straight from the marketplace API — newest first."
            action={
              <Link href="/jobs" className={buttonClass("secondary", "md")}>
                See all roles
                <ArrowRightIcon className="h-4 w-4" />
              </Link>
            }
          />

          {featured.length > 0 ? (
            <ul className="mt-9 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {featured.map((job) => (
                <li key={job.id}>
                  <JobCard job={job} />
                </li>
              ))}
            </ul>
          ) : (
            <div className="surface-card mt-9 px-6 py-14 text-center">
              <p className="font-display text-lg font-semibold text-ink">
                The board is empty right now
              </p>
              <p className="mx-auto mt-2 max-w-md text-sm leading-relaxed text-mute">
                No published roles came back from the API. If you are running this locally, start
                the API on port 8088 and seed it — or post the first job yourself.
              </p>
              <Link href="/post-job" className={buttonClass("primary", "md", "mt-6")}>
                Post the first job
              </Link>
            </div>
          )}
        </div>
      </section>

      {/* --------------------------------------------------------- How it works */}
      <section className="mx-auto max-w-board px-5 py-16 sm:px-8 sm:py-24">
        <div className="grid gap-14 lg:grid-cols-2 lg:gap-20">
          <div>
            <Eyebrow>For candidates</Eyebrow>
            <h2 className="mt-4 font-display text-3xl font-semibold tracking-[-0.02em] text-ink sm:text-4xl">
              Stop rewriting the same application
            </h2>
            <ol className="mt-8 space-y-6">
              {CANDIDATE_STEPS.map((step, i) => (
                <Step key={step.title} index={i + 1} {...step} />
              ))}
            </ol>
            <Link href="/profile" className={buttonClass("primary", "md", "mt-9")}>
              Build my profile
              <ArrowRightIcon className="h-4 w-4" />
            </Link>
          </div>

          <div>
            <Eyebrow>For employers</Eyebrow>
            <h2 className="mt-4 font-display text-3xl font-semibold tracking-[-0.02em] text-ink sm:text-4xl">
              Reach people who actually match
            </h2>
            <ol className="mt-8 space-y-6">
              {EMPLOYER_STEPS.map((step, i) => (
                <Step key={step.title} index={i + 1} {...step} />
              ))}
            </ol>
            <Link href="/post-job" className={buttonClass("secondary", "md", "mt-9")}>
              Post a job
              <ArrowRightIcon className="h-4 w-4" />
            </Link>
          </div>
        </div>
      </section>

      {/* ---------------------------------------------------------------- Trust */}
      <section className="border-t border-line bg-subtle">
        <div className="mx-auto max-w-board px-5 py-16 sm:px-8 sm:py-20">
          <div className="grid gap-8 md:grid-cols-3">
            <TrustCard
              icon={<ShieldIcon className="h-5 w-5" />}
              title="You approve every move"
              body="No agent sends an application, a message, or an offer without a human saying yes first."
            />
            <TrustCard
              icon={<TargetIcon className="h-5 w-5" />}
              title="Scores you can argue with"
              body="Every match ships with its reasons, so you can tell a good fit from a keyword collision."
            />
            <TrustCard
              icon={<GlobeIcon className="h-5 w-5" />}
              title="Remote treated properly"
              body="Remote is a first-class filter, not a checkbox buried at the bottom of a form."
            />
          </div>
        </div>
      </section>

      {/* ------------------------------------------------------------ Final CTA */}
      <section className="mx-auto max-w-board px-5 py-16 sm:px-8 sm:py-24">
        <div className="relative overflow-hidden rounded-card border border-line bg-surface px-6 py-14 text-center sm:px-14">
          <div aria-hidden className="pointer-events-none absolute inset-0 halo opacity-80" />
          <div className="relative">
            <h2 className="mx-auto max-w-2xl font-display text-3xl font-semibold tracking-[-0.025em] text-ink sm:text-[2.75rem] sm:leading-[1.08]">
              Hiring, or looking? Start on the same board.
            </h2>
            <p className="mx-auto mt-5 max-w-lg text-base leading-relaxed text-mute">
              One marketplace, two sides, and agents that do the reading so people can do the
              deciding.
            </p>
            <div className="mt-9 flex flex-wrap justify-center gap-3">
              <Link href="/jobs" className={buttonClass("primary", "lg")}>
                Browse open roles
              </Link>
              <Link href="/post-job" className={buttonClass("secondary", "lg")}>
                Post a job
              </Link>
            </div>
          </div>
        </div>
      </section>
    </>
  );
}

function Stat({ value, label }: { value: number; label: string }) {
  return (
    <div className="flex items-center justify-center gap-3 bg-surface px-4 py-4 sm:block sm:py-6 sm:text-center">
      <dt className="sr-only">{label}</dt>
      <dd className="flex items-center gap-3 sm:block">
        <span className="font-display text-3xl font-semibold tracking-[-0.02em] text-ink sm:block sm:text-4xl">
          {value}
        </span>
        <span className="text-xs font-medium uppercase tracking-[0.12em] text-mute sm:mt-1 sm:block">
          {label}
        </span>
      </dd>
    </div>
  );
}

function Step({
  index,
  icon,
  title,
  body,
}: {
  index: number;
  icon: React.ReactNode;
  title: string;
  body: string;
}) {
  return (
    <li className="flex gap-5">
      <span className="relative flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-shout">
        {icon}
        <span
          aria-hidden
          className="absolute -right-1.5 -top-1.5 flex h-5 w-5 items-center justify-center rounded-full bg-ink text-[0.65rem] font-bold text-bg"
        >
          {index}
        </span>
      </span>
      <div className="min-w-0 pt-1">
        <h3 className="font-display text-lg font-semibold text-ink">{title}</h3>
        <p className="mt-1.5 text-sm leading-relaxed text-mute">{body}</p>
      </div>
    </li>
  );
}

function TrustCard({
  icon,
  title,
  body,
}: {
  icon: React.ReactNode;
  title: string;
  body: string;
}) {
  return (
    <div>
      <span className="flex h-11 w-11 items-center justify-center rounded-xl border border-line bg-surface text-signal">
        {icon}
      </span>
      <h3 className="mt-4 font-display text-lg font-semibold text-ink">{title}</h3>
      <p className="mt-2 text-sm leading-relaxed text-mute">{body}</p>
    </div>
  );
}

function countCategories(jobs: Job[]): Array<{ name: string; count: number }> {
  const tally = new Map<string, number>();
  for (const job of jobs) {
    const name = jobCategory(job);
    tally.set(name, (tally.get(name) ?? 0) + 1);
  }
  return [...tally.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 8);
}
