"use server";

import { revalidatePath } from "next/cache";
import { createHireOrg, createRequisition } from "@/lib/hire";
import { currentViewer } from "@/lib/session";

export type HireActionState = { ok: boolean; error?: string; orgId?: string };

export async function createOrgAction(
  _prev: HireActionState,
  form: FormData,
): Promise<HireActionState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, error: "Sign in to create a hiring organisation." };
  const name = String(form.get("name") ?? "").trim();
  const slug = String(form.get("slug") ?? "")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, "-")
    .replace(/^-|-$/g, "");
  if (!name || !slug) return { ok: false, error: "Name and slug are required." };
  try {
    const org = await createHireOrg(viewer, { name, slug });
    revalidatePath("/hire");
    return { ok: true, orgId: org.id };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "Could not create org." };
  }
}

export async function createRequisitionAction(
  _prev: HireActionState,
  form: FormData,
): Promise<HireActionState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, error: "Sign in required." };
  const orgId = String(form.get("org_id") ?? "").trim();
  const title = String(form.get("title") ?? "").trim();
  if (!orgId || !title) return { ok: false, error: "Organisation and title are required." };
  const skills = String(form.get("skills") ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  try {
    await createRequisition(viewer, orgId, {
      title,
      description: String(form.get("description") ?? ""),
      kind: String(form.get("kind") ?? "permanent"),
      location: String(form.get("location") ?? ""),
      remote_ok: form.get("remote_ok") === "on",
      skills,
    });
    revalidatePath("/hire");
    return { ok: true, orgId };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "Could not create requisition." };
  }
}
