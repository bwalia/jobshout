"use server";

import { revalidatePath, revalidateTag } from "next/cache";
import type { FormState } from "@/app/actions";
import {
  confirmNewsletter,
  deleteInsight,
  isInsightKind,
  moderateInsight,
  previewMarkdown,
  saveInsight,
  subscribeNewsletter,
  unsubscribeNewsletter,
  type ModerationAction,
} from "@/lib/insights";
import { currentViewer } from "@/lib/session";

function text(form: FormData, key: string): string {
  return (form.get(key) as string | null)?.trim() ?? "";
}

function isHttpUrl(value: string): boolean {
  return /^https?:\/\/\S+$/i.test(value);
}

/** Map the API's validation message onto the field it is about. */
function fieldFor(message: string): string | null {
  const m = message.toLowerCase();
  if (m.includes("title")) return "title";
  if (m.includes("summary")) return "summary";
  if (m.includes("topic")) return "topics";
  if (m.includes("cover") || m.includes("screen reader")) return "cover_image_url";
  if (m.includes("media") || m.includes("youtube") || m.includes("spotify") || m.includes("link is"))
    return "media_url";
  if (m.includes("link")) return "link_url";
  if (m.includes("word") || m.includes("text")) return "body_md";
  if (m.includes("duration")) return "duration";
  return null;
}

function refresh(slug?: string) {
  revalidateTag("insights");
  revalidatePath("/insights");
  revalidatePath("/newsletter");
  if (slug) revalidatePath(`/insights/${slug}`);
}

export async function saveInsightAction(_prev: FormState, form: FormData): Promise<FormState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to write for Insights." };

  const kind = text(form, "kind");
  const title = text(form, "title");
  const submit = form.get("intent") !== "draft";
  const minutes = Number(text(form, "duration_minutes") || "0");
  const fieldErrors: Record<string, string> = {};

  if (!isInsightKind(kind)) fieldErrors.kind = "Pick what you are publishing.";
  if (!title) fieldErrors.title = "Give it a headline.";
  for (const key of ["cover_image_url", "link_url", "media_url"]) {
    const v = text(form, key);
    if (v && !isHttpUrl(v)) fieldErrors[key] = "Links need to start with https://";
  }
  if (!Number.isFinite(minutes) || minutes < 0 || minutes > 1440) {
    fieldErrors.duration = "Duration is in minutes, up to 24 hours.";
  }
  if (Object.keys(fieldErrors).length > 0) {
    return { ok: false, message: "Check the highlighted fields.", fieldErrors };
  }

  const id = text(form, "id") || undefined;
  try {
    const item = await saveInsight(
      viewer,
      {
        kind: kind as never,
        title,
        summary: text(form, "summary"),
        body_md: text(form, "body_md"),
        cover_image_url: text(form, "cover_image_url"),
        cover_image_alt: text(form, "cover_image_alt"),
        link_url: text(form, "link_url"),
        media_url: text(form, "media_url"),
        duration_seconds: minutes ? Math.round(minutes * 60) : null,
        transcript: text(form, "transcript"),
        topics: form.getAll("topics").map(String),
        submit,
      },
      id,
    );
    refresh(item.slug);
    revalidatePath("/insights/mine");
    return {
      ok: true,
      message:
        item.status === "published"
          ? "Published."
          : item.status === "pending_review"
            ? "Submitted for review."
            : "Draft saved.",
      result: { id: item.slug, status: item.status },
    };
  } catch (e) {
    const message = e instanceof Error ? e.message : "Could not save.";
    const field = fieldFor(message);
    return {
      ok: false,
      message: field ? "Check the highlighted field." : message,
      fieldErrors: field ? { [field]: message.charAt(0).toUpperCase() + message.slice(1) + "." } : undefined,
    };
  }
}

export async function deleteInsightAction(form: FormData): Promise<void> {
  const viewer = await currentViewer();
  if (!viewer) return;
  await deleteInsight(viewer, text(form, "id"));
  refresh();
  revalidatePath("/insights/mine");
}

export async function moderateAction(_prev: FormState, form: FormData): Promise<FormState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in first." };
  const action = text(form, "action") as ModerationAction;
  const note = text(form, "note");
  if (action === "reject" && !note) {
    return {
      ok: false,
      message: "Tell the author what to change.",
      fieldErrors: { note: "Tell the author what to change." },
    };
  }
  try {
    const item = await moderateInsight(viewer, text(form, "id"), action, note);
    refresh(item.slug);
    revalidatePath("/insights/review");
    const done: Record<ModerationAction, string> = {
      approve: "Published.",
      reject: "Sent back to the author.",
      feature: "Featured on the Insights front page.",
      unfeature: "No longer featured.",
      archive: "Archived.",
    };
    return { ok: true, message: done[action], result: { status: item.status } };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not update." };
  }
}

export async function previewAction(body_md: string): Promise<string> {
  if (!body_md.trim()) return "";
  return previewMarkdown(body_md.slice(0, 200_000));
}

export async function subscribeAction(_prev: FormState, form: FormData): Promise<FormState> {
  const email = text(form, "email");
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    return {
      ok: false,
      message: "That does not look like an email address.",
      fieldErrors: { email: "That does not look like an email address." },
    };
  }
  try {
    await subscribeNewsletter(email);
    return { ok: true, message: "Check your inbox to confirm your subscription." };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not subscribe." };
  }
}

export async function confirmSubscriptionAction(
  _prev: FormState,
  form: FormData,
): Promise<FormState> {
  const token = text(form, "token");
  const unsubscribe = form.get("intent") === "unsubscribe";
  try {
    await (unsubscribe ? unsubscribeNewsletter(token) : confirmNewsletter(token));
    return {
      ok: true,
      message: unsubscribe ? "You are unsubscribed." : "You are subscribed.",
      result: { status: unsubscribe ? "unsubscribed" : "confirmed" },
    };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Something went wrong." };
  }
}
