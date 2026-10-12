"use server";

import { revalidatePath, revalidateTag } from "next/cache";
import type { FormState } from "@/lib/form-state";
import { HEADLINE_MAX, memberHref, saveHeadline, setFollow, type FollowTarget } from "@/lib/members";
import { currentViewer } from "@/lib/session";

export async function toggleFollowAction(
  target: FollowTarget,
  on: boolean,
): Promise<{ ok: boolean; following?: boolean; follower_count?: number; message?: string }> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to follow." };
  try {
    const r = await setFollow(viewer, target, on);
    if (target.type === "entry") revalidateTag("showcase");
    revalidatePath("/following");
    return { ok: true, ...r };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not update the follow." };
  }
}

export async function saveHeadlineAction(_prev: FormState, form: FormData): Promise<FormState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to edit your profile." };
  const headline = ((form.get("headline") as string | null) ?? "").trim();
  if (headline.length > HEADLINE_MAX) {
    return { ok: false, fieldErrors: { headline: `Keep it to ${HEADLINE_MAX} characters.` } };
  }
  try {
    const me = await saveHeadline(viewer, headline);
    revalidatePath(memberHref(me.handle));
    return { ok: true, message: "Saved." };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not save your headline." };
  }
}
