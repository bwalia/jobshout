import Link from "next/link";
import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { UsersIcon } from "@/components/icons";
import { EmptyState, buttonClass } from "@/components/ui";
import { API_BASE, failure, headers } from "@/lib/insights";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Messages",
  robots: { index: false, follow: false },
};

type Conversation = {
  id: string;
  kind: string;
  subject: string;
  updated_at: string;
  unread: number;
  peer_handle?: string | null;
  peer_display_name?: string | null;
  last_body?: string | null;
};

export default async function MessagesPage() {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=/messages");

  let conversations: Conversation[] = [];
  let missing = false;
  let error = "";
  try {
    const res = await fetch(`${API_BASE}/api/v1/messages`, {
      headers: headers(viewer),
      cache: "no-store",
    });
    if (res.status === 404) missing = true;
    else if (!res.ok) throw await failure(res, "Failed to load messages");
    else {
      const body = (await res.json()) as { data: Conversation[] };
      conversations = body.data;
    }
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load messages";
  }

  if (missing) {
    return (
      <div className="mx-auto max-w-board px-5 py-16 sm:px-8">
        <EmptyState
          icon={<UsersIcon className="h-6 w-6" />}
          title="Messaging is dark on this ring"
          body="Set MESSAGING_ENABLED to turn on conversations. Until then, use follows and the dashboard."
        />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl px-5 pb-24 pt-8 sm:px-8">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="font-display text-3xl font-semibold tracking-[-0.03em] text-ink">Messages</h1>
          <p className="mt-2 text-mute">Direct conversations with connections and collaborators.</p>
        </div>
        <Link href="/members" className={buttonClass("secondary", "md")}>
          Find people
        </Link>
      </div>

      {error ? <p className="mt-8 text-mute">{error}</p> : null}

      {!error && conversations.length === 0 ? (
        <div className="mt-12">
          <EmptyState
            icon={<UsersIcon className="h-6 w-6" />}
            title="No conversations yet"
            body="Open a member profile and send a message once messaging is enabled for your account."
          />
        </div>
      ) : (
        <ul className="mt-8 divide-y divide-line rounded-card border border-line bg-surface">
          {conversations.map((c) => (
            <li key={c.id}>
              <Link
                href={`/messages/${c.id}`}
                className="flex items-start justify-between gap-4 px-5 py-4 hover:bg-raised"
              >
                <div className="min-w-0">
                  <p className="font-medium text-ink">
                    {c.peer_display_name || c.subject || "Conversation"}
                    {c.peer_handle ? (
                      <span className="ml-2 text-sm font-normal text-mute">@{c.peer_handle}</span>
                    ) : null}
                  </p>
                  <p className="mt-1 truncate text-sm text-mute">{c.last_body || "No messages yet"}</p>
                </div>
                {c.unread > 0 ? (
                  <span className="rounded-pill bg-shout/15 px-2 py-0.5 text-xs font-semibold text-shout">
                    {c.unread}
                  </span>
                ) : null}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
