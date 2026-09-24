import { cx } from "@/components/ui";

export function ShareLinks({ url, title }: { url: string; title: string }) {
  const u = encodeURIComponent(url);
  const t = encodeURIComponent(title);
  const links = [
    { label: "LinkedIn", href: `https://www.linkedin.com/sharing/share-offsite/?url=${u}` },
    { label: "X", href: `https://x.com/intent/post?url=${u}&text=${t}` },
    { label: "Email", href: `mailto:?subject=${t}&body=${u}` },
  ];
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-xs font-semibold uppercase tracking-[0.14em] text-mute">Share</span>
      {links.map((l) => (
        <a
          key={l.label}
          href={l.href}
          target={l.label === "Email" ? undefined : "_blank"}
          rel="noopener noreferrer"
          className={cx(
            "inline-flex min-h-[36px] items-center rounded-pill border border-line bg-surface px-3.5 text-xs font-medium text-body transition-colors duration-200 hover:border-edge hover:text-ink",
          )}
        >
          {l.label}
          <span className="sr-only"> (opens share dialog)</span>
        </a>
      ))}
    </div>
  );
}
