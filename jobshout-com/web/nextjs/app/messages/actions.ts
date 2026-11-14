"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import type { FormState } from "@/lib/form-state";
import {
  openConversation,
  requestConnection,
  respondConnection,
  sendMessage,
  setBlocked,
} from "@/lib/messaging";
import { currentViewer } from "@/lib/session";

function field(form: FormData, key: string): string {
  return ((form.get(key) as string | null) ?? "").trim();
}

const MAX_MESSAGE = 8000;

/** Send one message into a thread the viewer belongs to. */
export async function sendMessageAction(
  conversationId: string,
  _prev: FormState,
  form: FormData,
): Promise<FormState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to send messages." };
  const body = field(form, "body");
  if (!body) return { ok: false, fieldErrors: { body: "Write a message first." } };
  if (body.length > MAX_MESSAGE) {
    return { ok: false, fieldErrors: { body: `Keep it under ${MAX_MESSAGE} characters.` } };
  }
  try {
    await sendMessage(viewer, conversationId, body);
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Message not sent." };
  }
  revalidatePath(`/messages/${conversationId}`);
  revalidatePath("/messages");
  return { ok: true, message: "Sent." };
}

/** "Message" on a profile: open (or reuse) the 1:1 thread and go there. */
export async function startConversationAction(form: FormData): Promise<void> {
  const handle = field(form, "handle");
  const back = `/members/${encodeURIComponent(handle)}`;
  const viewer = await currentViewer();
  if (!viewer) redirect(`/login?callbackUrl=${encodeURIComponent(back)}`);
  let target: string;
  try {
    const c = await openConversation(viewer, handle);
    target = `/messages/${c.id}`;
  } catch (e) {
    target = `${back}?error=${encodeURIComponent(e instanceof Error ? e.message : "Could not message")}`;
  }
  redirect(target);
}

/** Block or unblock the other person in a thread. */
export async function blockAction(form: FormData): Promise<void> {
  const viewer = await currentViewer();
  const handle = field(form, "handle");
  const back = field(form, "back") || "/messages";
  if (!viewer) redirect(`/login?callbackUrl=${encodeURIComponent(back)}`);
  let outcome: string;
  try {
    const block = field(form, "op") !== "unblock";
    await setBlocked(viewer, handle, block);
    outcome = block ? "done=blocked" : "done=unblocked";
  } catch (e) {
    outcome = `error=${encodeURIComponent(e instanceof Error ? e.message : "That did not work.")}`;
  }
  redirect(`${back}?${outcome}`);
}

/** Connect / accept / decline. Comes back to the page it was sent from. */
export async function connectionAction(form: FormData): Promise<void> {
  const viewer = await currentViewer();
  const handle = field(form, "handle");
  const op = field(form, "op");
  const back = field(form, "back") || "/connections";
  if (!viewer) redirect(`/login?callbackUrl=${encodeURIComponent(back)}`);
  let outcome: string;
  try {
    const status =
      op === "accept" || op === "decline"
        ? await respondConnection(viewer, handle, op === "accept")
        : await requestConnection(viewer, handle);
    outcome = `connection=${encodeURIComponent(status)}`;
  } catch (e) {
    outcome = `error=${encodeURIComponent(e instanceof Error ? e.message : "That did not work.")}`;
  }
  revalidatePath("/connections");
  redirect(`${back}${back.includes("?") ? "&" : "?"}${outcome}`);
}
