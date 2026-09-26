import Link from "next/link";
import type { ComponentProps, ReactNode } from "react";

export function cx(...parts: Array<string | false | null | undefined>) {
  return parts.filter(Boolean).join(" ");
}

/* -------------------------------------------------------------------------- */
/* Button                                                                     */
/* -------------------------------------------------------------------------- */

type Variant = "primary" | "secondary" | "ghost" | "danger";
type Size = "sm" | "md" | "lg";

const VARIANTS: Record<Variant, string> = {
  // Ink-on-orange, not white-on-orange: white fails WCAG AA on this brand hue.
  primary:
    "bg-shout text-[rgb(var(--on-brand))] hover:bg-shout/90 active:bg-shout/80 font-semibold",
  secondary:
    "border border-edge bg-surface text-ink hover:border-ink/40 hover:bg-raised font-semibold",
  ghost: "text-body hover:bg-raised hover:text-ink font-medium",
  danger: "bg-ink text-bg hover:bg-ink/85 font-semibold",
};

const SIZES: Record<Size, string> = {
  // All sizes clear the 44px touch target at md and above; sm is for dense rows.
  sm: "h-9 px-3.5 text-sm gap-1.5",
  md: "h-11 px-5 text-sm gap-2",
  lg: "h-[3.25rem] px-7 text-base gap-2.5",
};

const BUTTON_BASE =
  "inline-flex cursor-pointer items-center justify-center rounded-pill whitespace-nowrap transition-colors duration-200 disabled:cursor-not-allowed disabled:opacity-50";

export function buttonClass(variant: Variant = "primary", size: Size = "md", extra?: string) {
  return cx(BUTTON_BASE, VARIANTS[variant], SIZES[size], extra);
}

export function Button({
  variant = "primary",
  size = "md",
  className,
  ...props
}: ComponentProps<"button"> & { variant?: Variant; size?: Size }) {
  return <button className={buttonClass(variant, size, className)} {...props} />;
}

export function ButtonLink({
  variant = "primary",
  size = "md",
  className,
  ...props
}: ComponentProps<typeof Link> & { variant?: Variant; size?: Size }) {
  return <Link className={buttonClass(variant, size, className)} {...props} />;
}

/* -------------------------------------------------------------------------- */
/* Badges and chips                                                           */
/* -------------------------------------------------------------------------- */

const TONES = {
  neutral: "border-line bg-raised text-body",
  brand: "border-shout/30 bg-shout/10 text-ink",
  signal: "border-signal/30 bg-signal/10 text-signal",
  good: "border-good/30 bg-good/10 text-good",
  warn: "border-warn/30 bg-warn/10 text-warn",
  solid: "border-transparent bg-ink text-bg",
} as const;

