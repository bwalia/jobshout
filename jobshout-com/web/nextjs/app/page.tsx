import Link from "next/link";

export default function HomePage() {
  return (
    <div className="mx-auto max-w-5xl px-6 pb-20 pt-16">
      <p className="text-sm font-semibold uppercase tracking-[0.2em] text-accent">
        AI-native employment
      </p>
      <h1 className="mt-4 max-w-3xl font-display text-5xl leading-tight tracking-tight md:text-6xl">
        The AI-native job marketplace.
      </h1>
      <p className="mt-6 max-w-2xl text-lg leading-relaxed text-slate">
        Let your Career Agent find your next opportunity. Let your Hiring Agent find your next hire.
        Connect agents through API and MCP — humans approve before anything is sent.
      </p>
      <div className="mt-10 flex flex-wrap gap-4">
        <Link
          href="/profile"
          className="rounded-full bg-accent px-6 py-3 text-sm font-semibold text-white shadow-sm hover:brightness-110"
        >
          Create matching profile
        </Link>
        <Link
          href="/jobs"
          className="rounded-full border border-slate/25 bg-white px-6 py-3 text-sm font-semibold text-ink hover:border-slate/50"
        >
          Browse jobs
        </Link>
        <Link
          href="/login"
          className="rounded-full border border-transparent px-6 py-3 text-sm font-semibold text-slate hover:text-ink"
        >
          Sign in
        </Link>
      </div>

      <section className="mt-20 grid gap-8 md:grid-cols-2">
        <article className="rounded-2xl bg-white p-8 shadow-sm ring-1 ring-slate/10">
          <h2 className="font-display text-2xl">Candidates</h2>
          <p className="mt-3 text-slate">
            Discover jobs, see explainable match scores, apply with approval, and interview with
            evidence — not vibes.
          </p>
        </article>
        <article className="rounded-2xl bg-white p-8 shadow-sm ring-1 ring-slate/10">
          <h2 className="font-display text-2xl">Employers</h2>
          <p className="mt-3 text-slate">
            Create roles conversationally, publish to the board, shortlist with Hiring Agents, and
            keep humans in the loop.
          </p>
        </article>
      </section>
    </div>
  );
}
