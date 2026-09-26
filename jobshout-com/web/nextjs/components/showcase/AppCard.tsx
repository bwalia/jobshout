import Link from "next/link";
import { BotIcon, RocketIcon, StarIcon } from "@/components/icons";
import { Badge, cx } from "@/components/ui";
import {
  APP_TYPES,
  BUILD_METHODS,
  CAPABILITIES,
  KINDS,
  MATURITY,
  PRICING,
  entryHref,
  isCapability,
  type ShowcaseApp,
} from "@/lib/showcase";

/** The app's logo, or its initial on a branded tile. */
export function AppLogo({
  app,
  size = "md",
}: {
  app: Pick<ShowcaseApp, "name" | "logo_url">;
  size?: "sm" | "md" | "lg";
}) {
  const box = { sm: "h-10 w-10 rounded-xl text-base", md: "h-12 w-12 rounded-xl text-lg", lg: "h-16 w-16 rounded-2xl text-2xl sm:h-20 sm:w-20" }[size];
  if (app.logo_url) {
    return (
      // Logos come from arbitrary hosts; next/image would need each one allowlisted.
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={app.logo_url}
        alt=""
        loading="lazy"
        className={cx("shrink-0 border border-line bg-surface object-cover", box)}
      />
    );
  }
  const initial = app.name.replace(/^Sample:\s*/, "").trim().charAt(0).toUpperCase() || "?";
  return (
    <span
      aria-hidden
      className={cx(
        "relative flex shrink-0 items-center justify-center overflow-hidden border border-line bg-raised font-display font-semibold text-ink",
        box,
      )}
    >
      <span className="absolute inset-0 halo opacity-90" />
      <span className="relative">{initial}</span>
    </span>
  );
}

export function MaturityBadge({ app }: { app: ShowcaseApp }) {
  const ready = app.maturity === "production_ready" || app.maturity === "enterprise_ready";
  return (
    <Badge tone={ready ? "good" : "neutral"}>
      {ready ? <RocketIcon className="h-3 w-3" /> : null}
      {MATURITY[app.maturity].label}
    </Badge>
  );
}

export function BuildBadge({ app }: { app: ShowcaseApp }) {
  const agents = app.build_method === "agent_built" || app.build_method === "agent_autonomous";
  return (
    <Badge tone={agents ? "signal" : "brand"}>
      <BotIcon className="h-3 w-3" />
      {BUILD_METHODS[app.build_method].label}
    </Badge>
  );
}

export function AppCard({ app }: { app: ShowcaseApp }) {
  const tech = app.technologies.slice(0, 4);
  return (
    <article className="group relative h-full">
      <div className="surface-card flex h-full flex-col p-5 transition-all duration-200 ease-out group-hover:-translate-y-0.5 group-hover:border-edge group-hover:shadow-lift">
        <div className="flex items-start gap-3.5">
          <AppLogo app={app} />
          <div className="min-w-0 flex-1">
            <h3 className="break-words font-display text-lg font-semibold leading-snug tracking-[-0.01em] text-ink">
              <Link href={entryHref(app)} className="after:absolute after:inset-0">
                {app.name}
              </Link>
            </h3>
            <p className="mt-0.5 text-xs text-mute">
              {APP_TYPES[app.app_type]}
              {app.creator_display_name ? ` · ${app.team_name || app.creator_display_name}` : ""}
            </p>
          </div>
          <span
            className="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-mute"
            aria-label={`${app.star_count} stars`}
          >
            <StarIcon className={cx("h-3.5 w-3.5", app.starred && "fill-current text-shout")} />
            {app.star_count}
          </span>
        </div>
        {app.tagline ? (
          <p className="mt-3 line-clamp-2 text-sm leading-relaxed text-body">{app.tagline}</p>
        ) : null}
        <div className="mt-4 flex flex-wrap gap-1.5">
          <MaturityBadge app={app} />
          <BuildBadge app={app} />
          {app.pricing === "open_source" ? <Badge>{PRICING.open_source}</Badge> : null}
        </div>
        {tech.length ? (
          <p className="mt-auto pt-4 text-xs text-mute">
            {tech.join(" · ")}
            {app.technologies.length > tech.length ? ` +${app.technologies.length - tech.length}` : ""}
          </p>
        ) : null}
      </div>
    </article>
  );
}

/** Dense row for the detail page's sidebar. */
export function AppRow({ app }: { app: ShowcaseApp }) {
  return (
    <article className="group relative flex gap-3 py-3.5">
      <AppLogo app={app} size="sm" />
      <div className="min-w-0">
        <h3 className="break-words text-sm font-semibold leading-snug text-ink group-hover:text-shout">
          <Link href={entryHref(app)} className="after:absolute after:inset-0">
            {app.name}
          </Link>
        </h3>
        <p className="mt-0.5 line-clamp-1 text-xs text-mute">{app.tagline || APP_TYPES[app.app_type]}</p>
      </div>
    </article>
  );
}

/** Directory card for an agent or a team. */
export function AgentCard({ app }: { app: ShowcaseApp }) {
  const caps = app.capabilities.filter(isCapability).slice(0, 3);
  const team = app.kind === "team";
  return (
    <article className="group relative h-full">
      <div className="surface-card flex h-full flex-col p-5 transition-all duration-200 ease-out group-hover:-translate-y-0.5 group-hover:border-edge group-hover:shadow-lift">
        <div className="flex items-start gap-3.5">
          <AppLogo app={app} />
          <div className="min-w-0 flex-1">
            <h3 className="break-words font-display text-lg font-semibold leading-snug tracking-[-0.01em] text-ink">
              <Link href={entryHref(app)} className="after:absolute after:inset-0">
                {app.name}
              </Link>
            </h3>
            <p className="mt-0.5 text-xs text-mute">
              {team
                ? `${KINDS.team.label} · ${app.linked_agents.length} agents`
                : [app.model_provider, app.ai_models[0]].filter(Boolean).join(" · ") || KINDS.agent.label}
            </p>
          </div>
          <span
            className="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-mute"
            aria-label={`${app.star_count} stars`}
          >
            <StarIcon className={cx("h-3.5 w-3.5", app.starred && "fill-current text-shout")} />
            {app.star_count}
          </span>
        </div>
        {app.tagline ? <p className="mt-3 line-clamp-2 text-sm leading-relaxed text-body">{app.tagline}</p> : null}
        <div className="mt-4 flex flex-wrap gap-1.5">
          {team
            ? app.linked_agents.slice(0, 4).map((a) => <Badge key={a.slug}>{a.name.replace(/^Sample:\s*/, "")}</Badge>)
            : caps.map((c) => (
                <Badge key={c} tone="signal">
                  {CAPABILITIES[c]}
                </Badge>
              ))}
        </div>
        <p className="mt-auto pt-4 text-xs text-mute">
          Used in {app.used_in} {app.used_in === 1 ? "app" : "apps"}
          {app.pricing === "open_source" ? " · Open source" : ""}
        </p>
      </div>
    </article>
  );
}
