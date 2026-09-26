import Link from "next/link";
import { buttonClass } from "@/components/ui";

export default function NotFound() {
  return (
    <div className="relative overflow-hidden">
      <div aria-hidden className="pointer-events-none absolute inset-0 halo" />
      <div className="relative mx-auto flex min-h-[60vh] max-w-lg flex-col items-center justify-center px-5 py-24 text-center sm:px-8">
        <p className="font-display text-7xl font-semibold tracking-[-0.04em] text-shout">404</p>
        <h1 className="mt-4 font-display text-3xl font-semibold tracking-[-0.025em] text-ink">
          That page has closed
        </h1>
        <p className="mt-3 text-base leading-relaxed text-mute">
          The role may have been filled or the link mistyped. The board is still open.
        </p>
        <div className="mt-8 flex flex-wrap justify-center gap-3">
          <Link href="/jobs" className={buttonClass("primary", "md")}>
            Browse open roles
          </Link>
          <Link href="/" className={buttonClass("secondary", "md")}>
            Go home
          </Link>
        </div>
      </div>
    </div>
  );
}
