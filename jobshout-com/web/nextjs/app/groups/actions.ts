"use server";

import { revalidatePath } from "next/cache";
import type { FormState } from "@/lib/form-state";
import {
  addComment,
  createGroup,
  createPost,
  joinGroup,
  listComments,
  reactToPost,
  slugify,
  type GroupComment,
} from "@/lib/groups";
import { currentViewer } from "@/lib/session";

function text(form: FormData, key: string): string {
  return (form.get(key) as string | null)?.trim() ?? "";
}

export async function createGroupAction(_prev: FormState, form: FormData): Promise<FormState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to create a group." };

  const name = text(form, "name");
  let slug = text(form, "slug") || slugify(name);
  slug = slugify(slug);
  const description = text(form, "description");
  const kind = text(form, "kind") || "skill";
  const visibility = text(form, "visibility") || "public";
  const join_policy = text(form, "join_policy") || "open";

  if (name.length < 2) {
    return { ok: false, fieldErrors: { name: "Give the group a name." } };
  }
  if (slug.length < 2) {
    return { ok: false, fieldErrors: { slug: "Pick a short URL slug." } };
  }

  try {
    const g = await createGroup(viewer, {
      name,
      slug,
      description,
      kind,
      visibility,
      join_policy,
    });
    revalidatePath("/groups");
    revalidatePath(`/groups/${g.slug}`);
    return { ok: true, result: { id: g.slug, status: g.slug } };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not create group." };
  }
}

export async function joinGroupAction(slug: string): Promise<{ ok: boolean; message?: string }> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to join." };
  try {
    await joinGroup(viewer, slug);
    revalidatePath(`/groups/${slug}`);
    return { ok: true };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not join." };
  }
}

export async function createPostAction(
  slug: string,
  _prev: FormState,
  form: FormData,
): Promise<FormState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to post." };
  const title = text(form, "title");
  const body = text(form, "body");
  const kind = text(form, "kind") || "discussion";
  if (body.length < 1) {
    return { ok: false, fieldErrors: { body: "Write something to share." } };
  }
  try {
    const post = await createPost(viewer, slug, { title, body, kind });
    revalidatePath(`/groups/${slug}`);
    return { ok: true, result: { id: post.id } };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not post." };
  }
}

export async function addCommentAction(
  slug: string,
  postId: string,
  _prev: FormState,
  form: FormData,
): Promise<FormState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to comment." };
  const body = text(form, "body");
  if (!body) return { ok: false, fieldErrors: { body: "Write a comment." } };
  try {
    await addComment(viewer, slug, postId, body);
    revalidatePath(`/groups/${slug}`);
    return { ok: true };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not comment." };
  }
}

export async function likePostAction(
  slug: string,
  postId: string,
): Promise<{ ok: boolean; reaction_count?: number; message?: string }> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to like." };
  try {
    const r = await reactToPost(viewer, slug, postId);
    revalidatePath(`/groups/${slug}`);
    return { ok: true, reaction_count: r.reaction_count };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not like." };
  }
}

export async function loadCommentsAction(
  slug: string,
  postId: string,
): Promise<{ ok: boolean; comments?: GroupComment[]; message?: string }> {
  const viewer = await currentViewer();
  try {
    const comments = await listComments(slug, postId, viewer);
    return { ok: true, comments };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not load comments." };
  }
}
