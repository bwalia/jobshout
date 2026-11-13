import Link from "next/link";
import type { Metadata } from "next";
import { buttonClass, EmptyState } from "@/components/ui";
import { SearchIcon, SlidersIcon, BriefcaseIcon, WalletIcon, ClockIcon } from "@/components/icons";
import {
  formatProjectBudget,
  listProjects,
  projectTypeLabel,
  type Project,
} from "@/lib/projects";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Projects",
  description: "Browse freelance projects and submit proposals on JobShout.",
  alternates: { canonical: "/projects" },
};

export default async function ProjectsPage() {
  let projects: Project[] = [];
  let dark = false;
  let error = "";
  try {
    projects = await listProjects();
  } catch (e) {
    const msg = e instanceof Error ? e.message : "Could not load projects";
    if (msg.includes("(404)")) dark = true;
    else error = msg;
  }

  return (
    <div className="mx-auto max-w-board px-5 py-8 sm:px-8">
      <div className="mb-10 text-center">
        <h1 className="font-display text-[clamp(2.5rem,6vw,3.5rem)] font-semibold leading-[1.02] tracking-[-0.035em] text-ink">
          Find freelance projects worth{" "}
          <span className="relative inline-block">
            shouting about
            <span
              aria-hidden
              className="absolute -bottom-1 left-0 h-[0.18em] w-full rounded-full bg-shout/70"
            />
          </span>
        </h1>
        <p className="mx-auto mt-5 max-w-xl text-base leading-relaxed text-mute sm:text-lg">
          Browse short-term projects, submit your bids, and work with clients on flexible terms.
        </p>
      </div>

      <div className="mb-8 flex flex-wrap items-center gap-4">
        <div className="relative min-w-[12rem] flex-1">
          <SearchIcon className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-mute" />
          <input
            type="search"
            placeholder="Search projects (React, design, writing, etc.)"
            className="w-full rounded-pill border border-line bg-bg py-2.5 pl-10 pr-4 text-ink placeholder:text-mute"
            disabled
            aria-disabled
          />
        </div>
        <button type="button" className={buttonClass("secondary", "md")} disabled>
          <SlidersIcon className="h-4 w-4" />
          Filters
        </button>
        <Link href="/projects/post" className={buttonClass("primary", "md")}>
          <BriefcaseIcon className="h-4 w-4" />
          Post Project
        </Link>
      </div>

      {dark ? (
        <EmptyState
          icon={<BriefcaseIcon className="h-6 w-6" />}
          title="Projects marketplace is dark on this ring"
          body="Set PROJECTS_LIVE to list and post real projects. Until then, the board stays empty."
        />
      ) : null}
      {error ? <p className="text-mute">{error}</p> : null}

      {!dark && !error && projects.length === 0 ? (
        <EmptyState
          icon={<BriefcaseIcon className="h-6 w-6" />}
          title="No open projects yet"
          body="Be the first to post a project, or ask an admin to enable sample seeding on int."
          action={
            <Link href="/projects/post" className={buttonClass("primary", "md")}>
              Post a project
            </Link>
          }
        />
      ) : null}

      <div className="space-y-6">
        {projects.map((project) => (
          <article key={project.id} className="rounded-card border border-line bg-surface p-6">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <h2 className="font-display text-xl font-semibold text-ink">
                  <Link href={`/projects/${project.id}`} className="hover:text-shout">
                    {project.title || "Untitled project"}
                  </Link>
                </h2>
                <p className="mt-2 text-body">{project.summary || project.description.slice(0, 180)}</p>
                <div className="mt-4 flex flex-wrap items-center gap-3 text-sm">
                  <span className="inline-flex items-center gap-1.5 rounded-pill bg-raised px-2.5 py-1 font-medium text-ink">
                    <WalletIcon className="h-3 w-3" />
                    {formatProjectBudget(project)}
                  </span>
                  <span className="inline-flex items-center gap-1.5 text-mute">
                    <ClockIcon className="h-3 w-3" />
                    {project.duration_estimate || "Flexible"}
                  </span>
                  <span className="text-mute">{projectTypeLabel(project.project_type)}</span>
                  <span className="text-mute">{project.bid_count ?? 0} bids</span>
                </div>
                {project.skills_required?.length ? (
                  <ul className="mt-4 flex flex-wrap gap-2">
                    {project.skills_required.map((s) => (
                      <li key={s} className="rounded-pill border border-line px-2.5 py-1 text-xs text-mute">
                        {s}
                      </li>
                    ))}
                  </ul>
                ) : null}
              </div>
            </div>
          </article>
        ))}
      </div>
    </div>
  );
}
