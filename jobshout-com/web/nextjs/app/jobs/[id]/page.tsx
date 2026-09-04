import Link from "next/link";
import { notFound } from "next/navigation";
import { formatCompensation, formatLocation, getJob } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function JobDetailPage({ params }: { params: { id: string } }) {
  const job = await getJob(params.id);
  if (!job) notFound();

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <Link href="/jobs" className="text-sm font-medium text-slate hover:text-ink">
        ← All jobs
      </Link>
      <p className="mt-8 text-sm font-semibold uppercase tracking-[0.18em] text-accent">
        {job.employment_type.replace("_", " ")}
      </p>
      <h1 className="mt-2 font-display text-4xl tracking-tight md:text-5xl">{job.title}</h1>
      <p className="mt-3 text-slate">
        {formatLocation(job.location)} · {formatCompensation(job.compensation)}
      </p>
      {job.summary && <p className="mt-6 text-lg leading-relaxed text-ink">{job.summary}</p>}
      <article className="prose prose-slate mt-10 max-w-none whitespace-pre-wrap leading-relaxed">
        {job.description}
      </article>
      {job.requirements.length > 0 && (
        <section className="mt-12">
          <h2 className="font-display text-2xl">Requirements</h2>
          <ul className="mt-4 list-disc space-y-1 pl-5 text-slate">
            {job.requirements.map((r) => (
              <li key={r}>{r}</li>
            ))}
          </ul>
        </section>
      )}
      <div className="mt-12 rounded-2xl bg-white p-6 ring-1 ring-slate/10">
        <p className="text-sm text-slate">
          Applications and Career Agent apply flow land in a later phase. This MVP is the public job
          board + Rust jobs API.
        </p>
      </div>
    </div>
  );
}
