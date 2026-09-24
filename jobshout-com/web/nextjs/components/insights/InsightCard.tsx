import Link from "next/link";
import { Badge, cx } from "@/components/ui";
import { KindIcon } from "@/components/insights/KindIcon";
import { consumeLabel, formatDate, KIND_META, type Insight } from "@/lib/insights";

/** Cover image, or a branded tile with the format's icon when there is none. */
export function InsightCover({
  item,
  className,
  large,
}: {
  item: Insight;
  className?: string;
  large?: boolean;
}) {
  if (item.cover_image_url) {
    return (
      // Covers come from arbitrary hosts; next/image would need each one allowlisted.
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={item.cover_image_url}
        alt={item.cover_image_alt}
        loading="lazy"
        className={cx("h-full w-full object-cover", className)}
      />
    );
  }
  return (
    <div
      aria-hidden
      className={cx(
        "relative flex h-full w-full items-center justify-center overflow-hidden bg-raised",
        className,
      )}
    >
      <div className="absolute inset-0 halo opacity-90" />
      <div className="absolute inset-0 grid-field" />
      <span
        className={cx(
          "relative flex items-center justify-center rounded-2xl border border-line bg-surface/80 text-ink shadow-card backdrop-blur",
          large ? "h-20 w-20" : "h-12 w-12",
        )}
      >
        <KindIcon kind={item.kind} className={large ? "h-9 w-9" : "h-6 w-6"} />
      </span>
    </div>
  );
}

function Meta({ item }: { item: Insight }) {
  return (
    <p className="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-xs text-mute">
      {item.author_display_name ? (
        <span className="font-medium text-body">{item.author_display_name}</span>
      ) : null}
      {item.published_at ? (
        <>
          <span aria-hidden className="h-1 w-1 rounded-full bg-edge" />
          <time dateTime={item.published_at}>{formatDate(item.published_at)}</time>
        </>
      ) : null}
      <span aria-hidden className="h-1 w-1 rounded-full bg-edge" />
      <span>{consumeLabel(item)}</span>
    </p>
  );
}

export function KindBadge({ item }: { item: Insight }) {
  return (
    <Badge tone={item.kind === "podcast" || item.kind === "video" ? "signal" : "brand"}>
      <KindIcon kind={item.kind} className="h-3 w-3" />
      {KIND_META[item.kind].label}
    </Badge>
  );
}

export function InsightCard({ item }: { item: Insight }) {
  return (
    <article className="group relative h-full">
      <div className="surface-card flex h-full flex-col overflow-hidden transition-all duration-200 ease-out group-hover:-translate-y-0.5 group-hover:border-edge group-hover:shadow-lift">
        <div className="aspect-[16/9] overflow-hidden border-b border-line">
          <InsightCover item={item} className="transition-transform duration-500 ease-out group-hover:scale-[1.02]" />
        </div>
        <div className="flex flex-1 flex-col p-5">
          <div className="flex flex-wrap items-center gap-2">
            <KindBadge item={item} />
            {item.topics.slice(0, 1).map((t) => (
              <Badge key={t.slug}>{t.name}</Badge>
            ))}
          </div>
          <h3 className="mt-3 break-words font-display text-lg font-semibold leading-snug tracking-[-0.01em] text-ink">
            <Link href={`/insights/${item.slug}`} className="after:absolute after:inset-0">
              {item.title}
            </Link>
          </h3>
          {item.summary ? (
            <p className="mt-2 line-clamp-3 text-sm leading-relaxed text-mute">{item.summary}</p>
          ) : null}
          <div className="mt-auto pt-4">
            <Meta item={item} />
          </div>
        </div>
      </div>
    </article>
  );
}

/** Wide lead story for the top of the landing page. */
export function InsightHero({ item }: { item: Insight }) {
  return (
    <article className="group relative">
      <div className="surface-card grid overflow-hidden transition-all duration-200 ease-out group-hover:border-edge group-hover:shadow-lift md:grid-cols-[1.15fr_1fr]">
        <div className="aspect-[16/9] overflow-hidden border-b border-line md:aspect-auto md:min-h-[20rem] md:border-b-0 md:border-r">
          <InsightCover item={item} large />
        </div>
        <div className="flex flex-col justify-center p-6 sm:p-9">
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone="solid">Featured</Badge>
            <KindBadge item={item} />
          </div>
          <h2 className="mt-4 break-words font-display text-2xl font-semibold leading-tight tracking-[-0.02em] text-ink sm:text-[2rem]">
            <Link href={`/insights/${item.slug}`} className="after:absolute after:inset-0">
              {item.title}
            </Link>
          </h2>
          {item.summary ? (
            <p className="mt-3 text-base leading-relaxed text-mute">{item.summary}</p>
          ) : null}
          <div className="mt-6">
            <Meta item={item} />
          </div>
        </div>
      </div>
    </article>
  );
}

/** Dense one-line row for sidebars and the newsletter archive. */
export function InsightRow({ item }: { item: Insight }) {
  return (
    <article className="group relative flex gap-3.5 py-3.5">
      <span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-line bg-raised text-mute transition-colors duration-200 group-hover:text-shout">
        <KindIcon kind={item.kind} className="h-4 w-4" />
      </span>
      <div className="min-w-0">
        <h3 className="break-words text-sm font-semibold leading-snug text-ink">
          <Link href={`/insights/${item.slug}`} className="after:absolute after:inset-0">
            {item.title}
          </Link>
        </h3>
        <p className="mt-1 text-xs text-mute">
          {KIND_META[item.kind].label} · {consumeLabel(item)}
        </p>
      </div>
    </article>
  );
}

export function InsightCardSkeleton() {
  return (
    <div className="surface-card overflow-hidden">
      <div className="skeleton aspect-[16/9]" />
      <div className="space-y-3 p-5">
        <div className="skeleton h-4 w-24 rounded-pill" />
        <div className="skeleton h-5 w-3/4 rounded" />
        <div className="skeleton h-4 w-full rounded" />
      </div>
    </div>
  );
}
