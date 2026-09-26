"use server";

import { revalidatePath, revalidateTag } from "next/cache";
import type { FormState } from "@/lib/form-state";
import {
  EVIDENCE,
  deleteApp,
  entryHref,
  isAppType,
  isBuildMethod,
  isKind,
  isMaturity,
  isPricing,
  isVisibility,
  linkCandidates,
  moderateApp,
  saveApp,
  setStar,
  type Evidence,
  type LinkInput,
  type ShowcaseAgent,
  type ShowcaseLink,
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

/** Directory links the picker serialised as JSON: [{slug, role}]. */
function links(form: FormData, key: string): LinkInput[] {
  try {
    const raw = JSON.parse(text(form, key) || "[]") as unknown;
    if (!Array.isArray(raw)) return [];
    return raw
      .filter((l): l is LinkInput => typeof l?.slug === "string")
      .map((l) => ({ slug: l.slug, role: typeof l.role === "string" ? l.role : "" }));
  } catch {
    return [];
  }
}

function isHttpUrl(value: string): boolean {
  return /^https?:\/\/\S+$/i.test(value);
}

const URL_FIELDS = ["logo_url", "repo_url", "demo_url", "website_url", "docs_url", "status_page_url"];

/** Map the API's validation message onto the field it is about. */
function fieldFor(message: string): string | null {
  const m = message.toLowerCase();
  if (m.includes("needs evidence")) return "evidence";
  if (m.includes("open jobs you posted") || m.includes("open roles")) return "job_ids";
  if (m.includes("no team called") || m.includes("which team")) return "team_slug";
  if (m.includes("no agent called") || m.includes("at least two agents") || m.includes("link at most") || m.includes("agent roles") || m.includes("link to itself") || m.includes("link other agents")) return "agent_links";
  if (m.includes("capabilit")) return "capabilities";
  if (m.includes("model provider")) return "model_provider";
  if (m.includes("model the agent")) return "ai_models";
  if (m.includes("mcp")) return "mcp_servers";
  if (m.includes("tool")) return "tools";
  if (m.startsWith("name")) return "name";
  if (m.includes("tagline")) return "tagline";
  if (m.includes("describe the") || m.includes("description")) return "description_md";
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

function refresh(entry?: { kind: ShowcaseLink["kind"]; slug: string }) {
  revalidateTag("showcase");
  revalidatePath("/showcase");
  revalidatePath("/agents");
  revalidatePath("/showcase/mine");
  if (entry) revalidatePath(entryHref(entry));
}

export async function saveAppAction(_prev: FormState, form: FormData): Promise<FormState> {
  const viewer = await currentViewer();
  if (!viewer) return { ok: false, message: "Sign in to add to the showcase." };

  const name = text(form, "name");
  const submit = form.get("intent") !== "draft";
  const fieldErrors: Record<string, string> = {};
  if (!name) fieldErrors.name = "Give it a name.";
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
    const kind = pick("kind", isKind) ?? "app";
    const app = await saveApp(
      viewer,
      {
        kind,
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
        model_provider: text(form, "model_provider"),
        tools: list(form, "tools"),
        mcp_servers: list(form, "mcp_servers"),
        capabilities: form.getAll("capabilities").map(String),
        agent_links: links(form, "agent_links"),
        team_slug: links(form, "team_slug")[0]?.slug ?? "",
        job_ids: form.getAll("job_ids").map(String),
        submit,
      },
      id,
    );
    refresh(app);
    return {
      ok: true,
      message:
        app.status === "published"
          ? "Published."
          : app.status === "pending_review"
            ? "Submitted for review."
            : "Draft saved.",
      result: { id: entryHref(app), status: app.status },
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
    refresh(app);
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

/** Directory search for the link picker: public entries plus the viewer's own. */
export async function linkCandidatesAction(
  kind: "agent" | "team",
  q: string,
): Promise<ShowcaseLink[]> {
  const viewer = await currentViewer();
  if (!viewer) return [];
  const found = await linkCandidates(viewer, kind, q.slice(0, 100)).catch(() => []);
  return found.map((e) => ({
    slug: e.slug,
    kind: e.kind,
    name: e.name,
    tagline: e.tagline,
    logo_url: e.logo_url,
    role: "",
  }));
}
