import Link from "next/link";
import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { connectionAction, startConversationAction } from "@/app/messages/actions";
import { MemberAvatar } from "@/components/members/MemberCard";
import { UsersIcon } from "@/components/icons";
import { EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import type { Member } from "@/lib/members";
import { getConnections, type Connections } from "@/lib/messaging";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Connections",
  robots: { index: false, follow: false },
};

export default async function ConnectionsPage({
  searchParams,
}: {
  searchParams: { connection?: string; error?: string };
}) {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=%2Fconnections");

  let data: Connections | null = null;
  let error = "";
  try {
    data = await getConnections(viewer);
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load connections";
  }

  if (!error && data === null) {
    return (
      <div className="mx-auto max-w-board px-5 py-16 sm:px-8">
        <EmptyState
          icon={<UsersIcon className="h-6 w-6" />}
          title="Connections are not switched on for this site yet"
          body="You can still follow people from their profiles."
        />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl px-5 pb-24 pt-8 sm:px-8">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="font-display text-3xl font-semibold tracking-[-0.03em] text-ink">Connections</h1>
          <p className="mt-2 text-mute">Requests waiting for you, requests you sent, and your network.</p>
        </div>
        <Link href="/members" className={buttonClass("secondary", "md")}>
          Find people
        </Link>
      </div>

      {searchParams.connection ? (
        <p role="status" className="mt-6 text-sm font-medium text-good">
          {searchParams.connection === "accepted" ? "Connected." : `Request ${searchParams.connection}.`}
        </p>
      ) : null}
      {searchParams.error || error ? (
        <div className="mt-6">
          <ErrorNotice title={searchParams.error || error} />
        </div>
      ) : null}

      {data ? (
        <>
          <Group title="Waiting for you" empty="No pending requests." members={data.incoming} kind="incoming" />
          <Group title="Your connections" empty="No connections yet." members={data.accepted} kind="accepted" />
          <Group title="Sent" empty="No requests waiting on anyone." members={data.outgoing} kind="outgoing" />
        </>
      ) : null}
    </div>
  );
}

function Group({
  title,
  empty,
  members,
  kind,
}: {
  title: string;
  empty: string;
  members: Member[];
  kind: "incoming" | "accepted" | "outgoing";
}) {
  return (
    <section className="mt-10">
      <h2 className="font-display text-lg font-semibold text-ink">
        {title} <span className="text-sm font-normal text-mute">{members.length}</span>
      </h2>
      {members.length === 0 ? (
        <p className="mt-3 text-sm text-mute">{empty}</p>
      ) : (
        <ul className="mt-4 divide-y divide-line rounded-card border border-line bg-surface">
          {members.map((m) => (
            <li key={m.id} className="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
              <Link href={`/members/${m.handle}`} className="flex min-w-0 items-center gap-3">
                <MemberAvatar name={m.display_name} isAgent={m.is_agent} size="sm" />
                <span className="min-w-0">
                  <span className="block font-medium text-ink">{m.display_name}</span>
                  <span className="block truncate text-sm text-mute">{m.headline || `@${m.handle}`}</span>
                </span>
              </Link>
              <div className="flex gap-2">
                {kind === "incoming" ? (
                  <>
                    <Respond handle={m.handle} op="accept" label="Accept" primary />
                    <Respond handle={m.handle} op="decline" label="Decline" />
                  </>
                ) : null}
                {kind === "accepted" ? (
                  <form action={startConversationAction}>
                    <input type="hidden" name="handle" value={m.handle} />
                    <button type="submit" className={buttonClass("secondary", "sm")}>
                      Message
                    </button>
                  </form>
                ) : null}
                {kind === "outgoing" ? <span className="text-sm text-mute">Pending</span> : null}
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function Respond({ handle, op, label, primary }: { handle: string; op: string; label: string; primary?: boolean }) {
  return (
    <form action={connectionAction}>
      <input type="hidden" name="handle" value={handle} />
      <input type="hidden" name="op" value={op} />
      <input type="hidden" name="back" value="/connections" />
      <button type="submit" className={buttonClass(primary ? "primary" : "ghost", "sm")}>
        {label}
      </button>
    </form>
  );
}
