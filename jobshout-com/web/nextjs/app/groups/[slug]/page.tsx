import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { JoinButton } from "@/components/groups/JoinButton";
import { PostCard } from "@/components/groups/PostCard";
import { PostComposer } from "@/components/groups/PostComposer";
import { ArrowLeftIcon, UsersIcon } from "@/components/icons";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import {
  GroupsDarkError,
  getGroup,
  kindLabel,
  listGroups,
  listPosts,
  type GroupPost,
} from "@/lib/groups";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

type Params = { slug: string };

export async function generateMetadata({ params }: { params: Params }): Promise<Metadata> {
  const g = await getGroup(params.slug).catch(() => null);
  if (!g) return { title: "Group · JobShout" };
  return {
    title: `${g.name} · Groups · JobShout`,
    description: g.description || `${g.name} on JobShout Groups`,
    alternates: { canonical: `/groups/${g.slug}` },
  };
}

export default async function GroupDetailPage({ params }: { params: Params }) {
  const viewer = await currentViewer();
  const returnTo = `/groups/${params.slug}`;

  let dark = false;
  try {
    await listGroups(1);
  } catch (e) {
    if (
      e instanceof GroupsDarkError ||
      (e instanceof Error && (e.message.includes("GROUPS_ENABLED") || e.message.includes("(404)")))
    ) {
      dark = true;
    }
  }

  if (dark) {
    return (
      <main className="mx-auto max-w-3xl px-5 py-12 sm:px-8">
        <EmptyState
          icon={<UsersIcon className="h-6 w-6" />}
          title="GROUPS_ENABLED is dark on this ring"
          body="Group pages stay quiet until the flag is on."
        />
        <Link href="/groups" className={`${buttonClass("secondary", "md")} mt-8 inline-flex`}>
          Back to Groups
        </Link>
      </main>
    );
  }

  let group = null;
  let posts: GroupPost[] = [];
  let error = "";
  let membersOnlyFeed = false;
  try {
    group = await getGroup(params.slug, viewer);
    if (group) {
      try {
        posts = await listPosts(params.slug, viewer);
      } catch (e) {
        const msg = e instanceof Error ? e.message.toLowerCase() : "";
        if (msg.includes("forbidden") || msg.includes("members only")) {
          membersOnlyFeed = true;
        } else {
          error = e instanceof Error ? e.message : "Could not load posts";
        }
      }
    }
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load group";
  }

  if (!group && !error) notFound();

  if (!group) {
    return (
      <main className="mx-auto max-w-3xl px-5 py-12 sm:px-8">
        <ErrorNotice title={error || "Group not available"} />
        <Link href="/groups" className={`${buttonClass("secondary", "md")} mt-8 inline-flex`}>
          Back to Groups
        </Link>
      </main>
    );
  }

  const isMember = Boolean(group.is_member);

  return (
    <div>
      <section className="border-b border-line bg-surface">
        <div className="mx-auto max-w-board px-5 py-8 sm:px-8 sm:py-10">
          <Link
            href="/groups"
            className="inline-flex items-center gap-1.5 text-sm font-medium text-mute hover:text-ink"
          >
            <ArrowLeftIcon className="h-4 w-4" />
            All groups
          </Link>
          <div className="mt-5 flex flex-wrap items-start justify-between gap-6">
            <div className="min-w-0 max-w-2xl">
              <div className="flex flex-wrap gap-2">
                <Badge tone="neutral">{kindLabel(group.kind)}</Badge>
                <Badge tone="neutral">{group.visibility}</Badge>
                {group.my_role ? <Badge tone="brand">{group.my_role}</Badge> : null}
              </div>
              <h1 className="mt-4 font-display text-3xl font-semibold tracking-[-0.03em] text-ink sm:text-4xl">
                {group.name}
              </h1>
              {group.description ? (
                <p className="mt-3 text-base leading-relaxed text-mute">{group.description}</p>
              ) : null}
              <p className="mt-3 text-sm text-mute">
                {typeof group.member_count === "number"
                  ? `${group.member_count} member${group.member_count === 1 ? "" : "s"}`
                  : null}
                {group.join_policy ? ` · Join: ${group.join_policy}` : null}
              </p>
            </div>
            <JoinButton
              slug={group.slug}
              isMember={isMember}
              joinPolicy={group.join_policy}
              signedIn={Boolean(viewer)}
              returnTo={returnTo}
            />
          </div>
        </div>
      </section>

      <div className="mx-auto max-w-board space-y-6 px-5 py-8 sm:px-8">
        {error ? <ErrorNotice title={error} /> : null}

        {isMember ? (
          <PostComposer slug={group.slug} />
        ) : (
          <div className="rounded-card border border-dashed border-line bg-raised/40 p-5 text-sm text-mute">
            {viewer
              ? "Join this group to post and comment."
              : "Sign in and join to start a discussion."}
          </div>
        )}

        {membersOnlyFeed ? (
          <EmptyState
            icon={<UsersIcon className="h-6 w-6" />}
            title="Members only"
            body="Join this private group to read the feed."
          />
        ) : posts.length === 0 ? (
          <EmptyState
            icon={<UsersIcon className="h-6 w-6" />}
            title="No posts yet"
            body={
              isMember
                ? "Be the first to share a discussion, question or opportunity."
                : "Join the group to start conversations — or wait for members to post."
            }
          />
        ) : (
          <div className="space-y-4">
            {posts.map((p) => (
              <PostCard
                key={p.id}
                slug={group.slug}
                post={p}
                isMember={isMember}
                signedIn={Boolean(viewer)}
                returnTo={returnTo}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
