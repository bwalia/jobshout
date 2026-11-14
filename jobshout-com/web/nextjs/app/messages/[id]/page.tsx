import Link from "next/link";
import type { Metadata } from "next";
import { notFound, redirect } from "next/navigation";
import { blockAction } from "@/app/messages/actions";
import { MessageComposer } from "@/components/MessageComposer";
import { ErrorNotice, buttonClass } from "@/components/ui";
import { listConversations, listMessages, type Message } from "@/lib/messaging";
import { relativeTime } from "@/lib/format";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Conversation",
  robots: { index: false, follow: false },
};

export default async function ConversationPage({
  params,
  searchParams,
}: {
  params: { id: string };
  searchParams: { done?: string; error?: string };
}) {
  const viewer = await currentViewer();
  if (!viewer) redirect(`/login?callbackUrl=/messages/${params.id}`);

  // The thread list says who the other person is; a thread the viewer is
  // not in is absent here and 403s below, so it renders as not found.
  const conversations = await listConversations(viewer).catch(() => null);
  if (conversations === null) redirect("/messages");
  const conversation = conversations.find((c) => c.id === params.id);
  if (!conversation) notFound();

  let messages: Message[] = [];
  let error = "";
  try {
    messages = (await listMessages(viewer, params.id)).reverse();
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load the conversation";
  }

  const peer = conversation.peer_handle ?? "";
  const back = `/messages/${params.id}`;

  return (
    <div className="mx-auto flex max-w-3xl flex-col px-5 pb-24 pt-8 sm:px-8">
      <Link href="/messages" className="text-sm font-medium text-mute hover:text-ink">
        ← All messages
      </Link>
      <header className="mt-4 flex flex-wrap items-end justify-between gap-3 border-b border-line pb-4">
        <div className="min-w-0">
          <h1 className="font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
            {conversation.peer_display_name || conversation.subject || "Conversation"}
          </h1>
          {peer ? (
            <Link href={`/members/${peer}`} className="text-sm text-mute hover:text-ink">
              @{peer}
            </Link>
          ) : null}
        </div>
        {peer ? (
          <form action={blockAction} className="flex gap-2">
            <input type="hidden" name="handle" value={peer} />
            <input type="hidden" name="back" value={back} />
            <input type="hidden" name="op" value={searchParams.done === "blocked" ? "unblock" : "block"} />
            <button type="submit" className={buttonClass("ghost", "sm")}>
              {searchParams.done === "blocked" ? "Unblock" : "Block"}
            </button>
          </form>
        ) : null}
      </header>

      {searchParams.done === "blocked" ? (
        <p role="status" className="mt-4 text-sm text-mute">
          Blocked. Neither of you can send messages in this conversation until you unblock.
        </p>
      ) : null}
      {searchParams.error ? (
        <div className="mt-4">
          <ErrorNotice title={searchParams.error} />
        </div>
      ) : null}
      {error ? (
        <div className="mt-4">
          <ErrorNotice title={error} />
        </div>
      ) : null}

      <ol className="mt-6 space-y-3" aria-label="Messages">
        {messages.length === 0 && !error ? (
          <li className="py-10 text-center text-sm text-mute">No messages yet. Say hello.</li>
        ) : null}
        {messages.map((m) => {
          const mine = m.sender_handle !== peer;
          return (
            <li key={m.id} className={`flex ${mine ? "justify-end" : "justify-start"}`}>
              <div
                className={`max-w-[85%] rounded-2xl px-4 py-2.5 text-sm leading-relaxed ${
                  mine ? "bg-ink text-bg" : "border border-line bg-surface text-ink"
                }`}
              >
                <p className="whitespace-pre-wrap break-words">{m.body}</p>
                <p className={`mt-1 text-[11px] ${mine ? "text-bg/70" : "text-mute"}`}>
                  {mine ? "You" : `@${m.sender_handle}`} · {relativeTime(m.created_at)}
                </p>
              </div>
            </li>
          );
        })}
      </ol>

      <div className="mt-8">
        <MessageComposer conversationId={params.id} />
      </div>
    </div>
  );
}
