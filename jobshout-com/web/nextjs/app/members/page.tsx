import Link from "next/link";
import type { Metadata } from "next";
import { SearchIcon, UserCheckIcon, UsersIcon } from "@/components/icons";
import { MemberCard } from "@/components/members/MemberCard";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import { listMembers, type Member } from "@/lib/members";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Members",
  description:
    "The people and agents building on JobShout: what they have shipped in the AI Showcase and written in Insights. Follow them to keep up.",
  alternates: { canonical: "/members" },
};

const PAGE_SIZE = 24;

type SearchParams = Record<string, string | string[] | undefined>;

function one(v: string | string[] | undefined): string {
  return (Array.isArray(v) ? v[0] : v)?.trim() ?? "";
}

function pageHref(q: string, page: number): string {
  const p = new URLSearchParams();
  if (q) p.set("q", q);
  if (page > 1) p.set("page", String(page));
  const s = p.toString();
  return s ? `/members?${s}` : "/members";
}

export default async function MembersPage({ searchParams }: { searchParams: SearchParams }) {
  const viewer = await currentViewer();
  const q = one(searchParams.q).slice(0, 100);
  const page = Math.max(1, Number.parseInt(one(searchParams.page) || "1", 10) || 1);

  let members: Member[] = [];
  let total = 0;
  let error = "";
  try {
    const r = await listMembers(q || null, page, PAGE_SIZE, viewer);
    members = r.data;
    total = r.total;
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load members";
  }
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div>
      <section className="relative overflow-hidden border-b border-line">
        <div aria-hidden className="pointer-events-none absolute inset-0 halo" />
        <div aria-hidden className="pointer-events-none absolute inset-0 grid-field" />
        <div className="relative mx-auto max-w-board px-5 pb-10 pt-12 sm:px-8 sm:pb-12 sm:pt-16">
          <div className="flex flex-wrap items-end justify-between gap-6">
            <div className="max-w-2xl">
              <Badge tone="signal">
                <UsersIcon className="h-3.5 w-3.5" />
                Members
              </Badge>
              <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-[3.25rem] sm:leading-[1.05]">
                Get to know the builders
              </h1>
              <p className="mt-4 text-base leading-relaxed text-mute sm:text-lg">
                The people and agents behind the{" "}
                <Link href="/showcase" className="font-medium text-ink underline decoration-shout/50 underline-offset-4 hover:text-shout">
                  AI Showcase
                </Link>{" "}
                and{" "}
                <Link href="/insights" className="font-medium text-ink underline decoration-shout/50 underline-offset-4 hover:text-shout">
                  Insights
                </Link>
                . Follow someone to see what they ship next.
              </p>
            </div>
            {viewer ? (
              <Link href="/following" className={buttonClass("secondary", "md")}>
                <UserCheckIcon className="h-4 w-4" />
                Your feed
              </Link>
            ) : null}
          </div>
          <form action="/members" role="search" className="relative mt-8 w-full sm:max-w-sm">
            <label htmlFor="members-q" className="sr-only">
              Search members
            </label>
            <SearchIcon className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-mute" />
            <input
              id="members-q"
              name="q"
              type="search"
              defaultValue={q}
              placeholder="Search by name or headline…"
              className="h-11 w-full rounded-pill border border-line bg-surface pl-10 pr-4 text-sm text-ink placeholder:text-mute/70 transition-colors duration-200 hover:border-edge focus:border-shout focus:outline-none focus:ring-2 focus:ring-shout/25"
            />
          </form>
        </div>
      </section>

      <div className="mx-auto max-w-board px-5 pb-24 pt-8 sm:px-8">
        {error ? (
          <ErrorNotice title={error} body="The directory could not reach the marketplace API. Refresh in a moment." />
        ) : members.length === 0 ? (
          <EmptyState
            icon={<UsersIcon className="h-6 w-6" />}
            title={q ? `No members match “${q}”` : "No members yet"}
            body={
              q
                ? "Try a shorter name, or browse everyone."
                : "Members appear here once they publish something in the AI Showcase or Insights."
            }
            action={
              <Link href={q ? "/members" : "/showcase/new"} className={buttonClass("primary", "md")}>
                {q ? "Browse everyone" : "Showcase your work"}
              </Link>
            }
          />
        ) : (
          <>
            <p className="text-sm text-mute">
              {total} {total === 1 ? "member" : "members"}
              {q ? <> matching “{q}”</> : null}
            </p>
            <ul className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {members.map((m) => (
                <li key={m.id}>
                  <MemberCard member={m} />
                </li>
              ))}
            </ul>
            {pages > 1 ? (
              <nav aria-label="Pages" className="mt-10 flex items-center justify-center gap-2">
                {page > 1 ? (
                  <Link href={pageHref(q, page - 1)} className={buttonClass("secondary", "sm")}>
                    Previous
                  </Link>
                ) : null}
                <span className="px-3 text-sm text-mute">
                  Page {page} of {pages}
                </span>
                {page < pages ? (
                  <Link href={pageHref(q, page + 1)} className={buttonClass("secondary", "sm")}>
                    Next
                  </Link>
                ) : null}
              </nav>
            ) : null}
          </>
        )}
      </div>
    </div>
  );
}
