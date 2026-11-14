"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { markNotificationsRead, setNotificationPreference } from "@/lib/notifications";
import { currentViewer } from "@/lib/session";

/** Mark one notification read and follow its link. */
export async function openNotificationAction(form: FormData): Promise<void> {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=%2Fnotifications");
  const id = String(form.get("id") ?? "");
  const href = String(form.get("href") ?? "");
  // Only follow in-site links.
  const target = href.startsWith("/") && !href.startsWith("//") ? href : "/notifications";
  if (id) await markNotificationsRead(viewer, [id]).catch(() => undefined);
  revalidatePath("/notifications");
  redirect(target);
}

export async function markAllReadAction(): Promise<void> {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=%2Fnotifications");
  let outcome = "done=read";
  try {
    await markNotificationsRead(viewer);
  } catch (e) {
    outcome = `error=${encodeURIComponent(e instanceof Error ? e.message : "That did not work.")}`;
  }
  revalidatePath("/notifications");
  redirect(`/notifications?${outcome}`);
}

export async function savePreferenceAction(form: FormData): Promise<void> {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=%2Fnotifications");
  const kind = String(form.get("kind") ?? "");
  const on = form.get("in_app") === "on";
  let outcome = "done=saved";
  try {
    await setNotificationPreference(viewer, kind, on);
  } catch (e) {
    outcome = `error=${encodeURIComponent(e instanceof Error ? e.message : "That did not work.")}`;
  }
  redirect(`/notifications?${outcome}#settings`);
}
