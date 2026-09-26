"use server";

import { revalidatePath, revalidateTag } from "next/cache";
import type { FormState } from "@/app/actions";
import {
  EVIDENCE,
  deleteApp,
  isAppType,
  isBuildMethod,
  isMaturity,
  isPricing,
  isVisibility,
  moderateApp,
  saveApp,
  setStar,
  type Evidence,
  type ShowcaseAgent,
  type ShowcaseModeration,
} from "@/lib/showcase";
import { currentViewer } from "@/lib/session";

function text(form: FormData, key: string): string {
  return (form.get(key) as string | null)?.trim() ?? "";
}

/** Comma- or newline-separated values. */
function list(form: FormData, key: string): string[] {
  return text(form, key)
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean);
}

/** One agent per line: "Name — what it did" (also accepts " - " or ":"). */
function agents(form: FormData): ShowcaseAgent[] {
  return text(form, "agents")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const m = line.match(/^(.+?)\s*(?:—|–|\s-\s|:)\s*(.*)$/);
      return m ? { name: m[1].trim(), role: m[2].trim() } : { name: line, role: "" };
    });
}

function isHttpUrl(value: string): boolean {
  return /^https?:\/\/\S+$/i.test(value);
}

const URL_FIELDS = ["logo_url", "repo_url", "demo_url", "website_url", "docs_url", "status_page_url"];

/** Map the API's validation message onto the field it is about. */
function fieldFor(message: string): string | null {
  const m = message.toLowerCase();
  if (m.includes("needs evidence")) return "evidence";
  if (m.startsWith("name")) return "name";
  if (m.includes("tagline")) return "tagline";
  if (m.includes("describe the app") || m.includes("description")) return "description_md";
  if (m.includes("kind of app")) return "app_type";
  if (m.includes("mature")) return "maturity";
  if (m.includes("how the app was built")) return "build_method";
  if (m.includes("agent")) return "agents";
  if (m.includes("screenshot")) return "screenshots";
  if (m.includes("logo")) return "logo_url";
  if (m.includes("repository, a demo")) return "repo_url";
  if (m.includes("repository link")) return "repo_url";
  if (m.includes("demo link")) return "demo_url";
  if (m.includes("website link")) return "website_url";
  if (m.includes("docs link")) return "docs_url";
  if (m.includes("status page")) return "status_page_url";
  if (m.includes("technolog")) return "technologies";
  if (m.includes("model")) return "ai_models";
  if (m.includes("oversight")) return "human_oversight";
  if (m.includes("licence")) return "license";
  if (m.includes("version")) return "version";
  if (m.includes("team")) return "team_name";
  return null;
}

function refresh(slug?: string) {
  revalidateTag("showcase");
  revalidatePath("/showcase");
  revalidatePath("/showcase/mine");
  if (slug) revalidatePath(`/showcase/${slug}`);
}

export async function saveAppAction(_prev: FormState, form: FormData): Promise<FormState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to add an app." };

  const name = text(form, "name");
  const submit = form.get("intent") !== "draft";
  const fieldErrors: Record<string, string> = {};
  if (!name) fieldErrors.name = "Give the app a name.";
  for (const key of URL_FIELDS) {
    const v = text(form, key);
    if (v && !isHttpUrl(v)) fieldErrors[key] = "Links need to start with https://";
  }
  const screenshots = list(form, "screenshots");
  if (screenshots.some((u) => !isHttpUrl(u))) {
    fieldErrors.screenshots = "Each screenshot needs to be an https:// link, one per line.";
  }
  if (Object.keys(fieldErrors).length > 0) {
    return { ok: false, message: "Check the highlighted fields.", fieldErrors };
  }

  const pick = <T,>(key: string, guard: (v: unknown) => v is T): T | undefined => {
    const v = text(form, key);
    return guard(v) ? v : undefined;
  };
  const evidence = Object.fromEntries(
    EVIDENCE.map(({ key }) => [key, form.get(`evidence_${key}`) === "on"]),
  ) as Omit<Evidence, "status_page_url">;

  const id = text(form, "id") || undefined;
  try {
    const app = await saveApp(
      viewer,
      {
        name,
        tagline: text(form, "tagline"),
        description_md: text(form, "description_md"),
        // Undefined drops out of the JSON, which the API reads as "not chosen yet".
        app_type: pick("app_type", isAppType) as never,
        maturity: pick("maturity", isMaturity) as never,
        build_method: pick("build_method", isBuildMethod) as never,
        pricing: pick("pricing", isPricing) as never,
        visibility: pick("visibility", isVisibility) ?? "public",
        license: text(form, "license"),
        version: text(form, "version"),
        logo_url: text(form, "logo_url"),
        screenshots,
        repo_url: text(form, "repo_url"),
        demo_url: text(form, "demo_url"),
        website_url: text(form, "website_url"),
        docs_url: text(form, "docs_url"),
        technologies: list(form, "technologies"),
        ai_models: list(form, "ai_models"),
        agents: agents(form),
        human_oversight: text(form, "human_oversight"),
        evidence: { ...evidence, status_page_url: text(form, "status_page_url") },
        team_name: text(form, "team_name"),
        submit,
      },
      id,
    );
    refresh(app.slug);
    return {
      ok: true,
      message:
        app.status === "published"
          ? "Published."
          : app.status === "pending_review"
            ? "Submitted for review."
            : "Draft saved.",
      result: { id: app.slug, status: app.status },
    };
  } catch (e) {
    const message = e instanceof Error ? e.message : "Could not save.";
    const field = fieldFor(message);
    return {
      ok: false,
      message: field ? "Check the highlighted field." : message,
      fieldErrors: field
        ? { [field]: message.charAt(0).toUpperCase() + message.slice(1) + "." }
        : undefined,
    };
  }
}

export async function deleteAppAction(form: FormData): Promise<void> {
  const viewer = await currentViewer();
  if (!viewer) return;
  await deleteApp(viewer, text(form, "id"));
  refresh();
}

export async function moderateAppAction(_prev: FormState, form: FormData): Promise<FormState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in first." };
  const action = text(form, "action") as ShowcaseModeration;
  const note = text(form, "note");
  if (action === "reject" && !note) {
    return {
      ok: false,
      message: "Tell the creator what to change.",
      fieldErrors: { note: "Tell the creator what to change." },
    };
  }
  try {
    const app = await moderateApp(viewer, text(form, "id"), action, note);
    refresh(app.slug);
    revalidatePath("/showcase/review");
    const done: Record<ShowcaseModeration, string> = {
      approve: "Published.",
      reject: "Sent back to the creator.",
      feature: "Featured on the showcase front page.",
      unfeature: "No longer featured.",
      archive: "Archived.",
    };
    return { ok: true, message: done[action], result: { status: app.status } };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not update." };
  }
}

export async function toggleStarAction(
  slug: string,
  on: boolean,
): Promise<{ ok: boolean; starred?: boolean; star_count?: number; message?: string }> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to star apps." };
  try {
    const r = await setStar(viewer, slug, on);
    revalidateTag("showcase");
    return { ok: true, ...r };
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "Could not update the star." };
  }
}
