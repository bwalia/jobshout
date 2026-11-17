import Link from "next/link";
import { HeroSearch } from "@/components/HeroSearch";
import { JobCard } from "@/components/JobCard";
import { SectionHeading, buttonClass } from "@/components/ui";
import {
  ArrowRightIcon,
  BotIcon,
  BriefcaseIcon,
  FileTextIcon,
  SparkIcon,
  UsersIcon,
} from "@/components/icons";
import { listJobs, type Job } from "@/lib/api";

export const dynamic = "force-dynamic";

const POPULAR: Array<{ label: string; href: string }> = [
  { label: "Rust jobs", href: "/jobs?q=Rust" },
  { label: "Remote", href: "/jobs?where=Remote" },
  { label: "AI agents", href: "/agents?q=AI" },
  { label: "Hiring talent", href: "/members?q=engineer" },
  { label: "Groups", href: "/groups" },
  { label: "Trends", href: "/insights" },
];

const DISCOVER = [
  {
    href: "/jobs",
    icon: <BriefcaseIcon className="h-5 w-5" />,
    title: "Jobs",
    body: "Open roles with filters recruiters and candidates already use.",
  },
  {
    href: "/agents",
    icon: <BotIcon className="h-5 w-5" />,
    title: "AI agents",
    body: "Browse showcase agents and teams — what they run and what they ship.",
  },
  {
    href: "/members",
    icon: <UsersIcon className="h-5 w-5" />,
    title: "Talents",
    body: "Find people by headline and skills when you are hiring or networking.",
  },
  {
    href: "/insights",
    icon: <FileTextIcon className="h-5 w-5" />,
    title: "Insights",
    body: "Latest trends, market shifts, and practical analysis.",
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

  return (
    <>
      {/* ---------------------------------------------------------------- Hero */}
      <section className="relative overflow-hidden border-b border-line">
        <div aria-hidden className="pointer-events-none absolute inset-0 halo" />
        <div aria-hidden className="pointer-events-none absolute inset-0 grid-field" />

        <div className="relative mx-auto max-w-board px-5 pb-16 pt-16 sm:px-8 sm:pb-24 sm:pt-24">
          <div className="mx-auto max-w-3xl text-center">
            <p className="animate-fade-in font-display text-sm font-semibold uppercase tracking-[0.22em] text-shout">
              JobShout
            </p>

            <h1 className="mt-5 animate-rise-in font-display text-[clamp(2.5rem,7vw,4.5rem)] font-semibold leading-[1.02] tracking-[-0.035em] text-ink">
              Search work, talent,
              <br className="hidden sm:block" />{" "}
              <span className="relative inline-block">
                agents &amp; trends
                <span
                  aria-hidden
                  className="absolute -bottom-1 left-0 h-[0.18em] w-full rounded-full bg-shout/70"
                />
              </span>
            </h1>

            <p className="mx-auto mt-6 max-w-xl text-base leading-relaxed text-mute sm:text-lg">
              A simple place to find AI showcase agents, jobs, talents, group posts, and the latest
              market insights — without a cluttered dashboard.
            </p>
          </div>

          <div className="mx-auto mt-10 max-w-3xl animate-fade-in">
            <HeroSearch />
            <div className="mt-5 flex flex-wrap items-center justify-center gap-2 text-sm">
              <span className="text-mute">Try:</span>
              {POPULAR.map((term) => (
                <Link
                  key={term.href}
                  href={term.href}
                  className="rounded-pill border border-line bg-surface px-3 py-1.5 text-xs font-medium text-body transition-colors duration-200 hover:border-shout hover:text-shout"
                >
                  {term.label}
                </Link>
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* ------------------------------------------------------ Discover lanes */}
      <section className="mx-auto max-w-board px-5 py-16 sm:px-8 sm:py-20">
        <SectionHeading
          eyebrow="Browse"
          title="Everything in one search"
          lead="Pick a lane, or stay on Home and switch the search scope above."
        />
        <ul className="mt-9 grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
          {DISCOVER.map((item) => (
            <li key={item.href}>
              <Link href={item.href} className="group block">
                <span className="flex h-10 w-10 items-center justify-center rounded-xl bg-shout/10 text-shout transition-transform duration-200 group-hover:scale-105">
                  {item.icon}
                </span>
                <span className="mt-4 block font-display text-lg font-semibold text-ink">
                  {item.title}
                </span>
                <span className="mt-1.5 block text-sm leading-relaxed text-mute">{item.body}</span>
                <span className="mt-3 inline-flex items-center gap-1 text-sm font-medium text-shout">
                  Open
                  <ArrowRightIcon className="h-3.5 w-3.5 transition-transform duration-200 group-hover:translate-x-0.5" />
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </section>

      {/* ------------------------------------------------------ Featured roles */}
      <section className="border-y border-line bg-subtle">
        <div className="mx-auto max-w-board px-5 py-16 sm:px-8 sm:py-20">
          <SectionHeading
            eyebrow="Fresh on the board"
            title="Latest open roles"
            lead="Straight from the marketplace — newest first."
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
            <div className="mt-9 px-6 py-14 text-center">
              <p className="font-display text-lg font-semibold text-ink">
                The board is empty right now
              </p>
              <p className="mx-auto mt-2 max-w-md text-sm leading-relaxed text-mute">
                No published roles came back from the API. Post the first job, or search agents and
                talents instead.
              </p>
              <div className="mt-6 flex flex-wrap justify-center gap-3">
                <Link href="/post-job" className={buttonClass("primary", "md")}>
                  Post a job
                </Link>
                <Link href="/agents" className={buttonClass("secondary", "md")}>
                  Browse agents
                </Link>
              </div>
            </div>
          )}
        </div>
      </section>

      {/* ------------------------------------------------------------ Final CTA */}
      <section className="mx-auto max-w-board px-5 py-16 sm:px-8 sm:py-24">
        <div className="relative overflow-hidden border border-line bg-surface px-6 py-14 text-center sm:px-14">
          <div aria-hidden className="pointer-events-none absolute inset-0 halo opacity-80" />
          <div className="relative">
            <p className="font-display text-sm font-semibold uppercase tracking-[0.2em] text-shout">
              JobShout
            </p>
            <h2 className="mx-auto mt-4 max-w-2xl font-display text-3xl font-semibold tracking-[-0.025em] text-ink sm:text-[2.5rem] sm:leading-[1.08]">
              Hiring, looking, or exploring AI agents?
            </h2>
            <p className="mx-auto mt-5 max-w-lg text-base leading-relaxed text-mute">
              Start with search. Switch scope for jobs, talents, groups, agents, or insights.
            </p>
            <div className="mt-9 flex flex-wrap justify-center gap-3">
              <Link href="/jobs" className={buttonClass("primary", "lg")}>
                <SparkIcon className="h-4 w-4" />
                Browse jobs
              </Link>
              <Link href="/groups" className={buttonClass("secondary", "lg")}>
                Explore groups
              </Link>
            </div>
          </div>
        </div>
      </section>
    </>
  );
}
