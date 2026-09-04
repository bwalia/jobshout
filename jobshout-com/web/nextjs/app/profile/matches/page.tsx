import Link from "next/link";
import {
  formatCompensation,
  formatLocation,
  listProfileMatches,
  type JobMatch,
} from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function ProfileMatchesPage({
  searchParams,
}: {
  searchParams: { id?: string };
}) {
  const id = searchParams.id ?? "";
  let matches: JobMatch[] = [];
  let error = "";

  if (!id) {
    error = "Missing profile id. Save your profile first.";
  } else {
    try {
      matches = await listProfileMatches(id);
    } catch (e) {
      error = e instanceof Error ? e.message : "Could not load matches";
    }
  }

  return (
    <div className="mx-auto max-w-5xl px-6 py-12">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.18em] text-accent">
            Agent matches
          </p>
          <h1 className="mt-2 font-display text-4xl tracking-tight">Jobs ranked for you</h1>
          <p className="mt-2 max-w-2xl text-slate">
            Scores are explainable so a Career agent (or you) can see why each role fits.
          </p>
        </div>
        <Link
          href="/profile"
          className="rounded-full border border-slate/25 bg-white px-4 py-2 text-sm font-semibold text-ink hover:border-slate/50"
        >
          Edit profile
        </Link>
      </div>

      {error && (
        <p className="mt-8 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
          {error}
        </p>
      )}

      <ul className="mt-10 space-y-4">
        {matches.map((m) => (
          <li
            key={m.job.id}
            className="rounded-2xl bg-white p-6 shadow-sm ring-1 ring-slate/10"
          >
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <Link
                  href={`/jobs/${m.job.id}`}
                  className="text-xl font-semibold tracking-tight hover:text-accent"
                >
                  {m.job.title}
                </Link>
                <p className="mt-1 text-sm text-slate">{formatLocation(m.job.location)}</p>
              </div>
              <div className="text-right">
                <p className="font-display text-3xl text-accent">{m.score}</p>
                <p className="text-xs font-medium uppercase tracking-wide text-slate">
                  match score
                </p>
              </div>
            </div>
            <p className="mt-3 text-slate">{m.job.summary || m.job.description}</p>
            <p className="mt-2 text-sm font-medium text-ink">
              {formatCompensation(m.job.compensation)}
            </p>
            <ul className="mt-4 space-y-1.5">
              {m.reasons.map((r) => (
                <li key={r} className="text-sm text-slate">
                  · {r}
                </li>
              ))}
            </ul>
          </li>
        ))}
        {!error && matches.length === 0 && (
          <li className="rounded-2xl bg-white p-8 text-center text-slate ring-1 ring-slate/10">
            No strong matches yet. Add more skills or preferred roles on your profile.
          </li>
        )}
      </ul>
    </div>
  );
}
