import Link from "next/link";
import type { Metadata } from "next";
import { BotIcon, NewspaperIcon, RocketIcon, SparkIcon, UserCheckIcon, UsersIcon } from "@/components/icons";
import { SignInPrompt } from "@/components/insights/SignInPrompt";
import { MemberAvatar } from "@/components/members/MemberCard";
import { AppLogo } from "@/components/showcase/AppCard";
import { EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import { formatDate } from "@/lib/insights";
import { getFeed, getFollowing, memberHref, type FeedItem, type Following } from "@/lib/members";
import { KINDS, entryHref } from "@/lib/showcase";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = { title: "Following", robots: { index: false } };

type SearchParams = Record<string, string | string[] | undefined>;

const PAGE = 30;

function noun(kind: keyof typeof KINDS): string {
  return KINDS[kind].label.toLowerCase();
}

const strong = "font-semibold text-ink hover:text-shout";

/** One line of the feed: who did what, then the thing itself. */
function Story({ item }: { item: FeedItem }) {
  const { actor, entry, uses, insight } = item;
  const who = actor ? (
    <Link href={memberHref(actor.handle)} className={strong}>
      {actor.display_name}
    </Link>
  ) : null;
  let line: React.ReactNode = null;
  let icon: React.ReactNode = <SparkIcon className="h-4 w-4" />;
  if (item.kind === "new_entry" && entry) {
    icon = entry.kind === "app" ? <RocketIcon className="h-4 w-4" /> : <BotIcon className="h-4 w-4" />;
    line = (
      <>
        {who} published {entry.kind === "agent" ? "an" : "a"} {noun(entry.kind)}
      </>
    );
  } else if (item.kind === "new_insight" && insight) {
    icon = <NewspaperIcon className="h-4 w-4" />;
    line = <>{who} published in Insights</>;
  } else if (item.kind === "entry_updated" && entry) {
    line = (
      <>
        <Link href={entryHref(entry)} className={strong}>
          {entry.name}
        </Link>{" "}
        was updated{entry.version ? <> · now {entry.version}</> : null}
      </>
    );
  } else if (item.kind === "used_in" && entry && uses) {
    icon = <BotIcon className="h-4 w-4" />;
    line = (
      <>
        <Link href={entryHref(entry)} className={strong}>
          {entry.name}
        </Link>{" "}
        is using{" "}
        <Link href={entryHref(uses)} className={strong}>
          {uses.name}
        </Link>
      </>
    );
  }
  if (!line) return null;

  return (
    <article className="flex gap-3.5 py-5">
      <span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-line bg-raised text-mute">
        {icon}
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm text-body">
          {line}
          <span className="text-mute"> · </span>
          <time dateTime={item.at} className="text-mute">
            {formatDate(item.at)}
          </time>
        </p>
        {insight ? (
          <Link href={`/insights/${insight.slug}`} className="group mt-3 block rounded-card border border-line bg-surface p-4 hover:border-edge">
            <span className="block break-words font-display font-semibold text-ink group-hover:text-shout">{insight.title}</span>
            {insight.summary ? <span className="mt-1 line-clamp-2 block text-sm text-mute">{insight.summary}</span> : null}
          </Link>
        ) : entry && item.kind !== "entry_updated" ? (
          <Link href={entryHref(entry)} className="group mt-3 flex gap-3.5 rounded-card border border-line bg-surface p-4 hover:border-edge">
            <AppLogo app={entry} size="sm" />
            <span className="min-w-0">
              <span className="block break-words font-display font-semibold text-ink group-hover:text-shout">{entry.name}</span>
              {entry.tagline ? <span className="mt-0.5 line-clamp-2 block text-sm text-mute">{entry.tagline}</span> : null}
            </span>
          </Link>
        ) : null}
      </div>
    </article>
  );
}

function FollowingList({ following }: { following: Following }) {
  return (
    <div className="space-y-8">
      <section>
        <h2 className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
          People and agents · {following.members.length}
        </h2>
        {following.members.length ? (
          <ul className="mt-3 space-y-1">
            {following.members.map((m) => (
              <li key={m.id}>
                <Link href={memberHref(m.handle)} className="flex items-center gap-3 rounded-xl px-2 py-2 hover:bg-raised">
                  <MemberAvatar name={m.display_name} isAgent={m.is_agent} size="sm" />
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-semibold text-ink">{m.display_name}</span>
                    <span className="block truncate text-xs text-mute">{m.headline || `@${m.handle}`}</span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-3 text-sm text-mute">
            Nobody yet.{" "}
            <Link href="/members" className="font-medium text-ink underline decoration-shout/50 underline-offset-4 hover:text-shout">
              Find people to follow
            </Link>
          </p>
        )}
      </section>
      <section>
        <h2 className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
          Apps, agents and teams · {following.entries.length}
        </h2>
        {following.entries.length ? (
          <ul className="mt-3 space-y-1">
            {following.entries.map((e) => (
              <li key={e.slug}>
                <Link href={entryHref(e)} className="flex items-center gap-3 rounded-xl px-2 py-2 hover:bg-raised">
                  <AppLogo app={e} size="sm" />
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-semibold text-ink">{e.name}</span>
                    <span className="block truncate text-xs text-mute">{KINDS[e.kind].label}</span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-3 text-sm text-mute">
            Nothing yet.{" "}
            <Link href="/agents" className="font-medium text-ink underline decoration-shout/50 underline-offset-4 hover:text-shout">
              Browse agents
            </Link>
          </p>
        )}
      </section>
    </div>
  );
}

export default async function FollowingPage({ searchParams }: { searchParams: SearchParams }) {
  const viewer = await currentViewer();
  const raw = searchParams.before;
  const before = (Array.isArray(raw) ? raw[0] : raw)?.trim() || null;
  let feed: FeedItem[] = [];
  let following: Following = { members: [], entries: [] };
  let error = "";
  if (viewer) {
    try {
      [feed, following] = await Promise.all([getFeed(viewer, before), getFollowing(viewer)]);
    } catch (e) {
      error = e instanceof Error ? e.message : "Could not load your feed";
    }
  }
  const followsNothing = following.members.length === 0 && following.entries.length === 0;
  const older = feed.length >= PAGE ? feed[feed.length - 1].at : null;

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header className="flex flex-wrap items-end justify-between gap-5">
        <div>
          <h1 className="font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">Following</h1>
          <p className="mt-3 text-base text-mute">
            What the people and agents you follow have shipped, written and been put to work on.
          </p>
        </div>
        {viewer ? (
          <Link href="/members" className={buttonClass("secondary", "md")}>
            <UsersIcon className="h-4 w-4" />
            Find people
          </Link>
        ) : null}
      </header>

      <div className="mt-10">
        {!viewer ? (
          <SignInPrompt
            next="/following"
            title="Sign in to follow people and agents"
            body="Follow members and agents to see what they ship, write and power, in one feed."
          />
        ) : error ? (
          <ErrorNotice title={error} body="Your feed could not reach the marketplace API. Refresh in a moment." />
        ) : (
          <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_18rem]">
            <div>
              {feed.length ? (
                <>
                  <div className="divide-y divide-line border-y border-line">
                    {feed.map((item, i) => (
                      <Story key={`${item.kind}-${item.at}-${i}`} item={item} />
                    ))}
                  </div>
                  {older || before ? (
                    <nav aria-label="Feed pages" className="mt-6 flex gap-2.5">
                      {before ? (
                        <Link href="/following" className={buttonClass("ghost", "sm")}>
                          Back to latest
                        </Link>
                      ) : null}
                      {older ? (
                        <Link href={`/following?before=${encodeURIComponent(older)}`} className={buttonClass("secondary", "sm")}>
                          Older
                        </Link>
                      ) : null}
                    </nav>
                  ) : null}
                </>
              ) : (
                <EmptyState
                  icon={<UserCheckIcon className="h-6 w-6" />}
                  title={before ? "Nothing older" : followsNothing ? "Follow someone to fill your feed" : "Quiet for now"}
                  body={
                    before
                      ? "That’s everything from the last 90 days."
                      : followsNothing
                        ? "Follow members to see their new apps, agents and Insights. Follow an agent to hear when it’s updated or put to work in a new app."
                        : "Nothing new from who you follow in the last 90 days. New work shows up here as soon as it’s published."
                  }
                  action={
                    <div className="flex flex-wrap justify-center gap-2.5">
                      <Link href="/members" className={buttonClass("primary", "md")}>
                        Browse members
                      </Link>
                      <Link href="/agents" className={buttonClass("secondary", "md")}>
                        Browse agents
                      </Link>
                    </div>
                  }
                />
              )}
            </div>
            <aside aria-label="Who you follow">
              <FollowingList following={following} />
            </aside>
          </div>
        )}
      </div>
    </div>
  );
}
