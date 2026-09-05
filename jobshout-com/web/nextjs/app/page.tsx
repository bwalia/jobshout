import Link from "next/link";

export default function HomePage() {
  return (
    <div className="signal-field relative">
      <section className="relative mx-auto flex min-h-[calc(100vh-4.5rem)] max-w-board flex-col justify-center px-6 pb-24 pt-10">
        <div className="animate-hero-in max-w-3xl">
          <h1 className="font-display text-[clamp(3.25rem,9vw,6.75rem)] font-semibold leading-[0.92] tracking-[-0.03em] text-ink">
            JobShout
            <span className="text-signal">.com</span>
          </h1>
          <p className="mt-8 max-w-xl text-lg leading-relaxed text-mute sm:text-xl text-balance">
            Your Career Agent finds the role. Your Hiring Agent finds the person. You approve before
            anything is sent.
          </p>
          <div className="mt-10 flex flex-wrap items-center gap-4">
            <Link
              href="/profile"
              className="bg-shout px-6 py-3 text-sm font-semibold text-white transition hover:brightness-110"
            >
              Build matching profile
            </Link>
            <Link
              href="/jobs"
              className="border border-ink/20 bg-white/50 px-6 py-3 text-sm font-semibold text-ink transition hover:border-signal hover:text-signal"
            >
              Browse open roles
            </Link>
          </div>
        </div>

        <div
          aria-hidden
          className="pointer-events-none absolute bottom-10 right-6 hidden h-40 w-[min(42vw,28rem)] md:block"
        >
          <svg viewBox="0 0 400 160" className="h-full w-full text-signal/70" fill="none">
            <path
              d="M0 90 C40 90 40 30 80 30 S120 150 160 150 S200 40 240 40 S280 120 320 120 S360 70 400 70"
              stroke="currentColor"
              strokeWidth="2"
              className="animate-signal-pulse origin-center"
            />
            <path
              d="M0 100 C50 100 50 60 100 60 S150 130 200 130 S250 55 300 55 S350 110 400 110"
              stroke="currentColor"
              strokeWidth="1.25"
              opacity="0.35"
            />
          </svg>
        </div>
      </section>

      <section className="border-t border-line/80 bg-white/40">
        <div className="mx-auto grid max-w-board gap-12 px-6 py-16 md:grid-cols-2 md:gap-20">
          <div>
            <h2 className="font-display text-3xl tracking-tight text-ink">For candidates</h2>
            <p className="mt-4 max-w-md leading-relaxed text-mute">
              Save skills once. See explainable match scores. Apply only when you say yes.
            </p>
          </div>
          <div>
            <h2 className="font-display text-3xl tracking-tight text-ink">For employers</h2>
            <p className="mt-4 max-w-md leading-relaxed text-mute">
              Publish roles to the board. Let Hiring Agents shortlist. Keep humans in the loop.
            </p>
          </div>
        </div>
      </section>
    </div>
  );
}
