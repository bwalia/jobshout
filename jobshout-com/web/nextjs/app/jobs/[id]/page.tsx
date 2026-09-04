import Link from "next/link";
import { notFound } from "next/navigation";
import { formatCompensation, formatLocation, getJob } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function JobDetailPage({ params }: { params: { id: string } }) {
  const job = await getJob(params.id);
  if (!job) notFound();

  return (
    <div className="mx-auto max-w-3xl px-6 pb-24 pt-10">
      <Link href="/jobs" className="text-sm text-mute transition hover:text-signal">
        All open roles
      </Link>

      <header className="mt-10 border-b border-line pb-10">
        <p className="text-sm text-mute">{job.employment_type.replaceAll("_", " ")}</p>
        <h1 className="mt-3 font-display text-4xl tracking-tight text-ink md:text-5xl">
          {job.title}
        </h1>
        <p className="mt-4 text-mute">
          {formatLocation(job.location)}
          <span className="mx-2 text-line">/</span>
          {formatCompensation(job.compensation)}
        </p>
        {job.summary ? (
          <p className="mt-8 text-lg leading-relaxed text-ink">{job.summary}</p>
        ) : null}
      </header>

      <article className="mt-10 whitespace-pre-wrap text-[1.05rem] leading-[1.75] text-ink/90">
        {job.description}
      </article>

      {job.requirements.length > 0 && (
        <section className="mt-14 border-t border-line pt-10">
          <h2 className="font-display text-2xl tracking-tight">Requirements</h2>
          <ul className="mt-5 space-y-2 text-mute">
            {job.requirements.map((r) => (
              <li key={r} className="flex gap-3">
                <span className="mt-2 h-1.5 w-1.5 shrink-0 bg-signal" aria-hidden />
                <span>{r}</span>
              </li>
            ))}
          </ul>
        </section>
      )}

      <div className="mt-14 flex flex-wrap items-center gap-4 border-t border-line pt-10">
        <Link
          href="/profile"
          className="bg-shout px-5 py-2.5 text-sm font-semibold text-white transition hover:brightness-110"
        >
          Match this role to my profile
        </Link>
        <p className="text-sm text-mute">Applications with agent approval land in a later phase.</p>
      </div>
    </div>
  );
}
