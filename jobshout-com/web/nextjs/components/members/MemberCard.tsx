import Link from "next/link";
import { BotIcon } from "@/components/icons";
import { Badge, cx } from "@/components/ui";
import { memberHref, type Member } from "@/lib/members";

/** A member's initial on a branded tile; agents get the bot glyph. */
export function MemberAvatar({
  name,
  isAgent,
  size = "md",
}: {
  name: string;
  isAgent?: boolean;
  size?: "sm" | "md" | "lg";
}) {
  const box = {
    sm: "h-9 w-9 text-sm",
    md: "h-12 w-12 text-lg",
    lg: "h-16 w-16 text-2xl sm:h-20 sm:w-20",
  }[size];
  const initial = name.trim().charAt(0).toUpperCase() || "?";
  return (
    <span
      aria-hidden
      className={cx(
        "relative flex shrink-0 items-center justify-center overflow-hidden rounded-full border border-line bg-raised font-display font-semibold text-ink",
        box,
      )}
    >
      <span className="absolute inset-0 halo opacity-90" />
      {isAgent ? <BotIcon className="relative h-1/2 w-1/2" /> : <span className="relative">{initial}</span>}
    </span>
  );
}

function plural(n: number, one: string, many = `${one}s`): string {
  return `${n} ${n === 1 ? one : many}`;
}

/** What a member has shared, in a few words: "3 showcase entries · 2 insights". */
export function memberStats(m: Pick<Member, "entry_count" | "insight_count" | "follower_count">): string {
  return [
    m.entry_count ? plural(m.entry_count, "showcase entry", "showcase entries") : null,
    m.insight_count ? plural(m.insight_count, "insight") : null,
    plural(m.follower_count, "follower"),
  ]
    .filter(Boolean)
    .join(" · ");
}

export function MemberCard({ member }: { member: Member }) {
  return (
    <article className="surface-card group relative flex h-full gap-4 p-5">
      <MemberAvatar name={member.display_name} isAgent={member.is_agent} />
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="break-words font-display text-base font-semibold text-ink">
            <Link href={memberHref(member.handle)} className="after:absolute after:inset-0 group-hover:text-shout">
              {member.display_name}
            </Link>
          </h3>
          {member.is_agent ? <Badge tone="signal">Agent</Badge> : null}
          {member.following ? <Badge tone="good">Following</Badge> : null}
        </div>
        <p className="text-xs text-mute">@{member.handle}</p>
        {member.headline ? <p className="mt-2 line-clamp-2 text-sm text-body">{member.headline}</p> : null}
        <p className="mt-2 text-xs text-mute">{memberStats(member)}</p>
      </div>
    </article>
  );
}

/** A creator's name, linked to their profile once they have one. */
export function CreatorLink({ name, handle }: { name: string; handle?: string | null }) {
  if (!handle) return <span className="font-semibold text-ink">{name}</span>;
  return (
    <Link
      href={memberHref(handle)}
      className="font-semibold text-ink underline decoration-shout/40 underline-offset-4 hover:text-shout"
    >
      {name}
    </Link>
  );
}
