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
    <div className="mx-auto max-w-board px-6 pb-20 pt-12">
      <div className="flex flex-wrap items-end justify-between gap-6 border-b border-line pb-8">
        <div className="max-w-2xl">
          <h1 className="font-display text-4xl tracking-tight text-ink md:text-5xl">
            Ranked for you
          </h1>
          <p className="mt-3 text-mute">
            Explainable scores so you — or a Career agent — can see why each role fits.
          </p>
        </div>
        <Link
          href="/profile"
          className="border border-ink/20 bg-white/60 px-4 py-2 text-sm font-semibold text-ink transition hover:border-signal hover:text-signal"
        >
          Edit profile
        </Link>
      </div>

      {error && (
        <p className="mt-8 border border-shout/30 bg-shout/5 px-4 py-3 text-sm text-ink">{error}</p>
      )}

      <ul className="mt-2">
        {matches.map((m) => (
          <li key={m.job.id}>
            <div className="job-row pl-4">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="min-w-0 flex-1">
                  <Link
                    href={`/jobs/${m.job.id}`}
                    className="font-display text-2xl tracking-tight text-ink transition hover:text-signal"
                  >
                    {m.job.title}
                  </Link>
                  <p className="mt-2 text-sm text-mute">
                    {formatLocation(m.job.location)}
                    <span className="mx-2 text-line">/</span>
                    {formatCompensation(m.job.compensation)}
                  </p>
                  <p className="mt-3 max-w-3xl text-[0.95rem] leading-relaxed text-mute">
                    {m.job.summary || m.job.description}
                  </p>
                  <ul className="mt-4 space-y-1.5">
                    {m.reasons.map((r) => (
                      <li key={r} className="flex gap-3 text-sm text-mute">
                        <span className="mt-2 h-1.5 w-1.5 shrink-0 bg-signal" aria-hidden />
                        <span>{r}</span>
                      </li>
                    ))}
                  </ul>
                </div>
                <div className="text-right">
                  <p className="font-display text-4xl leading-none text-signal">{m.score}</p>
                  <p className="mt-1 text-xs text-mute">match</p>
                </div>
              </div>
            </div>
          </li>
        ))}
        {!error && matches.length === 0 && (
          <li className="py-16 text-center text-mute">
            No strong matches yet. Add more skills or preferred roles on your profile.
          </li>
        )}
      </ul>
    </div>
  );
}
