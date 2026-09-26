import Link from "next/link";

export function Logo({ className }: { className?: string }) {
  return (
    <Link
      href="/"
      aria-label="JobShout.com home"
      className={`group inline-flex items-center gap-2.5 ${className ?? ""}`}
    >
      <span
        aria-hidden
        className="relative flex h-9 w-9 items-center justify-center rounded-[11px] bg-shout"
      >
        {/* Megaphone mark — the "shout". */}
        <svg viewBox="0 0 24 24" className="h-[19px] w-[19px] text-[rgb(var(--on-brand))]">
          <path
            fill="currentColor"
            d="M17.5 4.2a1 1 0 0 1 1.5.87v13.86a1 1 0 0 1-1.5.87L10 15.6V8.4l7.5-4.2Z"
          />
          <path
            fill="currentColor"
            d="M4.6 8.6h4.1v6.8H4.6a1.6 1.6 0 0 1-1.6-1.6v-3.6a1.6 1.6 0 0 1 1.6-1.6Z"
          />
          <path
            fill="currentColor"
            d="M6.9 16.6h2.4l1 4a1 1 0 0 1-.97 1.24H8.9a1 1 0 0 1-.97-.76l-1.03-4.48Z"
          />
        </svg>
      </span>
      <span className="font-display text-[1.3rem] font-semibold leading-none tracking-[-0.03em] text-ink">
        JobShout
        <span className="text-mute transition-colors duration-200 group-hover:text-shout">
          .com
        </span>
      </span>
    </Link>
  );
}
