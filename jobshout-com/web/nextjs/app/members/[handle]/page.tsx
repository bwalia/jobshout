import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ArrowLeftIcon, PlusIcon, UserCheckIcon } from "@/components/icons";
import { FollowButton } from "@/components/FollowButton";
import { InsightCard } from "@/components/insights/InsightCard";
import { HeadlineForm } from "@/components/members/HeadlineForm";
import { MemberAvatar, memberStats } from "@/components/members/MemberCard";
import { AgentCard, AppCard } from "@/components/showcase/AppCard";
import { Badge, EmptyState, buttonClass } from "@/components/ui";
import { formatDate } from "@/lib/insights";
import { getMember, memberHref } from "@/lib/members";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

type Params = { params: { handle: string } };

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const p = await getMember(params.handle).catch(() => null);
  if (!p) return { title: "Not found" };
  const { member } = p;
  const hasWork = member.entry_count + member.insight_count > 0;
  const description =
    member.headline || `${member.display_name} on JobShout: ${memberStats(member)}.`;
  return {
    title: `${member.display_name} (@${member.handle})`,
    description,
    alternates: { canonical: memberHref(member.handle) },
    // A profile with nothing public on it is not worth a search result.
    robots: hasWork ? undefined : { index: false, follow: false },
    openGraph: { type: "profile", title: member.display_name, description, url: memberHref(member.handle) },
  };
}

function Section({ title, count, children }: { title: string; count: number; children: React.ReactNode }) {
  if (!count) return null;
  return (
    <section className="mt-12">
      <h2 className="flex items-baseline gap-2 font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
        {title}
        <span className="text-base font-medium text-mute">{count}</span>
      </h2>
      <div className="mt-5">{children}</div>
    </section>
  );
}

export default async function MemberPage({ params }: Params) {
  const viewer = await currentViewer();
  const profile = await getMember(params.handle, viewer);
  if (!profile) notFound();
  const { member, is_self, entries, insights } = profile;
  const apps = entries.filter((e) => e.kind === "app");
  const agents = entries.filter((e) => e.kind !== "app");
  const empty = entries.length === 0 && insights.length === 0;
  const href = memberHref(member.handle);

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-8 sm:px-8 sm:pt-10">
      <Link href="/members" className="inline-flex items-center gap-1.5 text-sm font-medium text-mute hover:text-ink">
        <ArrowLeftIcon className="h-4 w-4" />
        Members
      </Link>

      <header className="mt-6 flex flex-col gap-6 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 gap-5">
          <MemberAvatar name={member.display_name} isAgent={member.is_agent} size="lg" />
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="break-words font-display text-3xl font-semibold leading-[1.1] tracking-[-0.025em] text-ink sm:text-[2.5rem]">
                {member.display_name}
              </h1>
              {member.is_agent ? <Badge tone="signal">Agent</Badge> : null}
            </div>
            <p className="mt-1 text-sm text-mute">@{member.handle}</p>
            {member.headline ? <p className="mt-3 max-w-2xl text-lg leading-relaxed text-body">{member.headline}</p> : null}
            <p className="mt-3 text-sm text-mute">
              {memberStats(member)} · following {member.following_count} · joined {formatDate(member.created_at)}
            </p>
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-start gap-2.5">
          {is_self ? (
            <Link href="/following" className={buttonClass("secondary", "md")}>
              <UserCheckIcon className="h-4 w-4" />
              Your feed
            </Link>
          ) : (
            <FollowButton
              target={{ type: "member", handle: member.handle }}
              following={member.following}
              count={member.follower_count}
              signedIn={Boolean(viewer)}
              returnTo={href}
            />
          )}
        </div>
      </header>

      {is_self ? <HeadlineForm headline={member.headline} /> : null}

      {empty ? (
        <div className="mt-12">
          <EmptyState
            icon={<PlusIcon className="h-6 w-6" />}
            title={is_self ? "Nothing public yet" : `${member.display_name} hasn’t published anything yet`}
            body={
              is_self
                ? "Showcase an app or agent you built, or write an Insight. Published work shows up here for your followers."
                : "Follow them and their first app, agent or Insight will land in your feed."
            }
            action={
              is_self ? (
                <Link href="/showcase/new" className={buttonClass("primary", "md")}>
                  Showcase your work
                </Link>
              ) : undefined
            }
          />
        </div>
      ) : null}

      <Section title="Apps" count={apps.length}>
        <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {apps.map((a) => (
            <li key={a.id}>
              <AppCard app={a} />
            </li>
          ))}
        </ul>
      </Section>

      <Section title="Agents & teams" count={agents.length}>
        <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {agents.map((a) => (
            <li key={a.id}>
              <AgentCard app={a} />
            </li>
          ))}
        </ul>
      </Section>

      <Section title="Insights" count={insights.length}>
        <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {insights.map((i) => (
            <li key={i.id}>
              <InsightCard item={i} />
            </li>
          ))}
        </ul>
      </Section>
    </div>
  );
}
