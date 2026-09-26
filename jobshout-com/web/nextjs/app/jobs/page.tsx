import Link from "next/link";
import type { Metadata } from "next";
import { JobCard, JobCardSkeleton } from "@/components/JobCard";
import { BoardSort, JobFilters } from "@/components/JobFilters";
import { JobSearchBar } from "@/components/JobSearchBar";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import { BriefcaseIcon, SearchIcon } from "@/components/icons";
import { listJobs, type Job } from "@/lib/api";
import { applyFilters, hasActiveFilters, parseFilters } from "@/lib/filter";
import { employmentLabel } from "@/lib/format";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Open roles",
  description: "Search and filter every open role on the JobShout.com board.",
};

export default async function JobsPage({
  searchParams,
}: {
  searchParams: Record<string, string | string[] | undefined>;
}) {
  const filters = parseFilters(searchParams);

  let jobs: Job[] = [];
  let error = "";
  try {
    jobs = await listJobs(100);
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load jobs";
  }

  const results = applyFilters(jobs, filters);
  const filtered = hasActiveFilters(filters);

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header>
        <h1 className="font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
          Open roles
        </h1>
        <p className="mt-3 max-w-xl text-base leading-relaxed text-mute">
          {jobs.length} live {jobs.length === 1 ? "listing" : "listings"} from the marketplace.
          Filter it down, then apply in a couple of minutes.
        </p>
      </header>

      <div className="mt-8">
        <JobSearchBar
          size="md"
          defaultQuery={filters.q}
          defaultLocation={filters.where}
        />
      </div>

      {/* Active filters, restated as removable chips. */}
      {filtered ? (
        <ul className="mt-4 flex flex-wrap items-center gap-2">
          <li className="text-xs font-semibold uppercase tracking-[0.14em] text-mute">Active</li>
          {filters.q ? <Chip label={`“${filters.q}”`} /> : null}
          {filters.where ? <Chip label={filters.where} /> : null}
          {filters.category ? <Chip label={filters.category} /> : null}
          {filters.remote ? <Chip label="Remote only" /> : null}
          {filters.types.map((t) => (
            <Chip key={t} label={employmentLabel(t)} />
          ))}
          {filters.minSalary !== null ? (
            <Chip label={`£${(filters.minSalary / 1000).toFixed(0)}k+`} />
          ) : null}
          <li>
            <Link
              href="/jobs"
              className="inline-flex min-h-[36px] items-center rounded-pill px-2 text-xs font-medium text-mute underline decoration-line underline-offset-4 transition-colors duration-200 hover:text-shout"
            >
              Clear all
            </Link>
          </li>
        </ul>
      ) : null}

      {error ? (
        <div className="mt-8">
          <ErrorNotice
            title={error}
            body="The board could not reach the marketplace API. Start it on port 8088 and refresh."
          />
        </div>
      ) : null}

      <div className="mt-8 grid gap-8 lg:grid-cols-[16rem_1fr] lg:gap-12">
        <JobFilters filters={filters} resultCount={results.length} />

        <section aria-label="Job results">
          <div className="hidden items-center justify-between gap-4 border-b border-line pb-4 lg:flex">
            <p className="text-sm text-mute">
              <span className="font-semibold text-ink">{results.length}</span>{" "}
              {results.length === 1 ? "role" : "roles"}
              {filtered ? " match your filters" : " on the board"}
            </p>
            <BoardSort value={filters.sort} />
          </div>

          {results.length > 0 ? (
            <ul className="mt-6 grid gap-4 xl:grid-cols-2">
              {results.map((job) => (
                <li key={job.id}>
                  <JobCard job={job} />
                </li>
              ))}
            </ul>
          ) : (
            <div className="mt-6">
              {/* A dead end always offers a way out. */}
              {filtered ? (
                <EmptyState
                  icon={<SearchIcon className="h-6 w-6" />}
                  title="No roles match those filters"
                  body={
                    filters.q
                      ? `Nothing on the board mentions “${filters.q}”. Try a broader keyword, drop the location, or clear the salary floor.`
                      : "Try widening the work type, turning off remote-only, or lowering the salary floor."
                  }
                  action={
                    <div className="flex flex-wrap justify-center gap-3">
                      <Link href="/jobs" className={buttonClass("primary", "md")}>
                        Clear all filters
                      </Link>
                      {filters.q ? (
                        <Link
                          href={`/jobs?q=${encodeURIComponent(filters.q)}`}
                          className={buttonClass("secondary", "md")}
                        >
                          Keep “{filters.q}” only
                        </Link>
                      ) : null}
                    </div>
                  }
                />
              ) : (
                <EmptyState
                  icon={<BriefcaseIcon className="h-6 w-6" />}
                  title="No published roles yet"
                  body="Nothing has been posted to the board. Be the first — publishing takes about two minutes."
                  action={
                    <Link href="/post-job" className={buttonClass("primary", "md")}>
                      Post a job
                    </Link>
                  }
                />
              )}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

function Chip({ label }: { label: string }) {
  return (
    <li>
      <Badge tone="brand">{label}</Badge>
    </li>
  );
}
