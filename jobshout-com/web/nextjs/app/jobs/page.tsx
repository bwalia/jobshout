import Link from "next/link";
import { formatCompensation, formatLocation, listJobs, type Job } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function JobsPage() {
  let jobs: Job[] = [];
  let error = "";
  try {
    jobs = await listJobs();
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load jobs";
  }

  return (
    <div className="mx-auto max-w-5xl px-6 py-12">
      <div className="flex items-end justify-between gap-4">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.18em] text-accent">Board</p>
          <h1 className="mt-2 font-display text-4xl tracking-tight">Open roles</h1>
          <p className="mt-2 text-slate">Published jobs from the JobShout.com marketplace API.</p>
        </div>
        <Link href="/" className="text-sm font-medium text-slate hover:text-ink">
          ← Home
        </Link>
      </div>

      {error && (
        <p className="mt-8 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
          {error}. Is the API running on :8088?
        </p>
      )}

      <ul className="mt-10 space-y-4">
        {jobs.map((job) => (
          <li key={job.id}>
            <Link
              href={`/jobs/${job.id}`}
              className="block rounded-2xl bg-white p-6 shadow-sm ring-1 ring-slate/10 transition hover:ring-accent/40"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h2 className="text-xl font-semibold tracking-tight">{job.title}</h2>
                  <p className="mt-1 text-sm text-slate">{formatLocation(job.location)}</p>
                </div>
                <p className="text-sm font-medium text-ink">{formatCompensation(job.compensation)}</p>
              </div>
              <p className="mt-3 line-clamp-2 text-slate">{job.summary || job.description}</p>
              <div className="mt-4 flex flex-wrap gap-2">
                <span className="rounded-full bg-mist px-2.5 py-1 text-xs font-medium uppercase tracking-wide text-slate">
                  {job.employment_type.replace("_", " ")}
                </span>
                {job.requirements.slice(0, 4).map((r) => (
                  <span
                    key={r}
                    className="rounded-full bg-mist px-2.5 py-1 text-xs font-medium text-slate"
                  >
                    {r}
                  </span>
                ))}
              </div>
            </Link>
          </li>
        ))}
        {!error && jobs.length === 0 && (
          <li className="rounded-2xl bg-white p-8 text-center text-slate ring-1 ring-slate/10">
            No published jobs yet. Run <code className="text-ink">make seed</code>.
          </li>
        )}
      </ul>
    </div>
  );
}
