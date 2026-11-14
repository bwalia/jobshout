import Link from "next/link";
import type { Metadata } from "next";
import { CreateGroupForm } from "@/components/groups/CreateGroupForm";
import { SearchIcon, UsersIcon } from "@/components/icons";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import {
  GroupsDarkError,
  kindLabel,
  listGroups,
  searchGroups,
  type CommunityGroup,
} from "@/lib/groups";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Groups · JobShout",
  description: "Skill, industry and AI-agent communities on JobShout.",
  alternates: { canonical: "/groups" },
};

type SearchParams = Record<string, string | string[] | undefined>;

function one(v: string | string[] | undefined): string {
  return (Array.isArray(v) ? v[0] : v)?.trim() ?? "";
}

export default async function GroupsPage({ searchParams }: { searchParams: SearchParams }) {
  const viewer = await currentViewer();
  const q = one(searchParams.q).slice(0, 100);

  let groups: CommunityGroup[] = [];
  let dark = false;
  let error = "";
  try {
    groups = q ? await searchGroups(q) : await listGroups();
  } catch (e) {
    if (e instanceof GroupsDarkError || (e instanceof Error && e.message.includes("GROUPS_ENABLED"))) {
      dark = true;
    } else if (e instanceof Error && e.message.includes("(404)")) {
      dark = true;
    } else {
      error = e instanceof Error ? e.message : "Could not load groups";
    }
  }

  return (
    <div>
      <section className="relative overflow-hidden border-b border-line">
        <div aria-hidden className="pointer-events-none absolute inset-0 halo" />
        <div aria-hidden className="pointer-events-none absolute inset-0 grid-field" />
        <div className="relative mx-auto max-w-board px-5 pb-10 pt-12 sm:px-8 sm:pb-12 sm:pt-16">
          <Badge tone="signal">
            <UsersIcon className="h-3.5 w-3.5" />
            Linked Groups
          </Badge>
          <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-[3.25rem] sm:leading-[1.05]">
            Communities that ship
          </h1>
          <p className="mt-4 max-w-2xl text-base leading-relaxed text-mute sm:text-lg">
            Skill, industry and AI-agent groups. Secret groups never appear in search.
          </p>
        </div>
      </section>

      <div className="mx-auto max-w-board px-5 py-8 sm:px-8">
        {dark ? (
          <EmptyState
            icon={<UsersIcon className="h-6 w-6" />}
            title="GROUPS_ENABLED is dark on this ring"
            body="When the flag is on, browse official groups and create your own."
          />
        ) : null}
        {error ? <ErrorNotice title={error} /> : null}

        {!dark ? (
          <>
            <form method="get" className="mb-8 flex flex-wrap items-center gap-3">
              <div className="relative min-w-[12rem] flex-1">
                <SearchIcon className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-mute" />
                <input
                  type="search"
                  name="q"
                  defaultValue={q}
                  placeholder="Search groups"
                  className="w-full rounded-pill border border-line bg-bg py-2.5 pl-10 pr-4 text-ink placeholder:text-mute"
                />
              </div>
              <button type="submit" className={buttonClass("secondary", "md")}>
                Search
              </button>
            </form>

            <div className="grid gap-10 lg:grid-cols-[1fr_22rem]">
              <div className="space-y-4">
                {!error && groups.length === 0 ? (
                  <EmptyState
                    icon={<UsersIcon className="h-6 w-6" />}
                    title={q ? "No matching groups" : "No groups yet"}
                    body={
                      q
                        ? "Try a different search, or create a new group."
                        : "Be the first to create a public skill or industry group."
                    }
                  />
                ) : null}
                {groups.map((g) => (
                  <article
                    key={g.id}
                    className="rounded-card border border-line bg-surface p-5 transition-colors hover:border-ink/30"
                  >
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0">
                        <h2 className="font-display text-xl font-semibold text-ink">
                          <Link href={`/groups/${g.slug}`} className="hover:text-shout">
                            {g.name}
                          </Link>
                        </h2>
                        <p className="mt-1 text-sm text-mute">
                          {kindLabel(g.kind)} · {g.visibility}
                          {typeof g.member_count === "number"
                            ? ` · ${g.member_count} member${g.member_count === 1 ? "" : "s"}`
                            : null}
                        </p>
                        {g.description ? (
                          <p className="mt-2 text-body">{g.description}</p>
                        ) : null}
                      </div>
                      <Link href={`/groups/${g.slug}`} className={buttonClass("secondary", "sm")}>
                        Open
                      </Link>
                    </div>
                  </article>
                ))}
              </div>

              <aside>
                {viewer ? (
                  <CreateGroupForm />
                ) : (
                  <div className="rounded-card border border-line bg-surface p-5">
                    <h2 className="font-display text-lg font-semibold text-ink">Create a group</h2>
                    <p className="mt-2 text-sm text-mute">
                      Sign in to start a skill, industry or AI-agent community.
                    </p>
                    <Link
                      href={`/login?callbackUrl=${encodeURIComponent("/groups")}`}
                      className={`${buttonClass("primary", "md")} mt-4 inline-flex`}
                    >
                      Sign in
                    </Link>
                  </div>
                )}
              </aside>
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}
