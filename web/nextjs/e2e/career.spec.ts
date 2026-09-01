import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI, navigateTo } from "./helpers";

const API_URL = process.env.E2E_API_URL ?? "http://localhost:8090/api/v1";

const GOLDEN_JD = `# Head of AI Platform

Company: Northwind Labs

We are hiring a Head of AI Platform to lead inference infrastructure.

Requirements:
- 10+ years in distributed systems
- Kubernetes, GPU scheduling, observability

Compensation: £180k–£220k. Remote (UK).
`;

let creds: { email: string; password: string; token: string };

async function authJSON(path: string, init: RequestInit = {}) {
  const res = await fetch(`${API_URL}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${creds.token}`,
      "Content-Type": "application/json",
      ...(init.headers ?? {}),
    },
  });
  const text = await res.text();
  let body: unknown = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = text;
    }
  }
  return { res, body };
}

test.describe("CareerOps", () => {
  test.beforeAll(async () => {
    creds = await registerViaAPI("career");
  });

  test("a new org gets the built-in CareerOps agent", async () => {
    const { res, body } = await authJSON("/agents?per_page=100");
    expect(res.status).toBe(200);
    const data = (
      body as { data: Array<{ name: string; status: string; metadata?: Record<string, string> }> }
    ).data;
    const agent = data.find((a) => a.metadata?.builtin === "career_ops");
    expect(agent).toBeTruthy();
    expect(agent!.name).toBe("CareerOps");
    expect(agent!.status).toBe("active");
  });

  test("doctor reports an empty profile", async () => {
    const { res, body } = await authJSON("/career/doctor");
    expect(res.status).toBe(200);
    const rep = body as { ok: boolean; warnings: string[] };
    expect(rep.ok).toBe(false);
    expect(rep.warnings.length).toBeGreaterThan(0);
  });

  test("evaluate a fixture JD and see a tracker row", async () => {
    const { res, body } = await authJSON("/career/evaluate", {
      method: "POST",
      body: JSON.stringify({ jd_text: GOLDEN_JD, mode: "full" }),
    });
    expect(res.status).toBe(200);
    const out = body as {
      evaluation?: { score?: { overall?: number }; report_markdown?: string; blocks?: { g?: string } };
      application?: { status: string; role: string };
      dead?: boolean;
    };
    expect(out.dead).toBeFalsy();
    expect(out.evaluation).toBeTruthy();
    expect(out.application).toBeTruthy();
    expect(out.evaluation!.report_markdown).toMatch(/Score/i);
    expect(out.application!.status).toMatch(/evaluated|skip/);

    const tracker = await authJSON("/career/applications?per_page=20");
    expect(tracker.res.status).toBe(200);
    const rows = (tracker.body as { data: Array<{ company: string }> }).data;
    expect(rows.length).toBeGreaterThan(0);
  });

  test("cover letter is a draft only", async () => {
    const { body } = await authJSON("/career/evaluate", {
      method: "POST",
      body: JSON.stringify({ jd_text: GOLDEN_JD, mode: "full" }),
    });
    const evalId = (body as { evaluation?: { id: string } }).evaluation?.id;
    expect(evalId).toBeTruthy();
    const cover = await authJSON(`/career/evaluations/${evalId}/cover`, { method: "POST" });
    expect(cover.res.status).toBe(200);
    const art = cover.body as { kind: string; body_markdown: string };
    expect(art.kind).toBe("cover");
    expect(art.body_markdown.length).toBeGreaterThan(0);
  });

  test("Career panel opens in Task Manager", async ({ page }) => {
    await loginViaUI(page, creds.email, creds.password);
    await navigateTo(page, "/panel/task-manager?agent=career");
    await expect(page.getByRole("heading", { name: "Career" })).toBeVisible({
      timeout: 8_000,
    });
    await expect(page.getByText("Paste the JD").or(page.getByText("Evaluate"))).toBeVisible();
  });
});