export function Badge({
  tone = "neutral",
  className,
  children,
}: {
  tone?: keyof typeof TONES;
  className?: string;
  children: ReactNode;
}) {
  return (
    <span
      className={cx(
        "inline-flex items-center gap-1.5 rounded-pill border px-2.5 py-1 text-xs font-medium leading-none",
        TONES[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}

/* -------------------------------------------------------------------------- */
/* Section furniture                                                          */
/* -------------------------------------------------------------------------- */

export function Eyebrow({ children }: { children: ReactNode }) {
  return (
    <p className="flex items-center gap-2.5 text-xs font-semibold uppercase tracking-[0.18em] text-mute">
      <span aria-hidden className="h-px w-6 bg-shout" />
      {children}
    </p>
  );
}

export function SectionHeading({
  eyebrow,
  title,
  lead,
  action,
}: {
  eyebrow?: string;
  title: string;
  lead?: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-x-8 gap-y-5">
      <div className="max-w-2xl">
        {eyebrow ? <Eyebrow>{eyebrow}</Eyebrow> : null}
        <h2 className="mt-3 font-display text-3xl font-semibold tracking-[-0.02em] text-ink sm:text-[2.5rem] sm:leading-[1.08]">
          {title}
        </h2>
        {lead ? <p className="mt-4 text-base leading-relaxed text-mute">{lead}</p> : null}
      </div>
      {action}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Form fields                                                                */
/* -------------------------------------------------------------------------- */

const CONTROL =
  "w-full rounded-xl border border-line bg-surface px-3.5 py-3 text-[0.95rem] text-ink placeholder:text-mute/70 transition-colors duration-200 hover:border-edge focus:border-shout focus:outline-none focus:ring-2 focus:ring-shout/25 disabled:opacity-60";

export function Field({
  label,
  hint,
  error,
  required,
  htmlFor,
  children,
  className,
}: {
  label: string;
  hint?: string;
  error?: string;
  required?: boolean;
  htmlFor: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cx("min-w-0", className)}>
      <label htmlFor={htmlFor} className="block text-sm font-semibold text-ink">
        {label}
        {required ? (
          <span className="ml-1 text-shout" aria-hidden>
            *
          </span>
        ) : (
          <span className="ml-2 text-xs font-normal text-mute">optional</span>
        )}
      </label>
      {hint ? (
        <p id={`${htmlFor}-hint`} className="mt-1 text-xs leading-relaxed text-mute">
          {hint}
        </p>
      ) : null}
      <div className="mt-2">{children}</div>
      {/* Errors sit next to the field they belong to, never only at the top. */}
      {error ? (
        <p id={`${htmlFor}-error`} role="alert" className="mt-1.5 text-xs font-medium text-shout">
          {error}
        </p>
      ) : null}
    </div>
  );
}

export function Input({ className, ...props }: ComponentProps<"input">) {
  return <input className={cx(CONTROL, className)} {...props} />;
}

export function Textarea({ className, ...props }: ComponentProps<"textarea">) {
  return <textarea className={cx(CONTROL, "resize-y leading-relaxed", className)} {...props} />;
}

export function Select({ className, ...props }: ComponentProps<"select">) {
  return <select className={cx(CONTROL, "cursor-pointer pr-9", className)} {...props} />;
}

export function Checkbox({
  label,
  className,
  ...props
}: ComponentProps<"input"> & { label: string }) {
  return (
    <label className="flex min-h-[44px] cursor-pointer items-center gap-3 py-1.5 text-sm text-body">
      <input
        type="checkbox"
        className={cx(
          "h-5 w-5 cursor-pointer rounded-md border-line text-shout accent-[rgb(var(--brand))]",
          className,
        )}
        {...props}
      />
      {label}
    </label>
  );
}

/* -------------------------------------------------------------------------- */
/* States                                                                     */
/* -------------------------------------------------------------------------- */

export function EmptyState({
  icon,
  title,
  body,
  action,
}: {
  icon: ReactNode;
  title: string;
  body: string;
  action?: ReactNode;
}) {
  return (
    <div className="surface-card flex flex-col items-center px-6 py-16 text-center">
      <span className="flex h-14 w-14 items-center justify-center rounded-2xl border border-line bg-raised text-mute">
        {icon}
      </span>
      <h3 className="mt-5 font-display text-xl font-semibold text-ink">{title}</h3>
      <p className="mt-2 max-w-md break-words text-sm leading-relaxed text-mute">{body}</p>
      {action ? <div className="mt-6">{action}</div> : null}
    </div>
  );
}

export function ErrorNotice({ title, body }: { title: string; body?: string }) {
  return (
    <div
      role="alert"
      className="flex gap-3 rounded-card border border-shout/35 bg-shout/[0.07] px-4 py-3.5"
    >
      <span aria-hidden className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-shout" />
      <div className="min-w-0">
        <p className="text-sm font-semibold text-ink">{title}</p>
        {body ? <p className="mt-1 text-sm leading-relaxed text-mute">{body}</p> : null}
      </div>
    </div>
  );
}
